package workflow

import (
	"errors"
	"github.com/cloud-shuttle/drover/pkg/types"
)

// MockGitManager is a test double for GitManager that allows fine-grained
// control over return values for every operation.
type MockGitManager struct {
	// CreateFn allows overriding the Create behaviour.
	// When nil, Create returns WorktreePath / CreateErr.
	CreateFn func(task *types.Task) (string, error)

	// Return values for Create when CreateFn is nil.
	WorktreePath string
	CreateErr    error

	// CommitFn allows overriding the Commit behaviour.
	CommitFn func(taskID, message string) (bool, error)

	// Return values for Commit when CommitFn is nil.
	HasChanges bool
	CommitErr  error

	// MergeToMainFn allows overriding the MergeToMain behaviour.
	MergeToMainFn func(taskID string) error
	MergeErr      error

	// RemoveFn allows overriding the Remove behaviour.
	RemoveFn  func(taskID string) error
	RemoveErr error

	// CleanupFn allows overriding the Cleanup behaviour.
	CleanupFn  func() error
	CleanupErr error

	// Call tracking
	CreateCalls      []string
	CommitCalls      []string
	MergeCalls       []string
	RemoveCalls      []string
	CleanupCallCount int
}

func (m *MockGitManager) Create(task *types.Task) (string, error) {
	m.CreateCalls = append(m.CreateCalls, task.ID)
	if m.CreateFn != nil {
		return m.CreateFn(task)
	}
	return m.WorktreePath, m.CreateErr
}

func (m *MockGitManager) Commit(taskID, message string) (bool, error) {
	m.CommitCalls = append(m.CommitCalls, taskID)
	if m.CommitFn != nil {
		return m.CommitFn(taskID, message)
	}
	return m.HasChanges, m.CommitErr
}

func (m *MockGitManager) MergeToMain(taskID string) error {
	m.MergeCalls = append(m.MergeCalls, taskID)
	if m.MergeToMainFn != nil {
		return m.MergeToMainFn(taskID)
	}
	return m.MergeErr
}

func (m *MockGitManager) Remove(taskID string) error {
	m.RemoveCalls = append(m.RemoveCalls, taskID)
	if m.RemoveFn != nil {
		return m.RemoveFn(taskID)
	}
	return m.RemoveErr
}

func (m *MockGitManager) Cleanup() error {
	m.CleanupCallCount++
	if m.CleanupFn != nil {
		return m.CleanupFn()
	}
	return m.CleanupErr
}

func (m *MockGitManager) SetVerbose(v bool) {
	// no-op for tests
}

// --- Convenience constructors ---

// NewSuccessGitManager returns a MockGitManager that always succeeds.
func NewSuccessGitManager(worktreePath string) *MockGitManager {
	return &MockGitManager{
		WorktreePath: worktreePath,
		HasChanges:   true,
	}
}

// NewFailingGitManager returns a MockGitManager where Create always fails.
func NewFailingGitManager(err error) *MockGitManager {
	if err == nil {
		err = errors.New("mock git error")
	}
	return &MockGitManager{
		CreateErr: err,
	}
}
