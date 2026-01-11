package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/cloud-shuttle/drover/internal/config"
	"github.com/cloud-shuttle/drover/pkg/telemetry"
	"github.com/cloud-shuttle/drover/pkg/types"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// OpenCodeExecutor runs tasks using OpenCode CLI
type OpenCodeExecutor struct {
	opencodePath string
	model        string
	serverURL    string
	timeout      time.Duration
	verbose      bool
}

// OpenCodeEvent represents a JSON event from OpenCode CLI
type OpenCodeEvent struct {
	Type    string          `json:"type"`
	Content json.RawMessage `json:"content,omitempty"`
	Text    string          `json:"text,omitempty"`
	Title   string          `json:"title,omitempty"`
}

// NewOpenCodeExecutor creates a new OpenCode executor from configuration
func NewOpenCodeExecutor(cfg *config.Config) (*OpenCodeExecutor, error) {
	if err := config.ValidateOpenCodeModel(cfg.OpenCodeModel); err != nil {
		return nil, err
	}

	return &OpenCodeExecutor{
		opencodePath: cfg.OpenCodePath,
		model:        cfg.OpenCodeModel,
		serverURL:    cfg.OpenCodeURL,
		timeout:      cfg.TaskTimeout,
		verbose:      cfg.Verbose,
	}, nil
}

// NewOpenCodeExecutorRaw creates a new OpenCode executor with raw parameters (for testing)
func NewOpenCodeExecutorRaw(opencodePath, model string, timeout interface{}) *OpenCodeExecutor {
	return &OpenCodeExecutor{
		opencodePath: opencodePath,
		model:        model,
		timeout:      timeout.(time.Duration),
		verbose:      false,
	}
}

func (e *OpenCodeExecutor) SetVerbose(v bool) {
	e.verbose = v
}

func (e *OpenCodeExecutor) Execute(worktreePath string, task *types.Task) *ExecutionResult {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	return e.ExecuteWithContext(ctx, worktreePath, task)
}

func (e *OpenCodeExecutor) ExecuteWithContext(ctx context.Context, worktreePath string, task *types.Task, parentSpan ...trace.Span) *ExecutionResult {
	var agentCtx context.Context
	var span trace.Span
	if len(parentSpan) > 0 && parentSpan[0] != nil {
		agentCtx, span = telemetry.StartAgentSpan(ctx, telemetry.AgentTypeOpenCode, "unknown",
			attribute.String(telemetry.KeyTaskID, task.ID),
			attribute.String(telemetry.KeyTaskTitle, task.Title),
		)
		defer span.End()
	} else {
		agentCtx = ctx
		span = trace.SpanFromContext(ctx)
	}

	telemetry.RecordAgentPrompt(agentCtx, telemetry.AgentTypeOpenCode)

	prompt := e.buildPrompt(task)

	if e.verbose {
		log.Printf("🤖 Sending prompt to OpenCode (model: %s)", e.model)
		log.Printf("📝 Prompt preview: %s", truncateString(prompt, 200))
	}

	result := e.executeOpenCode(agentCtx, worktreePath, prompt)

	telemetry.RecordAgentDuration(agentCtx, telemetry.AgentTypeOpenCode, result.Duration)

	return result
}

func (e *OpenCodeExecutor) buildPrompt(task *types.Task) string {
	var prompt strings.Builder

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

func (e *OpenCodeExecutor) executeOpenCode(ctx context.Context, worktreePath, prompt string) *ExecutionResult {
	args := e.buildArgs(prompt)

	cmd := exec.CommandContext(ctx, e.opencodePath, args...)
	cmd.Dir = worktreePath

	var outputBuf strings.Builder
	cmd.Stdout = io.MultiWriter(os.Stdout, &outputBuf)
	cmd.Stderr = os.Stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	fullOutput := outputBuf.String()

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return &ExecutionResult{
				Success:  false,
				Output:   fullOutput,
				Error:    fmt.Errorf("opencode timed out after %v", duration),
				Duration: duration,
			}
		}
		return &ExecutionResult{
			Success:  false,
			Output:   fullOutput,
			Error:    fmt.Errorf("opencode failed after %v: %w", duration, err),
			Duration: duration,
		}
	}

	return &ExecutionResult{
		Success:  true,
		Output:   fullOutput,
		Error:    nil,
		Duration: duration,
	}
}

func (e *OpenCodeExecutor) buildArgs(prompt string) []string {
	args := []string{"run", "--model", e.model, "--format", "json"}

	if e.serverURL != "" {
		args = append(args, "--attach", e.serverURL)
	}

	if e.verbose {
		args = append(args, "--title", "drover-task")
	}

	args = append(args, prompt)

	return args
}

func (e *OpenCodeExecutor) executeWithStreaming(ctx context.Context, worktreePath, prompt string) *ExecutionResult {
	args := e.buildArgsWithFiles(prompt)

	cmd := exec.CommandContext(ctx, e.opencodePath, args...)
	cmd.Dir = worktreePath

	var outputBuf strings.Builder
	cmd.Stdout = io.MultiWriter(os.Stdout, &outputBuf)
	cmd.Stderr = os.Stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	fullOutput := outputBuf.String()

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return &ExecutionResult{
				Success:  false,
				Output:   fullOutput,
				Error:    fmt.Errorf("opencode timed out after %v", duration),
				Duration: duration,
			}
		}
		return &ExecutionResult{
			Success:  false,
			Output:   fullOutput,
			Error:    fmt.Errorf("opencode failed after %v: %w", duration, err),
			Duration: duration,
		}
	}

	return &ExecutionResult{
		Success:  true,
		Output:   fullOutput,
		Error:    nil,
		Duration: duration,
	}
}

func (e *OpenCodeExecutor) buildArgsWithFiles(prompt string) []string {
	args := []string{"run", "--model", e.model, "--format", "json", "--title", "drover-task"}

	if e.serverURL != "" {
		args = append(args, "--attach", e.serverURL)
	}

	args = append(args, prompt)

	return args
}

func parseOpenCodeEvent(line string) (*OpenCodeEvent, error) {
	var event OpenCodeEvent
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		return nil, fmt.Errorf("failed to parse OpenCode event: %w", err)
	}
	return &event, nil
}

// CheckOpenCodeInstalled verifies OpenCode CLI is available
func CheckOpenCodeInstalled(path string) error {
	cmd := exec.Command(path, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("opencode not found at %s: %w\n%s", path, err, output)
	}
	return nil
}
