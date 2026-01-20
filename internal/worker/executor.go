package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/cloud-shuttle/drover/pkg/types"
)

// Execute runs a task and returns the result
func (e *Executor) Execute(input *TaskInput) *TaskResult {
	start := time.Now()

	// Start heartbeat goroutine
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	heartbeatDone := make(chan struct{})
	go e.heartbeatLoop(input.ID, heartbeatDone)
	defer close(heartbeatDone)

	// Build the prompt
	prompt := e.buildPrompt(input)

	// Execute Claude Code
	output, err := e.runClaude(ctx, input.Worktree, prompt)

	duration := time.Since(start)

	// Detect signal from output
	signal := e.detectSignal(output, duration, err)

	// Determine verdict
	verdict, verdictReason := e.determineVerdict(output, err)

	return &TaskResult{
		Success:       err == nil,
		TaskID:        input.ID,
		Output:        output,
		Error:        errorString(err),
		DurationMs:   duration.Milliseconds(),
		Signal:       signal,
		Verdict:      verdict,
		VerdictReason: verdictReason,
	}
}

// buildPrompt creates the Claude prompt from task input
func (e *Executor) buildPrompt(input *TaskInput) string {
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("Task: %s\n", input.Title))

	if input.Description != "" {
		prompt.WriteString(fmt.Sprintf("Description: %s\n", input.Description))
	}

	// Add guidance if provided
	if len(input.Guidance) > 0 {
		prompt.WriteString("\nGuidance:\n")
		for _, g := range input.Guidance {
			prompt.WriteString(fmt.Sprintf("  - %s\n", g))
		}
	}

	prompt.WriteString("\nPlease implement this task completely.")

	if input.EpicID != "" {
		prompt.WriteString(fmt.Sprintf("\n\nThis task is part of epic: %s", input.EpicID))
	}

	return prompt.String()
}

// runClaude executes Claude Code and captures output
func (e *Executor) runClaude(ctx context.Context, worktree, prompt string) (string, error) {
	cmd := exec.CommandContext(ctx, e.claudePath, "-p", prompt, "--dangerously-skip-permissions")
	cmd.Dir = worktree

	// Capture output while also streaming to stdout/stderr
	var outputBuf, errBuf strings.Builder
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start claude: %w", err)
	}

	// Stream output to parent stdout/stderr while capturing
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		io.Copy(io.MultiWriter(os.Stdout, &outputBuf), stdoutPipe)
	}()

	go func() {
		defer wg.Done()
		io.Copy(io.MultiWriter(os.Stderr, &errBuf), stderrPipe)
	}()

	wg.Wait()
	err = cmd.Wait()

	// Combine stdout and stderr for the result
	fullOutput := outputBuf.String() + errBuf.String()

	return fullOutput, err
}

// detectSignal analyzes output and duration to determine worker signal
func (e *Executor) detectSignal(output string, duration time.Duration, execErr error) WorkerSignal {
	// Check for rate limit patterns
	rateLimitPatterns := []string{
		"pre-flight check is taking longer than expected",
		"rate limit",
		"too many requests",
		"429",
	}

	lowerOutput := strings.ToLower(output)
	for _, pattern := range rateLimitPatterns {
		if strings.Contains(lowerOutput, strings.ToLower(pattern)) {
			return SignalRateLimited
		}
	}

	// Check for slow response
	if duration > SlowThreshold {
		return SignalSlowResponse
	}

	// Check for API errors
	if execErr != nil && strings.Contains(execErr.Error(), "context deadline exceeded") {
		return SignalAPIError
	}

	return SignalOK
}

// determineVerdict analyzes output to determine task verdict
func (e *Executor) determineVerdict(output string, execErr error) (string, string) {
	if execErr != nil {
		return string(types.TaskVerdictFail), execErr.Error()
	}

	// Parse output for verdict patterns
	lowerOutput := strings.ToLower(output)

	// Look for explicit verdict markers
	if strings.Contains(lowerOutput, "verdict: pass") || strings.Contains(lowerOutput, "task completed") {
		return string(types.TaskVerdictPass), "Task completed successfully"
	}

	if strings.Contains(lowerOutput, "verdict: fail") || strings.Contains(lowerOutput, "task failed") {
		return string(types.TaskVerdictFail), "Task failed during execution"
	}

	if strings.Contains(lowerOutput, "blocked by") || strings.Contains(lowerOutput, "verdict: blocked") {
		return string(types.TaskVerdictBlocked), "Task blocked by dependencies"
	}

	// Default to pass if no error and no explicit failure
	return string(types.TaskVerdictPass), "Task completed (no explicit verdict found)"
}

// heartbeatLoop sends periodic heartbeats to stderr
func (e *Executor) heartbeatLoop(taskID string, done <-chan struct{}) {
	ticker := time.NewTicker(HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			heartbeat := HeartbeatMessage{
				Type:      "heartbeat",
				TaskID:    taskID,
				Timestamp: time.Now().Unix(),
			}
			if data, err := json.Marshal(heartbeat); err == nil {
				fmt.Fprintf(os.Stderr, "%s\n", string(data))
			}
		}
	}
}

// errorString converts error to string, returning empty if nil
func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
