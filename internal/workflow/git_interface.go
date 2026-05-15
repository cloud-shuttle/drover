// Package workflow implements durable workflows using DBOS
package workflow

import (
	"github.com/cloud-shuttle/drover/pkg/types"
)

// GitManager defines the interface for git worktree operations needed by the orchestrator.
// This abstraction allows for testing with mock implementations.
type GitManager interface {
	// Create creates a new worktree for a task and returns the worktree path.
	Create(task *types.Task) (string, error)

	// Commit commits changes for a task and returns whether there were changes.
	Commit(taskID, message string) (bool, error)

	// MergeToMain merges the worktree changes to the main branch.
	MergeToMain(taskID string) error

	// Remove cleans up a worktree for a task.
	Remove(taskID string) error

	// Cleanup cleans up all worktrees.
	Cleanup() error

	// SetVerbose enables or disables verbose logging.
	SetVerbose(v bool)
}
