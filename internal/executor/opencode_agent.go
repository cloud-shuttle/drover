// Package executor provides OpenCode agent implementation
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

	ctxmngr "github.com/cloud-shuttle/drover/internal/context"
	"github.com/cloud-shuttle/drover/internal/taskcontext"
	"github.com/cloud-shuttle/drover/pkg/telemetry"
	"github.com/cloud-shuttle/drover/pkg/types"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// OpenCodeAgent runs tasks using OpenCode CLI
type OpenCodeAgent struct {
	opencodePath      string
	timeout           time.Duration
	verbose           bool
	projectGuidelines string
	contextManager    *ctxmngr.Manager
	recentTasks       []*types.Task
	taskContextCount  int
}

// NewOpenCodeAgent creates a new OpenCode agent
func NewOpenCodeAgent(opencodePath string, timeout time.Duration) *OpenCodeAgent {
	return &OpenCodeAgent{
		opencodePath: opencodePath,
		timeout:      timeout,
		verbose:      false,
	}
}

// SetVerbose enables or disables verbose logging
func (a *OpenCodeAgent) SetVerbose(v bool) {
	a.verbose = v
}

// SetProjectGuidelines sets project-specific guidelines for the agent
func (a *OpenCodeAgent) SetProjectGuidelines(guidelines string) {
	a.projectGuidelines = guidelines
}

// SetContextManager sets the context window manager for the agent
func (a *OpenCodeAgent) SetContextManager(manager *ctxmngr.Manager) {
	a.contextManager = manager
}

// SetTaskContext sets recent completed tasks for context carrying
func (a *OpenCodeAgent) SetTaskContext(recentTasks []*types.Task, taskContextCount int) {
	a.recentTasks = recentTasks
	a.taskContextCount = taskContextCount
}

// ExecuteWithContext runs a task with a context and returns the execution result
func (a *OpenCodeAgent) ExecuteWithContext(ctx context.Context, worktreePath string, task *types.Task, parentSpan ...trace.Span) *ExecutionResult {
	// Start telemetry span for agent execution
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

	// Record agent prompt
	telemetry.RecordAgentPrompt(agentCtx, telemetry.AgentTypeOpenCode)

	// Build the prompt
	prompt := a.buildPrompt(task)

	// Log what we're sending to OpenCode (verbose only)
	if a.verbose {
		log.Printf("🤖 Sending prompt to OpenCode (length: %d chars)", len(prompt))
		log.Printf("📝 Prompt preview: %s", truncateString(prompt, 200))
	}

	// Run OpenCode with run subcommand and prompt as argument
	// Use --format default for human-readable output
	cmd := exec.CommandContext(ctx, a.opencodePath, "run", prompt)
	cmd.Dir = worktreePath

	// Capture output while also streaming to stdout/stderr for real-time viewing
	var outputBuf, errBuf strings.Builder
	cmd.Stdout = io.MultiWriter(os.Stdout, &outputBuf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &errBuf)

	start := time.Now()
	if a.verbose {
		log.Printf("⏱️  OpenCode execution started at %s", start.Format("15:04:05"))
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
			log.Printf("❌ OpenCode exited with code %d after %v", exitCode, duration)
		}

		// Record error
		telemetry.RecordAgentError(agentCtx, telemetry.AgentTypeOpenCode, "execution_failed")

		if ctx.Err() == context.DeadlineExceeded {
			telemetry.RecordError(span, err, "TimeoutError", telemetry.ErrorCategoryTimeout)
			telemetry.RecordAgentDuration(agentCtx, telemetry.AgentTypeOpenCode, duration)
			return &ExecutionResult{
				Success: false,
				Output:  fullOutput,
				Error:   fmt.Errorf("opencode timed out after %v", duration),
			}
		}
		telemetry.RecordError(span, err, "ExecutionError", telemetry.ErrorCategoryAgent)
		telemetry.RecordAgentDuration(agentCtx, telemetry.AgentTypeOpenCode, duration)
		return &ExecutionResult{
			Success: false,
			Output:  fullOutput,
			Error:   fmt.Errorf("opencode failed after %v: %w", duration, err),
		}
	}

	if a.verbose {
		log.Printf("✅ OpenCode completed successfully in %v", duration)
	}

	// Record successful completion
	telemetry.RecordAgentDuration(agentCtx, telemetry.AgentTypeOpenCode, duration)

	return &ExecutionResult{
		Success: true,
		Output:  fullOutput,
		Error:   nil,
		Duration: duration,
	}
}

// CheckInstalled verifies OpenCode is available
func (a *OpenCodeAgent) CheckInstalled() error {
	cmd := exec.Command(a.opencodePath, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("opencode not found at %s: %w\n%s", a.opencodePath, err, output)
	}
	return nil
}

// buildPrompt creates the OpenCode prompt for a task
func (a *OpenCodeAgent) buildPrompt(task *types.Task) string {
	var prompt strings.Builder

	// Start with project guidelines if configured
	if a.projectGuidelines != "" {
		prompt.WriteString("=== PROJECT GUIDELINES ===\n")
		prompt.WriteString(a.projectGuidelines)
		prompt.WriteString("\n============================\n\n")
	}

	// Inject recent task context if available
	if a.taskContextCount > 0 && len(a.recentTasks) > 0 {
		taskContext := taskcontext.BuildContext(a.recentTasks, task, a.taskContextCount)
		if taskContext != "" {
			prompt.WriteString(taskContext)
		}
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
