// Package spec provides types and functions for generating epics and tasks from design specifications
package spec

// SpecAnalysis represents the AI's analysis of design specs
type SpecAnalysis struct {
	Epics []EpicSpec `json:"epics"`
}

// EpicSpec represents an epic with its tasks
type EpicSpec struct {
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Tasks       []TaskSpec `json:"tasks"`
}

// TaskSpec represents a task (story) with optional subtasks
type TaskSpec struct {
	Title             string        `json:"title"`
	Description       string        `json:"description"`
	Type              string        `json:"type"`              // feature, bug, refactor, test, docs, research, fix, other
	Priority          int           `json:"priority"`
	AcceptanceCriteria []string     `json:"acceptance_criteria"`
	TestMode          string        `json:"test_mode"`         // strict, lenient, disabled
	TestScope         string        `json:"test_scope"`        // all, diff, skip
	SubTasks          []SubTaskSpec `json:"sub_tasks,omitempty"`
	BlockedBy         []string      `json:"blocked_by,omitempty"`
}

// SubTaskSpec represents a subtask
type SubTaskSpec struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Priority    int      `json:"priority"`
	BlockedBy   []string `json:"blocked_by,omitempty"`
}

// ValidationResult represents validation errors
type ValidationResult struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors"`
}
