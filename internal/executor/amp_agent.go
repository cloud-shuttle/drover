// Package executor provides Amp agent implementation
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

// AmpAgent runs tasks using Amp CLI
// See: https://ampcode.com/manual
type AmpAgent struct {
	ampPath string
	timeout time.Duration
	verbose bool
}

// NewAmpAgent creates a new Amp agent
func NewAmpAgent(ampPath string, timeout time.Duration) *AmpAgent {
	return &AmpAgent{
		ampPath: ampPath,
		timeout: timeout,
		verbose: false,
	}
}

// SetVerbose enables or disables verbose logging
func (a *AmpAgent) SetVerbose(v bool) {
	a.verbose = v
}

// ExecuteWithContext runs a task with a context and returns the execution result
func (a *AmpAgent) ExecuteWithContext(ctx context.Context, worktreePath string, task *types.Task, parentSpan ...trace.Span) *ExecutionResult {
	// Start telemetry span for agent execution
	var agentCtx context.Context
	var span trace.Span
	if len(parentSpan) > 0 && parentSpan[0] != nil {
		agentCtx, span = telemetry.StartAgentSpan(ctx, telemetry.AgentTypeAmp, "unknown",
			attribute.String(telemetry.KeyTaskID, task.ID),
			attribute.String(telemetry.KeyTaskTitle, task.Title),
		)
		defer span.End()
	} else {
		agentCtx = ctx
		span = trace.SpanFromContext(ctx)
	}

	// Record agent prompt
	telemetry.RecordAgentPrompt(agentCtx, telemetry.AgentTypeAmp)

	// Build the prompt
	prompt := a.buildPrompt(task)

	// Log what we're sending to Amp (verbose only)
	if a.verbose {
		log.Printf("🤖 Sending prompt to Amp (length: %d chars)", len(prompt))
		log.Printf("📝 Prompt preview: %s", truncateString(prompt, 200))
	}

	// Run Amp with -x/--execute flag for non-interactive execution
	// Use --dangerously-allow-all to allow all tool uses without asking
	// See: https://ampcode.com/manual
	args := []string{
		"-x", // --execute: run in execute mode (non-interactive)
		"--dangerously-allow-all",
		prompt,
	}

	cmd := exec.CommandContext(ctx, a.ampPath, args...)
	cmd.Dir = worktreePath

	// Capture output while also streaming to stdout/stderr for real-time viewing
	var outputBuf, errBuf strings.Builder
	cmd.Stdout = io.MultiWriter(os.Stdout, &outputBuf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &errBuf)

	start := time.Now()
	if a.verbose {
		log.Printf("⏱️  Amp execution started at %s", start.Format("15:04:05"))
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
			log.Printf("❌ Amp exited with code %d after %v", exitCode, duration)
		}

		// Record error
		telemetry.RecordAgentError(agentCtx, telemetry.AgentTypeAmp, "execution_failed")

		if ctx.Err() == context.DeadlineExceeded {
			telemetry.RecordError(span, err, "TimeoutError", telemetry.ErrorCategoryTimeout)
			telemetry.RecordAgentDuration(agentCtx, telemetry.AgentTypeAmp, duration)
			return &ExecutionResult{
				Success: false,
				Output:  fullOutput,
				Error:   fmt.Errorf("amp timed out after %v", duration),
			}
		}
		telemetry.RecordError(span, err, "ExecutionError", telemetry.ErrorCategoryAgent)
		telemetry.RecordAgentDuration(agentCtx, telemetry.AgentTypeAmp, duration)
		return &ExecutionResult{
			Success: false,
			Output:  fullOutput,
			Error:   fmt.Errorf("amp failed after %v: %w", duration, err),
		}
	}

	if a.verbose {
		log.Printf("✅ Amp completed successfully in %v", duration)
	}

	// Record successful completion
	telemetry.RecordAgentDuration(agentCtx, telemetry.AgentTypeAmp, duration)

	return &ExecutionResult{
		Success: true,
		Output:  fullOutput,
		Error:   nil,
		Duration: duration,
	}
}

// CheckInstalled verifies Amp CLI is available
func (a *AmpAgent) CheckInstalled() error {
	cmd := exec.Command(a.ampPath, "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("amp not found at %s: %w\n%s", a.ampPath, err, output)
	}
	return nil
}

// buildPrompt creates the Amp prompt for a task
func (a *AmpAgent) buildPrompt(task *types.Task) string {
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
