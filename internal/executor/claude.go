// Package executor handles Claude Code subprocess execution
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

	"github.com/cloud-shuttle/drover/pkg/types"
)

// ExecutionResult contains the result of a Claude execution
type ExecutionResult struct {
	Success bool
	Output  string
	Error   error
}

// Executor runs tasks using Claude Code
type Executor struct {
	claudePath string
	timeout    time.Duration
	verbose    bool // Enable verbose logging
}

// NewExecutor creates a new Claude Code executor
func NewExecutor(claudePath string, timeout time.Duration) *Executor {
	return &Executor{
		claudePath: claudePath,
		timeout:    timeout,
		verbose:    false,
	}
}

// SetVerbose enables or disables verbose logging
func (e *Executor) SetVerbose(v bool) {
	e.verbose = v
}

// Execute runs a task using Claude Code in the given directory and returns the execution result
func (e *Executor) Execute(worktreePath string, task *types.Task) *ExecutionResult {
	// Build the prompt
	prompt := e.buildPrompt(task)

	// Run Claude Code with prompt as positional argument in print mode
	// Use -p for non-interactive mode and pass prompt as argument
	// Add --dangerously-skip-permissions to avoid hanging on permission prompts
	cmd := exec.Command(e.claudePath, "-p", prompt, "--dangerously-skip-permissions")
	cmd.Dir = worktreePath

	// Capture output while also streaming to stdout/stderr
	var outputBuf, errBuf strings.Builder
	cmd.Stdout = io.MultiWriter(os.Stdout, &outputBuf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &errBuf)

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	// Combine stdout and stderr for the result
	fullOutput := outputBuf.String() + errBuf.String()

	if err != nil {
		return &ExecutionResult{
			Success: false,
			Output:  fullOutput,
			Error:   fmt.Errorf("claude failed after %v: %w", duration, err),
		}
	}

	return &ExecutionResult{
		Success: true,
		Output:  fullOutput,
		Error:   nil,
	}
}

// buildPrompt creates the Claude prompt for a task
func (e *Executor) buildPrompt(task *types.Task) string {
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

// ExecuteWithTimeout runs a task with a timeout and returns the execution result
func (e *Executor) ExecuteWithTimeout(worktreePath string, task *types.Task) *ExecutionResult {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	return e.ExecuteWithContext(ctx, worktreePath, task)
}

// ExecuteWithContext runs a task with a context and returns the execution result
func (e *Executor) ExecuteWithContext(ctx context.Context, worktreePath string, task *types.Task) *ExecutionResult {
	// Build the prompt
	prompt := e.buildPrompt(task)

	// Log what we're sending to Claude (verbose only)
	if e.verbose {
		log.Printf("🤖 Sending prompt to Claude (length: %d chars)", len(prompt))
		log.Printf("📝 Prompt preview: %s", truncateString(prompt, 200))
	}

	// Run Claude Code with prompt as positional argument in print mode
	// Use -p for non-interactive mode and pass prompt as argument
	// Add --dangerously-skip-permissions to avoid hanging on permission prompts
	cmd := exec.CommandContext(ctx, e.claudePath, "-p", prompt, "--dangerously-skip-permissions")
	cmd.Dir = worktreePath

	// Capture output while also streaming to stdout/stderr for real-time viewing
	var outputBuf, errBuf strings.Builder
	cmd.Stdout = io.MultiWriter(os.Stdout, &outputBuf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &errBuf)

	start := time.Now()
	if e.verbose {
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
		if e.verbose {
			log.Printf("❌ Claude exited with code %d after %v", exitCode, duration)
		}

		if ctx.Err() == context.DeadlineExceeded {
			return &ExecutionResult{
				Success: false,
				Output:  fullOutput,
				Error:   fmt.Errorf("claude timed out after %v", duration),
			}
		}
		return &ExecutionResult{
			Success: false,
			Output:  fullOutput,
			Error:   fmt.Errorf("claude failed after %v: %w", duration, err),
		}
	}

	if e.verbose {
		log.Printf("✅ Claude completed successfully in %v", duration)
	}
	return &ExecutionResult{
		Success: true,
		Output:  fullOutput,
		Error:   nil,
	}
}

// truncateString truncates a string to a maximum length for logging
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// CheckClaudeInstalled verifies Claude Code is available
func CheckClaudeInstalled(path string) error {
	cmd := exec.Command(path, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("claude not found at %s: %w\n%s", path, err, output)
	}
	return nil
}
