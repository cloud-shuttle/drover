// Package types defines core data structures for Drover
package types

// TaskStatus represents the current state of a task
type TaskStatus string

const (
	TaskStatusReady      TaskStatus = "ready"
	TaskStatusClaimed    TaskStatus = "claimed"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusBlocked    TaskStatus = "blocked"
	TaskStatusCompleted  TaskStatus = "completed"
	TaskStatusFailed     TaskStatus = "failed"
)

// Task represents a unit of work for an AI agent
type Task struct {
	ID          string     `json:"id" db:"id"`
	Title       string     `json:"title" db:"title"`
	Description string     `json:"description" db:"description"`
	EpicID      string     `json:"epic_id" db:"epic_id"`
	Priority    int        `json:"priority" db:"priority"`
	Status      TaskStatus `json:"status" db:"status"`
	Attempts    int        `json:"attempts" db:"attempts"`
	MaxAttempts int        `json:"max_attempts" db:"max_attempts"`
	LastError   string     `json:"last_error" db:"last_error"`
	ClaimedBy   string     `json:"claimed_by" db:"claimed_by"`
	ClaimedAt   *int64     `json:"claimed_at" db:"claimed_at"`
	CreatedAt   int64      `json:"created_at" db:"created_at"`
	UpdatedAt   int64      `json:"updated_at" db:"updated_at"`
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

// ProjectStatus summarizes the current state of all tasks
type ProjectStatus struct {
	Total      int             `json:"total"`
	Ready      int             `json:"ready"`
	Claimed    int             `json:"claimed"`
	InProgress int             `json:"in_progress"`
	Blocked    int             `json:"blocked"`
	Completed  int             `json:"completed"`
	Failed     int             `json:"failed"`
	Epics      []EpicStatus    `json:"epics"`
}

// EpicStatusSummary summarizes a single epic
type EpicStatusSummary struct {
	Epic        Epic  `json:"epic"`
	TotalTasks  int   `json:"total_tasks"`
	Completed   int   `json:"completed"`
	Progress    float64 `json:"progress"`
}
