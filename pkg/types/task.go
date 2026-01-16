// Package types defines core data structures for Drover
package types

// TaskStatus represents the current state of a task
type TaskStatus string

const (
	TaskStatusReady      TaskStatus = "ready"
	TaskStatusClaimed    TaskStatus = "claimed"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusPaused     TaskStatus = "paused"
	TaskStatusBlocked    TaskStatus = "blocked"
	TaskStatusCompleted  TaskStatus = "completed"
	TaskStatusFailed     TaskStatus = "failed"
)

// TaskType represents the type of work a task represents
type TaskType string

const (
	TaskTypeFeature  TaskType = "feature"  // New feature implementation
	TaskTypeBug      TaskType = "bug"      // Bug fix
	TaskTypeRefactor TaskType = "refactor" // Code refactoring
	TaskTypeTest     TaskType = "test"     // Test writing/fixing
	TaskTypeDocs     TaskType = "docs"     // Documentation
	TaskTypeResearch TaskType = "research" // Research/investigation
	TaskTypeFix      TaskType = "fix"      // Fix task (created for blockers)
	TaskTypeOther    TaskType = "other"    // Other type
)

// Task represents a unit of work for an AI agent
type Task struct {
	ID             string     `json:"id" db:"id"`
	Title          string     `json:"title" db:"title"`
	Description    string     `json:"description" db:"description"`
	EpicID         string     `json:"epic_id" db:"epic_id"`
	ParentID       string     `json:"parent_id,omitempty" db:"parent_id"`             // Parent task ID for sub-tasks
	SequenceNumber int        `json:"sequence_number,omitempty" db:"sequence_number"` // Position among siblings (1-indexed)
	Type           TaskType   `json:"type" db:"type"`                                 // Task type (feature, bug, etc.)
	Priority       int        `json:"priority" db:"priority"`
	Status         TaskStatus `json:"status" db:"status"`
	Attempts       int        `json:"attempts" db:"attempts"`
	MaxAttempts    int        `json:"max_attempts" db:"max_attempts"`
	LastError      string     `json:"last_error" db:"last_error"`
	ClaimedBy      string     `json:"claimed_by" db:"claimed_by"`
	ClaimedAt      *int64     `json:"claimed_at" db:"claimed_at"`
	Operator       string     `json:"operator" db:"operator"` // The operator/user who created or claimed this task
	CreatedAt      int64      `json:"created_at" db:"created_at"`
	UpdatedAt      int64      `json:"updated_at" db:"updated_at"`
	// ExecutionContext is not persisted in DB - it's set at runtime for execution
	ExecutionContext *TaskExecutionContext `json:"-" db:"-"` // Runtime execution context (guidance, worktree path, etc.)
}

// EpicStatus represents the state of an epic
type EpicStatus string

const (
	EpicStatusOpen   EpicStatus = "open"
	EpicStatusClosed EpicStatus = "closed"
)

// Epic groups related tasks
type Epic struct {
	ID          string     `json:"id" db:"id"`
	Title       string     `json:"title" db:"title"`
	Description string     `json:"description" db:"description"`
	Status      EpicStatus `json:"status" db:"status"`
	CreatedAt   int64      `json:"created_at" db:"created_at"`
}

// TaskDependency represents a blocked-by relationship
type TaskDependency struct {
	TaskID    string `json:"task_id" db:"task_id"`
	BlockedBy string `json:"blocked_by" db:"blocked_by"`
}

// GuidanceMessage represents a hint or guidance message for a task
type GuidanceMessage struct {
	ID        string `json:"id"`
	TaskID    string `json:"task_id"`
	Message   string `json:"message"`
	CreatedAt int64  `json:"created_at"`
	Delivered bool   `json:"delivered"`
}

// TaskExecutionContext provides additional context for task execution
type TaskExecutionContext struct {
	Guidance     []*GuidanceMessage `json:"guidance,omitempty"`      // Pending guidance messages
	WorktreePath string             `json:"worktree_path,omitempty"` // Path to the worktree
}

// ProjectStatus summarizes the current state of all tasks
type ProjectStatus struct {
	Total      int          `json:"total"`
	Ready      int          `json:"ready"`
	Claimed    int          `json:"claimed"`
	InProgress int          `json:"in_progress"`
	Blocked    int          `json:"blocked"`
	Completed  int          `json:"completed"`
	Failed     int          `json:"failed"`
	Epics      []EpicStatus `json:"epics"`
}

// EpicStatusSummary summarizes a single epic
type EpicStatusSummary struct {
	Epic       Epic    `json:"epic"`
	TotalTasks int     `json:"total_tasks"`
	Completed  int     `json:"completed"`
	Progress   float64 `json:"progress"`
}
