// Package worker provides process-isolated task execution for Drover
package worker

import "time"

// WorkerSignal indicates downstream health for backpressure control
type WorkerSignal string

const (
	SignalOK          WorkerSignal = "ok"           // Normal execution
	SignalRateLimited WorkerSignal = "rate_limited" // Rate limit detected
	SignalSlowResponse WorkerSignal = "slow_response" // Slow response
	SignalAPIError    WorkerSignal = "api_error"    // Transient API error
)

// TaskInput represents the input for a worker task execution
type TaskInput struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	EpicID      string   `json:"epic_id,omitempty"`
	Worktree    string   `json:"worktree"`
	Guidance    []string `json:"guidance,omitempty"`
	Timeout     string   `json:"timeout,omitempty"`
	ClaudePath  string   `json:"claude_path,omitempty"`
	Verbose     bool     `json:"verbose,omitempty"`
	MemoryLimit string   `json:"memory_limit,omitempty"`
}

// TaskResult represents the output of a worker task execution
type TaskResult struct {
	Success      bool         `json:"success"`
	TaskID       string       `json:"task_id"`
	Output       string       `json:"output"`
	Error        string       `json:"error,omitempty"`
	DurationMs   int64        `json:"duration_ms"`
	Signal       WorkerSignal `json:"signal"`
	Verdict      string       `json:"verdict,omitempty"`
	VerdictReason string      `json:"verdict_reason,omitempty"`
}

// HeartbeatMessage is sent periodically to stderr for crash recovery
type HeartbeatMessage struct {
	Type      string `json:"type"`
	TaskID    string `json:"task_id"`
	Timestamp int64  `json:"timestamp"`
}

// ProgressMessage is sent to stderr for progress updates
type ProgressMessage struct {
	Type    string `json:"type"`
	TaskID  string `json:"task_id"`
	Message string `json:"message"`
}

// DebugMessage is sent to stderr in verbose mode
type DebugMessage struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// Executor handles the actual task execution
type Executor struct {
	claudePath string
	timeout    time.Duration
	verbose    bool
}

// NewExecutor creates a new worker executor
func NewExecutor(claudePath string, timeout time.Duration, verbose bool) *Executor {
	return &Executor{
		claudePath: claudePath,
		timeout:    timeout,
		verbose:    verbose,
	}
}

// DefaultTimeout is the default task execution timeout
const DefaultTimeout = 30 * time.Minute

// SlowThreshold is the response time threshold for slow response detection
const SlowThreshold = 10 * time.Second

// HeartbeatInterval is how often to send heartbeats
const HeartbeatInterval = 10 * time.Second
