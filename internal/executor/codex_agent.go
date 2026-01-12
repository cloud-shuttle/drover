// Package executor provides OpenAI Codex agent implementation
package executor

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/cloud-shuttle/drover/pkg/telemetry"
	"github.com/cloud-shuttle/drover/pkg/types"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// CodexAgent runs tasks using OpenAI Codex CLI
// See: https://developers.openai.com/codex/cli/
type CodexAgent struct {
	codexPath string
	timeout   time.Duration
	verbose   bool
}

// NewCodexAgent creates a new Codex agent
func NewCodexAgent(codexPath string, timeout time.Duration) *CodexAgent {
	return &CodexAgent{
		codexPath: codexPath,
		timeout:   timeout,
		verbose:   false,
	}
}

// SetVerbose enables or disables verbose logging
func (a *CodexAgent) SetVerbose(v bool) {
	a.verbose = v
}

// ExecuteWithContext runs a task with a context and returns the execution result
func (a *CodexAgent) ExecuteWithContext(ctx context.Context, worktreePath string, task *types.Task, parentSpan ...trace.Span) *ExecutionResult {
	// Start telemetry span for agent execution
	var agentCtx context.Context
	var span trace.Span
	if len(parentSpan) > 0 && parentSpan[0] != nil {
		agentCtx, span = telemetry.StartAgentSpan(ctx, telemetry.AgentTypeCodex, "unknown",
			attribute.String(telemetry.KeyTaskID, task.ID),
			attribute.String(telemetry.KeyTaskTitle, task.Title),
		)
		defer span.End()
	} else {
		agentCtx = ctx
		span = trace.SpanFromContext(ctx)
	}

	// Record agent prompt
	telemetry.RecordAgentPrompt(agentCtx, telemetry.AgentTypeCodex)

	// Build the prompt
	prompt := a.buildPrompt(task)

	// Log what we're sending to Codex (verbose only)
	if a.verbose {
		log.Printf("🤖 Sending prompt to Codex (length: %d chars)", len(prompt))
		log.Printf("📝 Prompt preview: %s", truncateString(prompt, 200))
	}

	// Run Codex with the 'exec' subcommand for non-interactive execution
	// Use --cd to set working directory
	// Use --full-auto for unattended local work (workspace-write sandbox, approvals on failure)
	// See: https://developers.openai.com/codex/cli/reference/
	args := []string{
		"exec",
		"--cd", worktreePath,
		"--full-auto",
		prompt,
	}

	cmd := exec.CommandContext(ctx, a.codexPath, args...)

	// Capture output while also streaming to stdout/stderr for real-time viewing
	var outputBuf, errBuf strings.Builder
	cmd.Stdout = io.MultiWriter(os.Stdout, &outputBuf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &errBuf)

	start := time.Now()
	if a.verbose {
		log.Printf("⏱️  Codex execution started at %s", start.Format("15:04:05"))
	}
	err := cmd.Run()
	duration := time.Since(start)

	// Combine stdout and stderr for the result
	fullOutput := outputBuf.String() + errBuf.String()

	// Log exit code regardless of success/failure
	if err != nil {
		exitCode := 1
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		}
		if a.verbose {
			log.Printf("❌ Codex exited with code %d after %v", exitCode, duration)
		}

		// Record error
		telemetry.RecordAgentError(agentCtx, telemetry.AgentTypeCodex, "execution_failed")

		if ctx.Err() == context.DeadlineExceeded {
			telemetry.RecordError(span, err, "TimeoutError", telemetry.ErrorCategoryTimeout)
			telemetry.RecordAgentDuration(agentCtx, telemetry.AgentTypeCodex, duration)
			return &ExecutionResult{
				Success: false,
				Output:  fullOutput,
				Error:   fmt.Errorf("codex timed out after %v", duration),
			}
		}
		telemetry.RecordError(span, err, "ExecutionError", telemetry.ErrorCategoryAgent)
		telemetry.RecordAgentDuration(agentCtx, telemetry.AgentTypeCodex, duration)
		return &ExecutionResult{
			Success: false,
			Output:  fullOutput,
			Error:   fmt.Errorf("codex failed after %v: %w", duration, err),
		}
	}

	if a.verbose {
		log.Printf("✅ Codex completed successfully in %v", duration)
	}

	// Record successful completion
	telemetry.RecordAgentDuration(agentCtx, telemetry.AgentTypeCodex, duration)

	return &ExecutionResult{
		Success: true,
		Output:  fullOutput,
		Error:   nil,
		Duration: duration,
	}
}

// CheckInstalled verifies Codex CLI is available
func (a *CodexAgent) CheckInstalled() error {
	cmd := exec.Command(a.codexPath, "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("codex not found at %s: %w\n%s", a.codexPath, err, output)
	}
	return nil
}

// buildPrompt creates the Codex prompt for a task
func (a *CodexAgent) buildPrompt(task *types.Task) string {
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
