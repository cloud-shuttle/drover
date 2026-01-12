// Package telemetry provides OpenTelemetry observability for Drover
package telemetry

import "go.opentelemetry.io/otel/attribute"

// Semantic convention keys for Drover-specific attributes
const (
	// Project attributes
	KeyProjectID      = "drover.project.id"
	KeyProjectPath    = "drover.project.path"
	KeyProjectName    = "drover.project.name"

	// Workflow attributes
	KeyWorkflowID     = "drover.workflow.id"
	KeyWorkflowType   = "drover.workflow.type"

	// Task attributes
	KeyTaskID         = "drover.task.id"
	KeyTaskTitle      = "drover.task.title"
	KeyTaskState      = "drover.task.state"
	KeyTaskPriority   = "drover.task.priority"
	KeyTaskAttempt    = "drover.task.attempt"
	KeyEpicID         = "drover.epic.id"

	// Worker attributes
	KeyWorkerID       = "drover.worker.id"
	KeyWorkerCount    = "drover.worker.count"

	// Worktree attributes
	KeyWorktreePath   = "drover.worktree.path"
	KeyWorktreeID     = "drover.worktree.id"

	// Agent attributes
	KeyAgentType      = "drover.agent.type"
	KeyAgentModel     = "drover.agent.model"
	KeyAgentPromptID  = "drover.agent.prompt.id"

	// Blocker attributes
	KeyBlockerType    = "drover.blocker.type"
	KeyBlockerTaskID  = "drover.blocker.task_id"
	KeyBlockerReason  = "drover.blocker.reason"

	// Error attributes
	KeyErrorType      = "drover.error.type"
	KeyErrorCategory  = "drover.error.category"
)

// Common attribute key values
const (
	// Workflow types
	WorkflowTypeSequential = "sequential"
	WorkflowTypeParallel   = "parallel"

	// Agent types
	AgentTypeClaudeCode = "claude-code"
	AgentTypeClaudeAPI  = "claude-api"
	AgentTypeCodex      = "codex"
	AgentTypeAmp        = "amp"

	// Error categories
	ErrorCategoryAgent     = "agent"
	ErrorCategoryGit       = "git"
	ErrorCategoryWorktree  = "worktree"
	ErrorCategoryDatabase  = "database"
	ErrorCategoryTimeout   = "timeout"
	ErrorCategoryUnknown   = "unknown"
)

// TaskAttrs returns a set of attributes for a task
func TaskAttrs(id, title, state string, priority, attempt int) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String(KeyTaskID, id),
		attribute.String(KeyTaskTitle, title),
		attribute.String(KeyTaskState, state),
		attribute.Int(KeyTaskPriority, priority),
		attribute.Int(KeyTaskAttempt, attempt),
	}
}

// WorkerAttrs returns a set of attributes for a worker
func WorkerAttrs(workerID string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String(KeyWorkerID, workerID),
	}
}

// ProjectAttrs returns a set of attributes for a project
func ProjectAttrs(id, path, name string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String(KeyProjectID, id),
		attribute.String(KeyProjectPath, path),
		attribute.String(KeyProjectName, name),
	}
}

// EpicAttrs returns a set of attributes for an epic
func EpicAttrs(epicID string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String(KeyEpicID, epicID),
	}
}
