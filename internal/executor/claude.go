// Package executor handles Claude Code subprocess execution
package executor

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/cloud-shuttle/drover/pkg/types"
)

// Executor runs tasks using Claude Code
type Executor struct {
	claudePath string
	timeout    time.Duration
}

// NewExecutor creates a new Claude Code executor
func NewExecutor(claudePath string, timeout time.Duration) *Executor {
	return &Executor{
		claudePath: claudePath,
		timeout:    timeout,
	}
}

// Execute runs a task using Claude Code in the given directory
func (e *Executor) Execute(worktreePath string, task *types.Task) error {
	// Build the prompt
	prompt := e.buildPrompt(task)

	// Run Claude Code
	cmd := exec.Command(e.claudePath,
		"--prompt", prompt,
		"--non-interactive",
	)
	cmd.Dir = worktreePath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	if err != nil {
		return fmt.Errorf("claude failed after %v: %w", duration, err)
	}

	return nil
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

// ExecuteWithTimeout runs a task with a timeout
func (e *Executor) ExecuteWithTimeout(worktreePath string, task *types.Task) error {
	// For now, just use Execute without explicit timeout
	// In production, we'd use context.WithTimeout
	return e.Execute(worktreePath, task)
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
