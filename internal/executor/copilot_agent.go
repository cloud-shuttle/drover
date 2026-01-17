// Package executor provides Copilot CLI agent implementation
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

// CopilotAgent runs tasks using GitHub Copilot CLI
type CopilotAgent struct {
	copilotPath string
	timeout     time.Duration
	verbose     bool
}

// NewCopilotAgent creates a new Copilot CLI agent
func NewCopilotAgent(copilotPath string, timeout time.Duration) *CopilotAgent {
	return &CopilotAgent{
		copilotPath: copilotPath,
		timeout:     timeout,
		verbose:     false,
	}
}

// SetVerbose enables or disables verbose logging
func (a *CopilotAgent) SetVerbose(v bool) {
	a.verbose = v
}

// ExecuteWithContext runs a task with a context and returns the execution result
func (a *CopilotAgent) ExecuteWithContext(ctx context.Context, worktreePath string, task *types.Task, parentSpan ...trace.Span) *ExecutionResult {
	var agentCtx context.Context
	var span trace.Span
	if len(parentSpan) > 0 && parentSpan[0] != nil {
		agentCtx, span = telemetry.StartAgentSpan(ctx, telemetry.AgentTypeCopilot, "unknown",
			attribute.String(telemetry.KeyTaskID, task.ID),
			attribute.String(telemetry.KeyTaskTitle, task.Title),
		)
		defer span.End()
	} else {
		agentCtx = ctx
		span = trace.SpanFromContext(ctx)
	}

	telemetry.RecordAgentPrompt(agentCtx, telemetry.AgentTypeCopilot)

	prompt := a.buildPrompt(task)

	if a.verbose {
		log.Printf("🤖 Sending prompt to Copilot (length: %d chars)", len(prompt))
		log.Printf("📝 Prompt preview: %s", truncateString(prompt, 200))
	}

	args := []string{
		"-p",
		prompt,
		"--allow-all-paths",
		"--allow-all-tools",
	}

	cmd := exec.CommandContext(ctx, a.copilotPath, args...)
	cmd.Dir = worktreePath

	var outputBuf, errBuf strings.Builder
	cmd.Stdout = io.MultiWriter(os.Stdout, &outputBuf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &errBuf)

	start := time.Now()
	if a.verbose {
		log.Printf("⏱️  Copilot execution started at %s", start.Format("15:04:05"))
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
			log.Printf("❌ Copilot exited with code %d after %v", exitCode, duration)
		}

		telemetry.RecordAgentError(agentCtx, telemetry.AgentTypeCopilot, "execution_failed")

		if ctx.Err() == context.DeadlineExceeded {
			telemetry.RecordError(span, err, "TimeoutError", telemetry.ErrorCategoryTimeout)
			telemetry.RecordAgentDuration(agentCtx, telemetry.AgentTypeCopilot, duration)
			return &ExecutionResult{
				Success: false,
				Output:  fullOutput,
				Error:   fmt.Errorf("copilot timed out after %v", duration),
			}
		}
		telemetry.RecordError(span, err, "ExecutionError", telemetry.ErrorCategoryAgent)
		telemetry.RecordAgentDuration(agentCtx, telemetry.AgentTypeCopilot, duration)
		return &ExecutionResult{
			Success: false,
			Output:  fullOutput,
			Error:   fmt.Errorf("copilot failed after %v: %w", duration, err),
		}
	}

	if a.verbose {
		log.Printf("✅ Copilot completed successfully in %v", duration)
	}

	telemetry.RecordAgentDuration(agentCtx, telemetry.AgentTypeCopilot, duration)

	return &ExecutionResult{
		Success:  true,
		Output:   fullOutput,
		Error:    nil,
		Duration: duration,
	}
}

// CheckInstalled verifies Copilot CLI is available
func (a *CopilotAgent) CheckInstalled() error {
	cmd := exec.Command(a.copilotPath, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("copilot not found at %s: %w\n%s", a.copilotPath, err, output)
	}
	return nil
}

// buildPrompt creates the Copilot prompt for a task
func (a *CopilotAgent) buildPrompt(task *types.Task) string {
	var prompt strings.Builder

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
