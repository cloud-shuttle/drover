package db_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cloud-shuttle/drover/pkg/types"
)

// ============================================================================
// CompleteTask – unblocks dependents
// ============================================================================

func TestStore_CompleteTask_UnblocksDependents(t *testing.T) {
	store, _ := setupTestDB(t)
	defer store.Close()

	// Create blocker → dependent chain
	blocker, err := store.CreateTask("Blocker", "", "", 10, nil)
	if err != nil {
		t.Fatalf("CreateTask blocker: %v", err)
	}

	dependent, err := store.CreateTask("Dependent", "", "", 10, []string{blocker.ID})
	if err != nil {
		t.Fatalf("CreateTask dependent: %v", err)
	}

	// Dependent should start blocked
	status, _ := store.GetTaskStatus(dependent.ID)
	if status != types.TaskStatusBlocked {
		t.Fatalf("expected dependent to start as blocked, got %s", status)
	}

	// Claim and complete the blocker
	_, _ = store.ClaimTask("w1")
	if err := store.CompleteTask(blocker.ID); err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}

	// Dependent should now be ready
	status, _ = store.GetTaskStatus(dependent.ID)
	if status != types.TaskStatusReady {
		t.Errorf("expected dependent to be ready after blocker completed, got %s", status)
	}
}

func TestStore_CompleteTask_MultipleBlockers(t *testing.T) {
	store, _ := setupTestDB(t)
	defer store.Close()

	b1, _ := store.CreateTask("B1", "", "", 0, nil)
	b2, _ := store.CreateTask("B2", "", "", 0, nil)
	dep, _ := store.CreateTask("Dep", "", "", 0, []string{b1.ID, b2.ID})

	// Complete only b1
	_, _ = store.ClaimTask("w1")
	_ = store.CompleteTask(b1.ID)

	// Dep should still be blocked (b2 still pending)
	status, _ := store.GetTaskStatus(dep.ID)
	if status != types.TaskStatusBlocked {
		t.Errorf("expected dep to still be blocked, got %s", status)
	}

	// Complete b2
	_, _ = store.ClaimTask("w2")
	_ = store.CompleteTask(b2.ID)

	// Now dep should be ready
	status, _ = store.GetTaskStatus(dep.ID)
	if status != types.TaskStatusReady {
		t.Errorf("expected dep to be ready after both blockers completed, got %s", status)
	}
}

// ============================================================================
// ResetTasks – status-based
// ============================================================================

func TestStore_ResetTasks_ByStatus(t *testing.T) {
	store, _ := setupTestDB(t)
	defer store.Close()

	t1, _ := store.CreateTask("T1", "", "", 0, nil)
	t2, _ := store.CreateTask("T2", "", "", 0, nil)
	t3, _ := store.CreateTask("T3", "", "", 0, nil)

	_ = store.UpdateTaskStatus(t1.ID, types.TaskStatusCompleted, "")
	_ = store.UpdateTaskStatus(t2.ID, types.TaskStatusFailed, "err")
	_ = store.UpdateTaskStatus(t3.ID, types.TaskStatusInProgress, "")

	// Reset only completed
	count, err := store.ResetTasks([]types.TaskStatus{types.TaskStatusCompleted})
	if err != nil {
		t.Fatalf("ResetTasks: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 reset, got %d", count)
	}

	s1, _ := store.GetTaskStatus(t1.ID)
	if s1 != types.TaskStatusReady {
		t.Errorf("expected t1 ready, got %s", s1)
	}
	s2, _ := store.GetTaskStatus(t2.ID)
	if s2 != types.TaskStatusFailed {
		t.Errorf("expected t2 still failed, got %s", s2)
	}
}

// ============================================================================
// ListTasks / ListTasksByEpic
// ============================================================================

func TestStore_ListTasks(t *testing.T) {
	store, _ := setupTestDB(t)
	defer store.Close()

	store.CreateTask("T1", "", "", 0, nil)
	store.CreateTask("T2", "", "", 0, nil)

	tasks, err := store.ListTasks()
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(tasks))
	}
}

func TestStore_ListTasksByEpic_Filtered(t *testing.T) {
	store, _ := setupTestDB(t)
	defer store.Close()

	epic, _ := store.CreateEpic("E1", "")
	store.CreateTask("T1", "", epic.ID, 0, nil)
	store.CreateTask("T2", "", epic.ID, 0, nil)
	store.CreateTask("T3", "", "", 0, nil) // no epic

	tasks, err := store.ListTasksByEpic(epic.ID)
	if err != nil {
		t.Fatalf("ListTasksByEpic: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 epic tasks, got %d", len(tasks))
	}
}

// ============================================================================
// ListEpics
// ============================================================================

func TestStore_ListEpics(t *testing.T) {
	store, _ := setupTestDB(t)
	defer store.Close()

	store.CreateEpic("E1", "desc1")
	store.CreateEpic("E2", "desc2")

	epics, err := store.ListEpics()
	if err != nil {
		t.Fatalf("ListEpics: %v", err)
	}
	if len(epics) != 2 {
		t.Errorf("expected 2 epics, got %d", len(epics))
	}
	if epics[0].Title != "E1" {
		t.Errorf("expected first epic title E1, got %s", epics[0].Title)
	}
}

// ============================================================================
// ListAllDependencies
// ============================================================================

func TestStore_ListAllDependencies(t *testing.T) {
	store, _ := setupTestDB(t)
	defer store.Close()

	b1, _ := store.CreateTask("B1", "", "", 0, nil)
	b2, _ := store.CreateTask("B2", "", "", 0, nil)
	store.CreateTask("D1", "", "", 0, []string{b1.ID, b2.ID})

	deps, err := store.ListAllDependencies()
	if err != nil {
		t.Fatalf("ListAllDependencies: %v", err)
	}
	if len(deps) != 2 {
		t.Errorf("expected 2 deps, got %d", len(deps))
	}
}

// ============================================================================
// ClaimTaskForEpic
// ============================================================================

func TestStore_ClaimTaskForEpic(t *testing.T) {
	store, _ := setupTestDB(t)
	defer store.Close()

	e1, _ := store.CreateEpic("E1", "")
	e2, _ := store.CreateEpic("E2", "")
	store.CreateTask("TE1", "", e1.ID, 0, nil)
	store.CreateTask("TE2", "", e2.ID, 0, nil)

	// Claim only from e1
	task, err := store.ClaimTaskForEpic("w1", e1.ID)
	if err != nil {
		t.Fatalf("ClaimTaskForEpic: %v", err)
	}
	if task == nil {
		t.Fatal("expected a task, got nil")
	}
	if task.EpicID != e1.ID {
		t.Errorf("expected epic %s, got %s", e1.ID, task.EpicID)
	}

	// Claim again from e1 – should be nil (only 1 task in e1)
	task2, _ := store.ClaimTaskForEpic("w1", e1.ID)
	if task2 != nil {
		t.Error("expected nil for second claim from e1")
	}
}

// ============================================================================
// MigrateSchema
// ============================================================================

func TestStore_MigrateSchema(t *testing.T) {
	store, _ := setupTestDB(t)
	defer store.Close()

	// MigrateSchema on an already-initialized DB should be idempotent
	if err := store.MigrateSchema(); err != nil {
		t.Fatalf("MigrateSchema: %v", err)
	}

	// Verify we can still create tasks with hierarchy columns
	task, err := store.CreateTask("T1", "", "", 0, nil)
	if err != nil {
		t.Fatalf("CreateTask after migrate: %v", err)
	}
	sub, err := store.CreateSubTask("S1", "", task.ID, 0, nil)
	if err != nil {
		t.Fatalf("CreateSubTask after migrate: %v", err)
	}
	if sub.ParentID != task.ID {
		t.Errorf("expected parentID %s, got %s", task.ID, sub.ParentID)
	}
}

// ============================================================================
// Worktree CRUD
// ============================================================================

func TestStore_Worktree_Lifecycle(t *testing.T) {
	store, _ := setupTestDB(t)
	defer store.Close()

	task, _ := store.CreateTask("WT Task", "", "", 0, nil)

	// Create worktree
	if err := store.CreateWorktree(task.ID, "/tmp/wt-1", "branch-1"); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}

	// List worktrees
	worktrees, err := store.ListWorktrees()
	if err != nil {
		t.Fatalf("ListWorktrees: %v", err)
	}
	if len(worktrees) != 1 {
		t.Fatalf("expected 1 worktree, got %d", len(worktrees))
	}
	if worktrees[0].TaskID != task.ID {
		t.Errorf("expected task_id %s, got %s", task.ID, worktrees[0].TaskID)
	}
	if worktrees[0].Branch != "branch-1" {
		t.Errorf("expected branch branch-1, got %s", worktrees[0].Branch)
	}
	if worktrees[0].Status != "active" {
		t.Errorf("expected status active, got %s", worktrees[0].Status)
	}

	// Update status
	if err := store.UpdateWorktreeStatus(task.ID, "merged"); err != nil {
		t.Fatalf("UpdateWorktreeStatus: %v", err)
	}

	// Update disk size
	if err := store.UpdateWorktreeDiskSize(task.ID, 12345); err != nil {
		t.Fatalf("UpdateWorktreeDiskSize: %v", err)
	}

	// Touch
	if err := store.TouchWorktree(task.ID); err != nil {
		t.Fatalf("TouchWorktree: %v", err)
	}

	// Verify updates via ListWorktrees
	wts, _ := store.ListWorktrees()
	if wts[0].Status != "merged" {
		t.Errorf("expected merged, got %s", wts[0].Status)
	}
	if wts[0].DiskSize != 12345 {
		t.Errorf("expected disk size 12345, got %d", wts[0].DiskSize)
	}

	// Delete worktree
	if err := store.DeleteWorktree(task.ID); err != nil {
		t.Fatalf("DeleteWorktree: %v", err)
	}
	wts2, _ := store.ListWorktrees()
	if len(wts2) != 0 {
		t.Errorf("expected 0 worktrees after delete, got %d", len(wts2))
	}
}

func TestStore_GetWorktreeStats(t *testing.T) {
	store, _ := setupTestDB(t)
	defer store.Close()

	t1, _ := store.CreateTask("T1", "", "", 0, nil)
	t2, _ := store.CreateTask("T2", "", "", 0, nil)
	store.CreateWorktree(t1.ID, "/tmp/wt1", "b1")
	store.CreateWorktree(t2.ID, "/tmp/wt2", "b2")
	store.UpdateWorktreeDiskSize(t1.ID, 100)
	store.UpdateWorktreeDiskSize(t2.ID, 200)

	stats, err := store.GetWorktreeStats()
	if err != nil {
		t.Fatalf("GetWorktreeStats: %v", err)
	}
	if stats["active_count"] != 2 {
		t.Errorf("expected 2 active worktrees, got %d", stats["active_count"])
	}
	if stats["active_size"] != 300 {
		t.Errorf("expected 300 total size, got %d", stats["active_size"])
	}
}

func TestStore_GetWorktreesForCleanup(t *testing.T) {
	store, _ := setupTestDB(t)
	defer store.Close()

	t1, _ := store.CreateTask("T1", "", "", 0, nil)
	t2, _ := store.CreateTask("T2", "", "", 0, nil)
	t3, _ := store.CreateTask("T3", "", "", 0, nil)

	store.CreateWorktree(t1.ID, "/tmp/wt1", "b1")
	store.CreateWorktree(t2.ID, "/tmp/wt2", "b2")
	store.CreateWorktree(t3.ID, "/tmp/wt3", "b3")

	// Complete t1, fail t2, leave t3 as ready
	store.UpdateTaskStatus(t1.ID, types.TaskStatusCompleted, "")
	store.UpdateTaskStatus(t2.ID, types.TaskStatusFailed, "err")

	// completedOnly=true should return t1 and t2
	cleanup, err := store.GetWorktreesForCleanup(true)
	if err != nil {
		t.Fatalf("GetWorktreesForCleanup(true): %v", err)
	}
	if len(cleanup) != 2 {
		t.Errorf("expected 2 worktrees for cleanup, got %d", len(cleanup))
	}

	// completedOnly=false should return all 3
	all, err := store.GetWorktreesForCleanup(false)
	if err != nil {
		t.Fatalf("GetWorktreesForCleanup(false): %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 worktrees, got %d", len(all))
	}

	// Mark one as removed – should be excluded
	store.UpdateWorktreeStatus(t3.ID, "removed")
	all2, _ := store.GetWorktreesForCleanup(false)
	if len(all2) != 2 {
		t.Errorf("expected 2 after removing, got %d", len(all2))
	}
}

func TestStore_GetOrphanedWorktrees(t *testing.T) {
	store, _ := setupTestDB(t)
	defer store.Close()

	// Create a task and worktree that is tracked
	task, _ := store.CreateTask("T1", "", "", 0, nil)
	store.CreateWorktree(task.ID, "/tmp/wt-tracked", "b1")

	// Create a fake worktree dir with tracked and orphaned entries
	wtDir := filepath.Join(t.TempDir(), "worktrees")
	os.MkdirAll(filepath.Join(wtDir, task.ID), 0755)         // tracked
	os.MkdirAll(filepath.Join(wtDir, "orphaned-task"), 0755) // orphaned
	os.WriteFile(filepath.Join(wtDir, "not-a-dir.txt"), []byte("x"), 0644) // file, skip

	orphaned, err := store.GetOrphanedWorktrees(wtDir)
	if err != nil {
		t.Fatalf("GetOrphanedWorktrees: %v", err)
	}
	if len(orphaned) != 1 {
		t.Fatalf("expected 1 orphaned, got %d", len(orphaned))
	}
	if filepath.Base(orphaned[0]) != "orphaned-task" {
		t.Errorf("expected orphaned-task, got %s", orphaned[0])
	}
}

func TestStore_GetOrphanedWorktrees_NonExistentDir(t *testing.T) {
	store, _ := setupTestDB(t)
	defer store.Close()

	orphaned, err := store.GetOrphanedWorktrees("/nonexistent/path")
	if err != nil {
		t.Fatalf("expected no error for non-existent dir, got: %v", err)
	}
	if len(orphaned) != 0 {
		t.Errorf("expected empty orphan list, got %d", len(orphaned))
	}
}

// ============================================================================
// GetTaskTree / GetParentTask edge cases
// ============================================================================

func TestStore_GetTaskTree(t *testing.T) {
	store, _ := setupTestDB(t)
	defer store.Close()

	parent, _ := store.CreateTask("Parent", "", "", 0, nil)
	store.CreateSubTask("Child 1", "", parent.ID, 0, nil)
	store.CreateSubTask("Child 2", "", parent.ID, 0, nil)

	tree, err := store.GetTaskTree(parent.ID)
	if err != nil {
		t.Fatalf("GetTaskTree: %v", err)
	}
	if tree.ID != parent.ID {
		t.Errorf("expected parent ID, got %s", tree.ID)
	}

	// Verify children can be fetched separately
	children, err := store.GetSubTasks(parent.ID)
	if err != nil {
		t.Fatalf("GetSubTasks: %v", err)
	}
	if len(children) != 2 {
		t.Errorf("expected 2 children, got %d", len(children))
	}
}

func TestStore_GetParentTask_NoParent(t *testing.T) {
	store, _ := setupTestDB(t)
	defer store.Close()

	task, _ := store.CreateTask("Root", "", "", 0, nil)
	_, err := store.GetParentTask(task.ID)
	if err == nil {
		t.Error("expected error for task with no parent")
	}
}

// ============================================================================
// RetryTask – additional scenarios
// ============================================================================

func TestStore_RetryTask_SetsReadyWhenNoDependencies(t *testing.T) {
	store, _ := setupTestDB(t)
	defer store.Close()

	task, _ := store.CreateTask("T1", "", "", 0, nil)
	store.UpdateTaskStatus(task.ID, types.TaskStatusFailed, "err")

	err := store.RetryTask(task.ID, false)
	if err != nil {
		t.Fatalf("RetryTask: %v", err)
	}
	// Verify status changed to ready
	status, _ := store.GetTaskStatus(task.ID)
	if status != types.TaskStatusReady {
		t.Errorf("expected ready, got %s", status)
	}
}

// NOTE: TestStore_RetryTask_CannotRetryCompleted and TestStore_RetryTask_ForceRetryCompleted removed —
// main's RetryTask only operates on failed/cancelled tasks; completed tasks are silently skipped.

// ============================================================================
// GetProjectStatus
// ============================================================================

func TestStore_GetProjectStatus_AllStatuses(t *testing.T) {
	store, _ := setupTestDB(t)
	defer store.Close()

	t1, _ := store.CreateTask("T1", "", "", 0, nil)
	t2, _ := store.CreateTask("T2", "", "", 0, nil)
	t3, _ := store.CreateTask("T3", "", "", 0, nil)
	store.CreateTask("T4", "", "", 0, nil)

	store.UpdateTaskStatus(t1.ID, types.TaskStatusCompleted, "")
	store.UpdateTaskStatus(t2.ID, types.TaskStatusFailed, "err")
	store.UpdateTaskStatus(t3.ID, types.TaskStatusInProgress, "")
	// t4 stays ready

	status, err := store.GetProjectStatus()
	if err != nil {
		t.Fatalf("GetProjectStatus: %v", err)
	}
	if status.Total != 4 {
		t.Errorf("expected total 4, got %d", status.Total)
	}
	if status.Ready != 1 {
		t.Errorf("expected 1 ready, got %d", status.Ready)
	}
	if status.Completed != 1 {
		t.Errorf("expected 1 completed, got %d", status.Completed)
	}
	if status.Failed != 1 {
		t.Errorf("expected 1 failed, got %d", status.Failed)
	}
	if status.InProgress != 1 {
		t.Errorf("expected 1 in_progress, got %d", status.InProgress)
	}
}

// ============================================================================
// CreateSubTaskWithSequence
// ============================================================================

func TestStore_CreateSubTaskWithSequence_DuplicateRejected(t *testing.T) {
	store, _ := setupTestDB(t)
	defer store.Close()

	parent, _ := store.CreateTask("P", "", "", 0, nil)
	_, err := store.CreateSubTaskWithSequence("S1", "", parent.ID, 5, 0, nil)
	if err != nil {
		t.Fatalf("first CreateSubTaskWithSequence: %v", err)
	}

	// Same sequence should fail
	_, err = store.CreateSubTaskWithSequence("S2", "", parent.ID, 5, 0, nil)
	if err == nil {
		t.Error("expected error for duplicate sequence number")
	}
}
