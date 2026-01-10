// Package db tests for sub-task workflow execution
package db

import (
	"path/filepath"
	"testing"
)

// TestSubTaskWorkflow tests the complete sub-task execution workflow
// This simulates what happens when a parent task with sub-tasks is executed
func TestSubTaskWorkflow(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer store.Close()

	if err := store.InitSchema(); err != nil {
		t.Fatalf("Failed to initialize schema: %v", err)
	}

	// 1. Create a parent task
	parent, err := store.CreateTask("Parent task", "Parent description", "", 0, nil)
	if err != nil {
		t.Fatalf("Failed to create parent task: %v", err)
	}

	// 2. Create sub-tasks
	subTask1, _ := store.CreateSubTask("Sub task 1", "Desc 1", parent.ID, 0, nil)
	_, _ = store.CreateSubTask("Sub task 2", "Desc 2", parent.ID, 0, nil)
	_, _ = store.CreateSubTask("Sub task 3", "Desc 3", parent.ID, 0, nil)

	// 3. Verify HasSubTasks
	hasChildren, err := store.HasSubTasks(parent.ID)
	if err != nil {
		t.Fatalf("HasSubTasks failed: %v", err)
	}
	if !hasChildren {
		t.Error("Expected HasSubTasks to return true")
	}

	// 4. Verify GetSubTasks returns all sub-tasks in order
	subTasks, err := store.GetSubTasks(parent.ID)
	if err != nil {
		t.Fatalf("GetSubTasks failed: %v", err)
	}
	if len(subTasks) != 3 {
		t.Errorf("Expected 3 sub-tasks, got %d", len(subTasks))
	}
	if subTasks[0].ID != subTask1.ID {
		t.Errorf("Expected first sub-task %s, got %s", subTask1.ID, subTasks[0].ID)
	}

	// 5. Verify ClaimTask only claims the parent (not sub-tasks)
	claimed, err := store.ClaimTask("test-worker")
	if err != nil {
		t.Fatalf("ClaimTask failed: %v", err)
	}
	if claimed == nil {
		t.Fatal("Expected to claim a task")
	}
	if claimed.ID != parent.ID {
		t.Errorf("Expected to claim parent %s, got %s", parent.ID, claimed.ID)
	}
	if claimed.ParentID != "" {
		t.Errorf("Claimed task should have empty ParentID, got %s", claimed.ParentID)
	}

	// 6. Verify sub-tasks are still in 'ready' status (not claimed)
	for _, st := range subTasks {
		status, err := store.GetTaskStatus(st.ID)
		if err != nil {
			t.Fatalf("Failed to get sub-task status: %v", err)
		}
		if status != "ready" {
			t.Errorf("Sub-task %s should still be 'ready', got '%s'", st.ID, status)
		}
	}

	// 7. Verify we cannot claim sub-tasks directly (no more claimable tasks)
	secondClaim, err := store.ClaimTask("test-worker-2")
	if err != nil {
		t.Fatalf("Second ClaimTask failed: %v", err)
	}
	if secondClaim != nil {
		t.Errorf("Expected no more claimable tasks, got %s", secondClaim.ID)
	}
}

// TestSubTaskClaimingBehavior tests that sub-tasks are never claimed independently
func TestSubTaskClaimingBehavior(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer store.Close()

	if err := store.InitSchema(); err != nil {
		t.Fatalf("Failed to initialize schema: %v", err)
	}

	// Create a mix of parent tasks and sub-tasks
	parent1, _ := store.CreateTask("Parent 1", "", "", 0, nil)
	parent2, _ := store.CreateTask("Parent 2", "", "", 0, nil)
	_, _ = store.CreateSubTask("Sub 1.1", "", parent1.ID, 0, nil)
	_, _ = store.CreateSubTask("Sub 1.2", "", parent1.ID, 0, nil)
	_, _ = store.CreateSubTask("Sub 2.1", "", parent2.ID, 0, nil)

	// Try to claim multiple times - should only get parent tasks
	claimedIDs := make(map[string]bool)
	for i := 0; i < 5; i++ {
		task, err := store.ClaimTask("worker")
		if err != nil {
			t.Fatalf("ClaimTask failed: %v", err)
		}
		if task == nil {
			break // No more tasks
		}
		if task.ParentID != "" {
			t.Errorf("Task %s has ParentID=%s (should be empty)", task.ID, task.ParentID)
		}
		claimedIDs[task.ID] = true
	}

	// Verify we only claimed the 2 parent tasks
	if len(claimedIDs) != 2 {
		t.Errorf("Expected to claim 2 parent tasks, claimed %d: %v", len(claimedIDs), claimedIDs)
	}
	if !claimedIDs[parent1.ID] {
		t.Errorf("Parent task %s was not claimed", parent1.ID)
	}
	if !claimedIDs[parent2.ID] {
		t.Errorf("Parent task %s was not claimed", parent2.ID)
	}
}
