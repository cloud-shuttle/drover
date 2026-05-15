// Package executor provides drover-code agent implementation
package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloud-shuttle/drover/pkg/telemetry"
	"github.com/cloud-shuttle/drover/pkg/types"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const AgentTypeDroverCode = "drovercode"

// DroverCodeAgent runs tasks using the drover-code binary (pure-Go Claude-compatible agent)
type DroverCodeAgent struct {
	binaryPath string
	timeout    time.Duration
	verbose    bool
	guidelines string

	// drover-code specific options (can be set via SetDroverCodeConfig later)
	resultJSONPath   string
	permissionPreset string
	coordinatorMode  bool
	jsonl            bool
}

// NewDroverCodeAgent creates a new drover-code agent
func NewDroverCodeAgent(binaryPath string, timeout time.Duration) *DroverCodeAgent {
	if binaryPath == "" {
		binaryPath = "drover-code"
	}
	return &DroverCodeAgent{
		binaryPath: binaryPath,
		timeout:    timeout,
	}
}

// SetVerbose enables or disables verbose logging
func (a *DroverCodeAgent) SetVerbose(v bool) {
	a.verbose = v
}

// SetProjectGuidelines sets project-specific guidelines for the agent
func (a *DroverCodeAgent) SetProjectGuidelines(guidelines string) {
	a.guidelines = guidelines
}

// SetDroverCodeConfig sets drover-code-specific options (call after NewDroverCodeAgent)
func (a *DroverCodeAgent) SetDroverCodeConfig(resultJSONPath, permissionPreset string, coordinatorMode, jsonl bool) {
	a.resultJSONPath = resultJSONPath
	a.permissionPreset = permissionPreset
	a.coordinatorMode = coordinatorMode
	a.jsonl = jsonl
}

// CheckInstalled verifies drover-code is available
func (a *DroverCodeAgent) CheckInstalled() error {
	cmd := exec.Command(a.binaryPath, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("drover-code not found at %s: %w\n%s", a.binaryPath, err, output)
	}
	return nil
}

// ExecuteWithContext runs a task with a context and returns the execution result
func (a *DroverCodeAgent) ExecuteWithContext(ctx context.Context, worktreePath string, task *types.Task, parentSpan ...trace.Span) *ExecutionResult {
	// Start telemetry span
	var agentCtx context.Context
	var span trace.Span
	if len(parentSpan) > 0 && parentSpan[0] != nil {
		agentCtx, span = telemetry.StartAgentSpan(ctx, "drovercode", "unknown",
			attribute.String(telemetry.KeyTaskID, task.ID),
			attribute.String(telemetry.KeyTaskTitle, task.Title),
		)
		defer span.End()
	} else {
		agentCtx = ctx
		span = trace.SpanFromContext(ctx)
	}

	telemetry.RecordAgentPrompt(agentCtx, "drovercode")

	// Build the prompt (exact same logic as ClaudeAgent)
	prompt := a.buildPrompt(task)

	if a.verbose {
		log.Printf("Sending prompt to drover-code (length: %d chars)", len(prompt))
		log.Printf("Prompt preview: %s", truncateString(prompt, 200))
	}

	// Prepare result file (structured output)
	resultPath := a.resultJSONPath
	if resultPath == "" {
		resultPath = filepath.Join(worktreePath, ".drover-result.json")
		_ = os.MkdirAll(filepath.Dir(resultPath), 0755)
	}

	// drover-code uses stdin for the prompt in headless mode (no -p flag needed)
	cmd := exec.CommandContext(ctx, a.binaryPath)
	cmd.Dir = worktreePath
	cmd.Stdin = strings.NewReader(prompt)

	// Environment setup (this is how drover-code is configured)
	env := append(os.Environ(),
		"ANTHROPIC_API_KEY="+os.Getenv("ANTHROPIC_API_KEY"),
		"ANTHROPIC_MODEL="+os.Getenv("ANTHROPIC_MODEL"),
		"ANTHROPIC_BASE_URL="+os.Getenv("ANTHROPIC_BASE_URL"),
		"DROVER_CODE_HEADLESS=1",
		"DROVER_CODE_RESULT_PATH="+resultPath,
	)

	if a.permissionPreset != "" {
		env = append(env, "DROVER_CODE_PERMISSION_PRESET="+a.permissionPreset)
	}
	if a.coordinatorMode {
		env = append(env, "CLAUDE_CODE_COORDINATOR_MODE=1")
	}
	if a.jsonl {
		env = append(env, "DROVER_CODE_JSONL=1")
	}
	if a.verbose {
		env = append(env, "DROVER_CODE_VERBOSE=1")
	}
	cmd.Env = env

	// Stream output live while capturing
	var outputBuf, errBuf bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &outputBuf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &errBuf)

	start := time.Now()
	if a.verbose {
		log.Printf("drover-code execution started at %s", start.Format("15:04:05"))
	}
	err := cmd.Run()
	duration := time.Since(start)

	fullOutput := outputBuf.String() + errBuf.String()

	if err != nil {
		exitCode := 1
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		}

		if a.verbose {
			log.Printf("drover-code exited with code %d after %v", exitCode, duration)
		}

		telemetry.RecordAgentError(agentCtx, "drovercode", "execution_failed")

		if ctx.Err() == context.DeadlineExceeded {
			telemetry.RecordError(span, err, "TimeoutError", telemetry.ErrorCategoryTimeout)
			telemetry.RecordAgentDuration(agentCtx, "drovercode", duration)
			return &ExecutionResult{
				Success:  false,
				Output:   fullOutput,
				Error:    fmt.Errorf("drover-code timed out after %v", duration),
				Duration: duration,
			}
		}

		// Try to read structured result even on failure
		if result, parseErr := a.readResultJSON(resultPath); parseErr == nil {
			if result.Error != "" {
				err = fmt.Errorf("%s", result.Error)
			}
		}

		telemetry.RecordError(span, err, "ExecutionError", telemetry.ErrorCategoryAgent)
		telemetry.RecordAgentDuration(agentCtx, "drovercode", duration)

		return &ExecutionResult{
			Success:  false,
			Output:   fullOutput,
			Error:    fmt.Errorf("drover-code failed (exit %d) after %v: %w", exitCode, duration, err),
			Duration: duration,
		}
	}

	if a.verbose {
		log.Printf("drover-code completed successfully in %v", duration)
	}

	telemetry.RecordAgentDuration(agentCtx, "drovercode", duration)

	// Prefer structured result if available
	if result, parseErr := a.readResultJSON(resultPath); parseErr == nil {
		return &ExecutionResult{
			Success:  result.Success,
			Output:   result.Output,
			Duration: duration,
		}
	}

	return &ExecutionResult{
		Success:  true,
		Output:   fullOutput,
		Duration: duration,
	}
}

// readResultJSON parses the structured result written by --result-json / DROVER_CODE_RESULT_PATH
func (a *DroverCodeAgent) readResultJSON(path string) (*AgentResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r AgentResult
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

type AgentResult struct {
	Success bool   `json:"success"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
	Version int    `json:"version,omitempty"`
}

// buildPrompt is copied verbatim from ClaudeAgent for 100% consistency
func (a *DroverCodeAgent) buildPrompt(task *types.Task) string {
	var prompt strings.Builder

	if a.guidelines != "" {
		prompt.WriteString("## Project Guidelines\n\n")
		// Expand template variables in guidelines
		expandedGuidelines := expandGuidelineTemplates(a.guidelines, task)
		prompt.WriteString(expandedGuidelines)
		prompt.WriteString("\n\n---\n\n")
	}

	prompt.WriteString(fmt.Sprintf("Task: %s\n", task.Title))

	if task.Description != "" {
		prompt.WriteString(fmt.Sprintf("Description: %s\n", task.Description))
	}

	prompt.WriteString("\nPlease implement this task completely.")

	if len(task.EpicID) > 0 {
		prompt.WriteString(fmt.Sprintf("\n\nThis task is part of epic: %s", task.EpicID))
	}

	return prompt.String()
}

