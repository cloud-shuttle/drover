// Package executor provides Claude Code agent implementation
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

// ClaudeAgent runs tasks using Claude Code CLI
type ClaudeAgent struct {
	claudePath string
	timeout    time.Duration
	verbose    bool
}

// NewClaudeAgent creates a new Claude Code agent
func NewClaudeAgent(claudePath string, timeout time.Duration) *ClaudeAgent {
	return &ClaudeAgent{
		claudePath: claudePath,
		timeout:    timeout,
		verbose:    false,
	}
}

// SetVerbose enables or disables verbose logging
func (a *ClaudeAgent) SetVerbose(v bool) {
	a.verbose = v
}

// ExecuteWithContext runs a task with a context and returns the execution result
func (a *ClaudeAgent) ExecuteWithContext(ctx context.Context, worktreePath string, task *types.Task, parentSpan ...trace.Span) *ExecutionResult {
	// Start telemetry span for agent execution
	var agentCtx context.Context
	var span trace.Span
	if len(parentSpan) > 0 && parentSpan[0] != nil {
		agentCtx, span = telemetry.StartAgentSpan(ctx, telemetry.AgentTypeClaudeCode, "unknown",
			attribute.String(telemetry.KeyTaskID, task.ID),
			attribute.String(telemetry.KeyTaskTitle, task.Title),
		)
		defer span.End()
	} else {
		agentCtx = ctx
		span = trace.SpanFromContext(ctx)
	}

	// Record agent prompt
	telemetry.RecordAgentPrompt(agentCtx, telemetry.AgentTypeClaudeCode)

	// Build the prompt
	prompt := a.buildPrompt(task)

	// Log what we're sending to Claude (verbose only)
	if a.verbose {
		log.Printf("🤖 Sending prompt to Claude (length: %d chars)", len(prompt))
		log.Printf("📝 Prompt preview: %s", truncateString(prompt, 200))
	}

	// Run Claude Code with prompt as positional argument in print mode
	// Use -p for non-interactive mode and pass prompt as argument
	// Add --dangerously-skip-permissions to avoid hanging on permission prompts
	cmd := exec.CommandContext(ctx, a.claudePath, "-p", prompt, "--dangerously-skip-permissions")
	cmd.Dir = worktreePath

	// Capture output while also streaming to stdout/stderr for real-time viewing
	var outputBuf, errBuf strings.Builder
	cmd.Stdout = io.MultiWriter(os.Stdout, &outputBuf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &errBuf)

	start := time.Now()
	if a.verbose {
		log.Printf("⏱️  Claude execution started at %s", start.Format("15:04:05"))
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
			log.Printf("❌ Claude exited with code %d after %v", exitCode, duration)
		}

		// Record error
		telemetry.RecordAgentError(agentCtx, telemetry.AgentTypeClaudeCode, "execution_failed")

		if ctx.Err() == context.DeadlineExceeded {
			telemetry.RecordError(span, err, "TimeoutError", telemetry.ErrorCategoryTimeout)
			telemetry.RecordAgentDuration(agentCtx, telemetry.AgentTypeClaudeCode, duration)
			return &ExecutionResult{
				Success: false,
				Output:  fullOutput,
				Error:   fmt.Errorf("claude timed out after %v", duration),
			}
		}
		telemetry.RecordError(span, err, "ExecutionError", telemetry.ErrorCategoryAgent)
		telemetry.RecordAgentDuration(agentCtx, telemetry.AgentTypeClaudeCode, duration)
		return &ExecutionResult{
			Success: false,
			Output:  fullOutput,
			Error:   fmt.Errorf("claude failed after %v: %w", duration, err),
		}
	}

	if a.verbose {
		log.Printf("✅ Claude completed successfully in %v", duration)
	}

	// Record successful completion
	telemetry.RecordAgentDuration(agentCtx, telemetry.AgentTypeClaudeCode, duration)

	return &ExecutionResult{
		Success: true,
		Output:  fullOutput,
		Error:   nil,
		Duration: duration,
	}
}

// CheckInstalled verifies Claude Code is available
func (a *ClaudeAgent) CheckInstalled() error {
	cmd := exec.Command(a.claudePath, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("claude not found at %s: %w\n%s", a.claudePath, err, output)
	}
	return nil
}

// buildPrompt creates the Claude prompt for a task
func (a *ClaudeAgent) buildPrompt(task *types.Task) string {
	var prompt strings.Builder

	// Start with human guidance if present
	if task.ExecutionContext != nil && len(task.ExecutionContext.Guidance) > 0 {
		prompt.WriteString("=== HUMAN GUIDANCE ===\n")
		prompt.WriteString("The following guidance has been provided for this task:\n\n")
		for i, msg := range task.ExecutionContext.Guidance {
			prompt.WriteString(fmt.Sprintf("[%d] %s\n", i+1, msg.Message))
		}
		prompt.WriteString("======================\n\n")
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
