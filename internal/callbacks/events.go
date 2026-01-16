// Package callbacks provides a callback/hook system for workforce lifecycle events.
// This enables observability integrations (OpenTelemetry, logging, metrics) without
// coupling core logic to specific tools.
package callbacks

import (
	"time"
)

// EventType represents the type of lifecycle event
type EventType string

const (
	// Task lifecycle events
	EventTaskCreated   EventType = "task.created"
	EventTaskAssigned  EventType = "task.assigned"
	EventTaskStarted   EventType = "task.started"
	EventTaskCompleted EventType = "task.completed"
	EventTaskFailed    EventType = "task.failed"
	EventTaskBlocked   EventType = "task.blocked"
	EventTaskUnblocked EventType = "task.unblocked"
	EventTaskRecovered EventType = "task.recovered"

	// Worker lifecycle events
	EventWorkerStarted EventType = "worker.started"
	EventWorkerStopped EventType = "worker.stopped"
	EventWorkerStalled EventType = "worker.stalled"
)

// TaskEventContext provides context for task-related events
type TaskEventContext struct {
	// Task identification
	TaskID    string
	Title     string
	EpicID    string
	Type      string
	Priority  int

	// State information
	PrevState   string
	NewState    string
	Transition  string // e.g., "ready", "retry", "manual"

	// Worker information
	WorkerID    string
	WorkerIndex int

	// Timing
	Timestamp time.Time
	Duration  *time.Duration

	// Attempt information
	Attempt     int
	MaxAttempts int

	// Failure information (for failed/recovered events)
	Error      string
	ErrorType  string
	ErrorCategory string

	// Additional metadata
	Metadata map[string]string
}

// WorkerEventContext provides context for worker-related events
type WorkerEventContext struct {
	// Worker identification
	WorkerID    string
	WorkerIndex int

	// Worker state
	State string // "started", "stopped", "stalled"

	// Current task (if any)
	CurrentTaskID   string
	CurrentTaskTitle string

	// Timing
	Timestamp time.Time

	// Stall information (for stalled events)
	StallReason string
	StallDuration time.Duration

	// Additional metadata
	Metadata map[string]string
}

// RecoveryEventContext provides context for task recovery events
type RecoveryEventContext struct {
	// Task identification
	TaskID string
	Title  string

	// Recovery decision
	Strategy     string // "retry", "decompose", "reassign", "escalate", "abandon"
	Reason       string
	Confidence   float64

	// Failure context
	OriginalError string
	AttemptCount  int

	// Timing
	Timestamp time.Time

	// Additional metadata
	Metadata map[string]string
}

// Callback is the interface that all callback implementations must implement.
// Each method corresponds to a specific lifecycle event type.
// Callbacks should be idempotent where possible and should not panic.
//
// Performance requirements:
// - Each callback invocation should complete in < 1ms
// - Callbacks are invoked synchronously by default
// - For long-running operations, callbacks should spawn goroutines
type Callback interface {
	// Task lifecycle events

	// OnTaskCreated is called when a new task is created
	OnTaskCreated(ctx *TaskEventContext) error

	// OnTaskAssigned is called when a task is claimed by a worker
	OnTaskAssigned(ctx *TaskEventContext) error

	// OnTaskStarted is called when a task begins execution
	OnTaskStarted(ctx *TaskEventContext) error

	// OnTaskCompleted is called when a task completes successfully
	OnTaskCompleted(ctx *TaskEventContext) error

	// OnTaskFailed is called when a task fails
	OnTaskFailed(ctx *TaskEventContext) error

	// OnTaskBlocked is called when a task is blocked by dependencies
	OnTaskBlocked(ctx *TaskEventContext) error

	// OnTaskUnblocked is called when a blocked task becomes ready
	OnTaskUnblocked(ctx *TaskEventContext) error

	// OnTaskRecovered is called when a failed task is recovered
	OnTaskRecovered(ctx *RecoveryEventContext) error

	// Worker lifecycle events

	// OnWorkerStarted is called when a worker starts
	OnWorkerStarted(ctx *WorkerEventContext) error

	// OnWorkerStopped is called when a worker stops
	OnWorkerStopped(ctx *WorkerEventContext) error

	// OnWorkerStalled is called when a worker is unable to claim tasks
	OnWorkerStalled(ctx *WorkerEventContext) error
}

// CallbackFunc is a convenience type for simple function-based callbacks.
// Only implement the events you care about; unimplemented methods return nil.
type CallbackFunc struct {
	OnTaskCreatedFunc   func(*TaskEventContext) error
	OnTaskAssignedFunc  func(*TaskEventContext) error
	OnTaskStartedFunc   func(*TaskEventContext) error
	OnTaskCompletedFunc func(*TaskEventContext) error
	OnTaskFailedFunc    func(*TaskEventContext) error
	OnTaskBlockedFunc   func(*TaskEventContext) error
	OnTaskUnblockedFunc func(*TaskEventContext) error
	OnTaskRecoveredFunc func(*RecoveryEventContext) error
	OnWorkerStartedFunc func(*WorkerEventContext) error
	OnWorkerStoppedFunc func(*WorkerEventContext) error
	OnWorkerStalledFunc func(*WorkerEventContext) error
}

// OnTaskCreated implements Callback
func (c *CallbackFunc) OnTaskCreated(ctx *TaskEventContext) error {
	if c.OnTaskCreatedFunc != nil {
		return c.OnTaskCreatedFunc(ctx)
	}
	return nil
}

// OnTaskAssigned implements Callback
func (c *CallbackFunc) OnTaskAssigned(ctx *TaskEventContext) error {
	if c.OnTaskAssignedFunc != nil {
		return c.OnTaskAssignedFunc(ctx)
	}
	return nil
}

// OnTaskStarted implements Callback
func (c *CallbackFunc) OnTaskStarted(ctx *TaskEventContext) error {
	if c.OnTaskStartedFunc != nil {
		return c.OnTaskStartedFunc(ctx)
	}
	return nil
}

// OnTaskCompleted implements Callback
func (c *CallbackFunc) OnTaskCompleted(ctx *TaskEventContext) error {
	if c.OnTaskCompletedFunc != nil {
		return c.OnTaskCompletedFunc(ctx)
	}
	return nil
}

// OnTaskFailed implements Callback
func (c *CallbackFunc) OnTaskFailed(ctx *TaskEventContext) error {
	if c.OnTaskFailedFunc != nil {
		return c.OnTaskFailedFunc(ctx)
	}
	return nil
}

// OnTaskBlocked implements Callback
func (c *CallbackFunc) OnTaskBlocked(ctx *TaskEventContext) error {
	if c.OnTaskBlockedFunc != nil {
		return c.OnTaskBlockedFunc(ctx)
	}
	return nil
}

// OnTaskUnblocked implements Callback
func (c *CallbackFunc) OnTaskUnblocked(ctx *TaskEventContext) error {
	if c.OnTaskUnblockedFunc != nil {
		return c.OnTaskUnblockedFunc(ctx)
	}
	return nil
}

// OnTaskRecovered implements Callback
func (c *CallbackFunc) OnTaskRecovered(ctx *RecoveryEventContext) error {
	if c.OnTaskRecoveredFunc != nil {
		return c.OnTaskRecoveredFunc(ctx)
	}
	return nil
}

// OnWorkerStarted implements Callback
func (c *CallbackFunc) OnWorkerStarted(ctx *WorkerEventContext) error {
	if c.OnWorkerStartedFunc != nil {
		return c.OnWorkerStartedFunc(ctx)
	}
	return nil
}

// OnWorkerStopped implements Callback
func (c *CallbackFunc) OnWorkerStopped(ctx *WorkerEventContext) error {
	if c.OnWorkerStoppedFunc != nil {
		return c.OnWorkerStoppedFunc(ctx)
	}
	return nil
}

// OnWorkerStalled implements Callback
func (c *CallbackFunc) OnWorkerStalled(ctx *WorkerEventContext) error {
	if c.OnWorkerStalledFunc != nil {
		return c.OnWorkerStalledFunc(ctx)
	}
	return nil
}
