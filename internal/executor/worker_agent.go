// Package executor provides agent execution interfaces for different AI coding agents
package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	ctxmngr "github.com/cloud-shuttle/drover/internal/context"
	"github.com/cloud-shuttle/drover/pkg/types"
	"go.opentelemetry.io/otel/trace"
)

// WorkerAgent executes tasks via the drover-worker subprocess
type WorkerAgent struct {
	workerBinary  string
	claudePath    string
	timeout       time.Duration
	memoryLimit   string
	verbose       bool
}

// NewWorkerAgent creates a new worker subprocess agent
func NewWorkerAgent(workerBinary, claudePath string, timeout time.Duration) *WorkerAgent {
	return &WorkerAgent{
		workerBinary: workerBinary,
		claudePath:   claudePath,
		timeout:      timeout,
	}
}

// SetVerbose enables or disables verbose logging
func (a *WorkerAgent) SetVerbose(v bool) {
	a.verbose = v
}

// SetMemoryLimit sets the memory limit for worker processes
func (a *WorkerAgent) SetMemoryLimit(limit string) {
	a.memoryLimit = limit
}

// SetProjectGuidelines sets project-specific guidelines (not yet supported in worker mode)
func (a *WorkerAgent) SetProjectGuidelines(guidelines string) {
	// Worker mode doesn't support guidelines yet
	// Could be passed via --guidance flag in the future
}

// SetContextManager sets the context window manager (not used in worker mode)
func (a *WorkerAgent) SetContextManager(manager *ctxmngr.Manager) {
	// Worker mode manages its own context
}

// SetTaskContext sets recent completed tasks for context carrying
func (a *WorkerAgent) SetTaskContext(recentTasks []*types.Task, taskContextCount int) {
	// Worker mode doesn't support task context yet
}

// CheckInstalled verifies the drover-worker binary is available
func (a *WorkerAgent) CheckInstalled() error {
	path := a.workerBinary
	if path == "" {
		path = "drover-worker"
	}
	cmd := exec.Command(path, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("drover-worker not found: %w\n%s", err, output)
	}
	return nil
}

// ExecuteWithContext runs a task using the drover-worker subprocess
func (a *WorkerAgent) ExecuteWithContext(ctx context.Context, worktreePath string, task *types.Task, parentSpan ...trace.Span) *ExecutionResult {
	start := time.Now()

	// Build task input for worker
	input := map[string]interface{}{
		"id":          task.ID,
		"title":       task.Title,
		"description": task.Description,
		"worktree":    worktreePath,
		"timeout":     a.timeout.String(),
		"claude_path": a.claudePath,
		"verbose":     a.verbose,
	}

	if task.EpicID != "" {
		input["epic_id"] = task.EpicID
	}

	// Add guidance if available
	if task.ExecutionContext != nil && len(task.ExecutionContext.Guidance) > 0 {
		guidance := make([]string, len(task.ExecutionContext.Guidance))
		for i, g := range task.ExecutionContext.Guidance {
			guidance[i] = g.Message
		}
		input["guidance"] = guidance
	}

	if a.memoryLimit != "" {
		input["memory_limit"] = a.memoryLimit
	}

	// Marshal input to JSON
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return &ExecutionResult{
			Success: false,
			Output:  "",
			Error:   fmt.Errorf("failed to marshal task input: %w", err),
			Duration: time.Since(start),
		}
	}

	// Build command
	args := []string{"execute", "-"}
	cmd := exec.CommandContext(ctx, a.workerBinary, args...)

	// Set up stdin with JSON input
	cmd.Stdin = strings.NewReader(string(inputJSON))

	// Capture stdout (result JSON) and stream stderr (heartbeats, debug output)
	var stdoutBuf, stderrBuf strings.Builder
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)

	// Run the worker
	err = cmd.Run()
	duration := time.Since(start)

	// Parse result from stdout
	var result struct {
		Success       bool   `json:"success"`
		TaskID        string `json:"task_id"`
		Output        string `json:"output"`
		Error         string `json:"error,omitempty"`
		DurationMs    int64  `json:"duration_ms"`
		Signal        string `json:"signal"`
		Verdict       string `json:"verdict,omitempty"`
		VerdictReason string `json:"verdict_reason,omitempty"`
	}

	resultJSON := stdoutBuf.String()
	if resultJSON == "" {
		// Worker failed without producing output
		errMsg := err.Error()
		if stderrBuf.String() != "" {
			errMsg += ": " + stderrBuf.String()
		}
		return &ExecutionResult{
			Success: false,
			Output:  stderrBuf.String(),
			Error:   fmt.Errorf("worker failed: %w", err),
			Duration: duration,
		}
	}

	if err := json.Unmarshal([]byte(resultJSON), &result); err != nil {
		return &ExecutionResult{
			Success: false,
			Output:  resultJSON,
			Error:   fmt.Errorf("failed to parse worker result: %w", err),
			Duration: duration,
		}
	}

	// Return execution result
	execResult := &ExecutionResult{
		Success:  result.Success,
		Output:   result.Output,
		Duration: duration,
	}

	if !result.Success {
		if result.Error != "" {
			execResult.Error = fmt.Errorf("worker error: %s", result.Error)
		} else {
			execResult.Error = fmt.Errorf("worker exited with error")
		}
	}

	return execResult
}
