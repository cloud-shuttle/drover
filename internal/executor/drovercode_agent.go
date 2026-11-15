// Package executor provides drover-code agent implementation
package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	ctxmngr "github.com/cloud-shuttle/drover/internal/context"
	"github.com/cloud-shuttle/drover/internal/sessionevent"
	"github.com/cloud-shuttle/drover/internal/taskcontext"
	"github.com/cloud-shuttle/drover/pkg/telemetry"
	"github.com/cloud-shuttle/drover/pkg/types"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const AgentTypeDroverCode = "drovercode"

// DroverCodeAgent runs tasks using the drover-code binary (pure-Go Claude-compatible agent)
type DroverCodeAgent struct {
	binaryPath string
	timeout    time.Duration
	verbose    bool
	guidelines string

	contextManager   *ctxmngr.Manager
	recentTasks      []*types.Task
	taskContextCount int

	// drover-code specific options (can be set via SetDroverCodeConfig later)
	resultJSONPath   string
	permissionPreset string
	coordinatorMode  bool
	jsonl            bool
}

// NewDroverCodeAgent creates a new drover-code agent
func NewDroverCodeAgent(binaryPath string, timeout time.Duration) *DroverCodeAgent {
	if binaryPath == "" {
		binaryPath = "drover-code"
	}
	return &DroverCodeAgent{
		binaryPath: binaryPath,
		timeout:    timeout,
	}
}

// SetVerbose enables or disables verbose logging
func (a *DroverCodeAgent) SetVerbose(v bool) {
	a.verbose = v
}

// SetProjectGuidelines sets project-specific guidelines for the agent
func (a *DroverCodeAgent) SetProjectGuidelines(guidelines string) {
	a.guidelines = guidelines
}

// SetContextManager sets the context window manager for the agent
func (a *DroverCodeAgent) SetContextManager(manager *ctxmngr.Manager) {
	a.contextManager = manager
}

// SetTaskContext sets recent completed tasks for context carrying
func (a *DroverCodeAgent) SetTaskContext(recentTasks []*types.Task, taskContextCount int) {
	a.recentTasks = recentTasks
	a.taskContextCount = taskContextCount
}

// SetDroverCodeConfig sets drover-code-specific options (call after NewDroverCodeAgent)
func (a *DroverCodeAgent) SetDroverCodeConfig(resultJSONPath, permissionPreset string, coordinatorMode, jsonl bool) {
	a.resultJSONPath = resultJSONPath
	a.permissionPreset = permissionPreset
	a.coordinatorMode = coordinatorMode
	a.jsonl = jsonl
}

// CheckInstalled verifies drover-code is available
func (a *DroverCodeAgent) CheckInstalled() error {
	cmd := exec.Command(a.binaryPath, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("drover-code not found at %s: %w\n%s", a.binaryPath, err, output)
	}
	return nil
}

// ExecuteWithContext runs a task with a context and returns the execution result
func (a *DroverCodeAgent) ExecuteWithContext(ctx context.Context, worktreePath string, task *types.Task, parentSpan ...trace.Span) *ExecutionResult {
	// Start telemetry span
	var agentCtx context.Context
	var span trace.Span
	if len(parentSpan) > 0 && parentSpan[0] != nil {
		agentCtx, span = telemetry.StartAgentSpan(ctx, "drovercode", "unknown",
			attribute.String(telemetry.KeyTaskID, task.ID),
			attribute.String(telemetry.KeyTaskTitle, task.Title),
		)
		defer span.End()
	} else {
		agentCtx = ctx
		span = trace.SpanFromContext(ctx)
	}

	telemetry.RecordAgentPrompt(agentCtx, "drovercode")

	eventsPath, sessionID := beginWorktreeSession(worktreePath, task)
	finalStatus := "failed"
	if eventsPath != "" {
		defer func() {
			ev := sessionevent.New(sessionevent.TypeStatus, sessionID, map[string]any{"status": finalStatus})
			ev.IssueID = task.ID
			ev.Phase = "execute"
			_ = sessionevent.Append(eventsPath, ev)
		}()
	}

	// Build the prompt (exact same logic as ClaudeAgent)
	prompt := a.buildPrompt(task)

	if a.verbose {
		log.Printf("Sending prompt to drover-code (length: %d chars)", len(prompt))
		log.Printf("Prompt preview: %s", truncateString(prompt, 200))
	}

	// Prepare result file (structured output). By default it lives in a
	// fresh temp dir outside the worktree: anything written inside the
	// worktree would be picked up by `git add -A` and merged into main, and
	// a per-run file can't be confused with a stale result from a retry.
	resultPath := a.resultJSONPath
	if resultPath == "" {
		dir, err := os.MkdirTemp("", "drover-code-result-")
		if err != nil {
			return &ExecutionResult{Success: false, Error: fmt.Errorf("creating drover-code result dir: %w", err)}
		}
		defer os.RemoveAll(dir)
		resultPath = filepath.Join(dir, "result.json")
	}

	// drover-code uses stdin for the prompt in headless mode (no -p flag needed)
	cmd := exec.CommandContext(ctx, a.binaryPath)
	cmd.Dir = worktreePath
	cmd.Stdin = strings.NewReader(prompt)

	// Environment setup (this is how drover-code is configured)
	env := append(os.Environ(),
		"ANTHROPIC_API_KEY="+os.Getenv("ANTHROPIC_API_KEY"),
		"ANTHROPIC_MODEL="+os.Getenv("ANTHROPIC_MODEL"),
		"ANTHROPIC_BASE_URL="+os.Getenv("ANTHROPIC_BASE_URL"),
		"DROVER_CODE_HEADLESS=1",
		"DROVER_CODE_RESULT_PATH="+resultPath,
	)

	if a.permissionPreset != "" {
		env = append(env, "DROVER_CODE_PERMISSION_PRESET="+a.permissionPreset)
	}
	if a.coordinatorMode {
		env = append(env, "CLAUDE_CODE_COORDINATOR_MODE=1")
	}
	if a.jsonl {
		env = append(env, "DROVER_CODE_JSONL=1")
	}
	if eventsPath != "" {
		env = append(env,
			"DROVER_SESSION_EVENTS="+eventsPath,
			"DROVER_SESSION_ID="+sessionID,
			"DROVER_ISSUE_ID="+task.ID,
			"DROVER_PHASE=execute",
			"DROVER_SESSION_EVENTS_ENVELOPE=0",
		)
	}
	if a.verbose {
		env = append(env, "DROVER_CODE_VERBOSE=1")
	}
	cmd.Env = env

	// Stream output live while capturing
	var outputBuf, errBuf bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &outputBuf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &errBuf)

	start := time.Now()
	if a.verbose {
		log.Printf("drover-code execution started at %s", start.Format("15:04:05"))
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
			log.Printf("drover-code exited with code %d after %v", exitCode, duration)
		}

		telemetry.RecordAgentError(agentCtx, "drovercode", "execution_failed")

		if ctx.Err() == context.DeadlineExceeded {
			telemetry.RecordError(span, err, "TimeoutError", telemetry.ErrorCategoryTimeout)
			telemetry.RecordAgentDuration(agentCtx, "drovercode", duration)
			return &ExecutionResult{
				Success:  false,
				Output:   fullOutput,
				Error:    fmt.Errorf("drover-code timed out after %v", duration),
				Duration: duration,
			}
		}

		// Try to read structured result even on failure
		if result, parseErr := a.readResultJSON(resultPath); parseErr == nil {
			if result.Error != "" {
				err = fmt.Errorf("%s", result.Error)
			}
		}

		telemetry.RecordError(span, err, "ExecutionError", telemetry.ErrorCategoryAgent)
		telemetry.RecordAgentDuration(agentCtx, "drovercode", duration)

		return &ExecutionResult{
			Success:  false,
			Output:   fullOutput,
			Error:    fmt.Errorf("drover-code failed (exit %d) after %v: %w", exitCode, duration, err),
			Duration: duration,
		}
	}

	if a.verbose {
		log.Printf("drover-code completed successfully in %v", duration)
	}

	telemetry.RecordAgentDuration(agentCtx, "drovercode", duration)

	// Prefer structured result if available
	if result, parseErr := a.readResultJSON(resultPath); parseErr == nil {
		if result.Success {
			finalStatus = "completed"
		}
		return &ExecutionResult{
			Success:  result.Success,
			Output:   result.Output,
			Duration: duration,
		}
	}

	finalStatus = "completed"
	return &ExecutionResult{
		Success:  true,
		Output:   fullOutput,
		Duration: duration,
	}
}

// beginWorktreeSession writes the issue-run envelope beside the worktree.
// drover-code appends tool events to the same file and does not repeat the envelope.
func beginWorktreeSession(worktreePath string, task *types.Task) (string, string) {
	if worktreePath == "" || task == nil {
		return "", ""
	}
	path := filepath.Join(worktreePath, ".drover", "session-events.jsonl")
	ensureSessionEventsIgnored(filepath.Dir(path))
	sessionID := task.ID
	if sessionID == "" {
		sessionID = "worktree"
	}
	started := sessionevent.New(sessionevent.TypeStarted, sessionID, nil)
	started.IssueID = task.ID
	started.Phase = "execute"
	if err := sessionevent.Append(path, started); err != nil {
		return "", ""
	}
	linked := sessionevent.New(sessionevent.TypeLinked, sessionID, map[string]any{
		"issue_id": task.ID,
		"phase":    "execute",
	})
	linked.IssueID = task.ID
	linked.Phase = "execute"
	_ = sessionevent.Append(path, linked)
	running := sessionevent.New(sessionevent.TypeStatus, sessionID, map[string]any{"status": "running"})
	running.IssueID = task.ID
	running.Phase = "execute"
	_ = sessionevent.Append(path, running)
	return path, sessionID
}

// ensureSessionEventsIgnored keeps the session-events file out of the task's
// commit. The worktree is committed with `git add -A`, so without this the
// events log would be merged into main. It writes a self-ignoring
// .drover/.gitignore only when the repo doesn't already have one, so a
// tracked .drover/.gitignore is never modified.
func ensureSessionEventsIgnored(dir string) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}
	p := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(p); err == nil {
		return
	}
	_ = os.WriteFile(p, []byte("# Written by drover: per-run session artifacts, never committed\nsession-events.jsonl\n.gitignore\n"), 0644)
}

// readResultJSON parses the structured result written by --result-json / DROVER_CODE_RESULT_PATH
func (a *DroverCodeAgent) readResultJSON(path string) (*AgentResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r AgentResult
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

type AgentResult struct {
	Success bool   `json:"success"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
	Version int    `json:"version,omitempty"`
}

// buildPrompt mirrors ClaudeAgent.buildPrompt (guidelines, task context,
// human guidance, context-managed description)
func (a *DroverCodeAgent) buildPrompt(task *types.Task) string {
	var prompt strings.Builder

	if a.guidelines != "" {
		prompt.WriteString("## Project Guidelines\n\n")
		// Expand template variables in guidelines
		expandedGuidelines := expandGuidelineTemplates(a.guidelines, task)
		prompt.WriteString(expandedGuidelines)
		prompt.WriteString("\n\n---\n\n")
	}

	// Inject recent task context if available (same as ClaudeAgent)
	if a.taskContextCount > 0 && len(a.recentTasks) > 0 {
		if taskContext := taskcontext.BuildContext(a.recentTasks, task, a.taskContextCount); taskContext != "" {
			prompt.WriteString(taskContext)
		}
	}

	// Human guidance (drover hint) if present (same as ClaudeAgent)
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
		if a.contextManager != nil && a.contextManager.ContentTooLarge(ctxmngr.ContentTypeDescription, task.Description) {
			prompt.WriteString("\nDescription: [Content size exceeds threshold, using reference format]\n")
			prompt.WriteString(a.contextManager.ManageDescription(task.Description))
		} else {
			prompt.WriteString(fmt.Sprintf("Description: %s\n", task.Description))
		}
	}

	prompt.WriteString("\nPlease implement this task completely.")

	if len(task.EpicID) > 0 {
		prompt.WriteString(fmt.Sprintf("\n\nThis task is part of epic: %s", task.EpicID))
	}

	return prompt.String()
}
