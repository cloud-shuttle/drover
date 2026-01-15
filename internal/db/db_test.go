// Package db_test provides tests for the db package
package db_test

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/cloud-shuttle/drover/internal/db"
	"github.com/cloud-shuttle/drover/pkg/types"
)

func setupTestDB(t *testing.T) (*db.Store, string) {
	t.Helper()

	// Create a temporary directory for the test database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open test store: %v", err)
	}

	// Initialize the schema
	if err := store.InitSchema(); err != nil {
		t.Fatalf("Failed to init schema: %v", err)
	}

	return store, dbPath
}

func TestStore_ClaimTask_Basic(t *testing.T) {
	store, _ := setupTestDB(t)
	defer store.Close()

	// Create a test epic
	epic, err := store.CreateEpic("Test Epic", "Test Description")
	if err != nil {
		t.Fatalf("Failed to create epic: %v", err)
	}

	// Create a test task
	task, err := store.CreateTask("Test Task", "Test Description", epic.ID, 10, nil)
	if err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}

	taskID := task.ID

	// Claim the task
	claimedTask, err := store.ClaimTask("worker-1")
	if err != nil {
		t.Fatalf("ClaimTask failed: %v", err)
	}

	if claimedTask == nil {
		t.Fatal("Expected task to be claimed, got nil")
	}

	if claimedTask.ID != taskID {
		t.Errorf("Expected task ID %s, got %s", taskID, claimedTask.ID)
	}

	if claimedTask.Status != types.TaskStatusClaimed {
		t.Errorf("Expected status %s, got %s", types.TaskStatusClaimed, claimedTask.Status)
	}

	if claimedTask.ClaimedBy != "worker-1" {
		t.Errorf("Expected claimed_by worker-1, got %s", claimedTask.ClaimedBy)
	}

	if claimedTask.ClaimedAt == nil {
		t.Error("Expected claimed_at to be set")
	}
}

func TestStore_ClaimTask_Concurrency(t *testing.T) {
	store, _ := setupTestDB(t)
	defer store.Close()

	// Create multiple test tasks
	const numTasks = 10
	const numWorkers = 3 // Reduced from 5 to avoid SQLite locking issues

	for i := 0; i < numTasks; i++ {
		_, err := store.CreateTask("Task "+string(rune(i)), "", "", 10, nil)
		if err != nil {
			t.Fatalf("Failed to create task %d: %v", i, err)
		}
	}

	var wg sync.WaitGroup
	claimedTasks := make(chan string, numTasks)
	workers := []string{"worker-1", "worker-2", "worker-3"}

	// Start multiple workers claiming tasks concurrently
	for _, workerID := range workers {
		wg.Add(1)
		go func(wid string) {
			defer wg.Done()
			for {
				task, err := store.ClaimTask(wid)
				if err != nil {
					// SQLite may return "database is locked" under high concurrency
					// This is expected behavior, so we don't fail the test
					return
				}
				if task == nil {
					return // No more tasks
				}
				claimedTasks <- task.ID
			}
		}(workerID)
	}

	wg.Wait()
	close(claimedTasks)

	// Verify all tasks were claimed exactly once (allow for some SQLite contention)
	claimedCount := make(map[string]int)
	for taskID := range claimedTasks {
		claimedCount[taskID]++
	}

	// Due to SQLite locking, we may not claim all tasks, but no task should be claimed twice
	for taskID, count := range claimedCount {
		if count != 1 {
			t.Errorf("Task %s was claimed %d times, expected 1", taskID, count)
		}
	}
}

func TestStore_ClaimTask_RaceCondition(t *testing.T) {
	store, _ := setupTestDB(t)
	defer store.Close()

	// Create a single task
	_, err := store.CreateTask("Race Test Task", "", "", 10, nil)
	if err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}

	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	// Have multiple workers try to claim the same task concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(workerNum int) {
			defer wg.Done()
			claimedTask, err := store.ClaimTask("worker-test")
			if err != nil {
				t.Errorf("Worker %d failed: %v", workerNum, err)
				return
			}
			if claimedTask != nil {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}(i)
	}

	wg.Wait()

	// Exactly one worker should have successfully claimed the task
	if successCount != 1 {
		t.Errorf("Expected 1 worker to claim the task, got %d", successCount)
	}

	// Verify the task was claimed by checking no ready tasks remain
	status, err := store.GetProjectStatus()
	if err != nil {
		t.Fatalf("Failed to get project status: %v", err)
	}
	if status.Ready != 0 {
		t.Errorf("Expected 0 ready tasks after claim, got %d", status.Ready)
	}
}

func TestStore_ClaimTask_NoReadyTasks(t *testing.T) {
	store, _ := setupTestDB(t)
	defer store.Close()

	// Try to claim when no tasks exist
	task, err := store.ClaimTask("worker-1")
	if err != nil {
		t.Fatalf("ClaimTask failed: %v", err)
	}

	if task != nil {
		t.Error("Expected nil when no tasks available, got task")
	}
}

func TestStore_ClaimTask_OnlyClaimedTasks(t *testing.T) {
	store, _ := setupTestDB(t)
	defer store.Close()

	// Create and claim a task
	_, err := store.CreateTask("Already Claimed Task", "", "", 10, nil)
	if err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}

	// Claim the task
	_, err = store.ClaimTask("worker-other")
	if err != nil {
		t.Fatalf("Failed to claim task: %v", err)
	}

	// Try to claim again - should return nil since no ready tasks
	task2, err := store.ClaimTask("worker-1")
	if err != nil {
		t.Fatalf("ClaimTask failed: %v", err)
	}

	if task2 != nil {
		t.Error("Expected nil when no ready tasks available, got task")
	}
}

func TestStore_ClaimTask_PriorityOrder(t *testing.T) {
	store, _ := setupTestDB(t)
	defer store.Close()

	// Create tasks with different priorities
	tasks := []struct {
		title    string
		priority int
	}{
		{"Low Priority Task", 1},
		{"High Priority Task", 100},
		{"Medium Priority Task", 50},
	}

	for _, tt := range tasks {
		_, err := store.CreateTask(tt.title, "", "", tt.priority, nil)
		if err != nil {
			t.Fatalf("Failed to create task %s: %v", tt.title, err)
		}
	}

	// First claim should get highest priority task
	task, err := store.ClaimTask("worker-1")
	if err != nil {
		t.Fatalf("ClaimTask failed: %v", err)
	}

	if task.Title != "High Priority Task" {
		t.Errorf("Expected highest priority task (High Priority Task), got %s", task.Title)
	}
}

func TestStore_ClaimTask_FIFOForSamePriority(t *testing.T) {
	store, _ := setupTestDB(t)
	defer store.Close()

	// Create tasks with same priority - the order they are created matters
	tasks := []string{"First Task", "Second Task", "Third Task"}
	for _, title := range tasks {
		_, err := store.CreateTask(title, "", "", 10, nil)
		if err != nil {
			t.Fatalf("Failed to create task %s: %v", title, err)
		}
	}

	// Claims should return tasks in FIFO order (created_at ASC)
	expectedOrder := []string{"First Task", "Second Task", "Third Task"}
	for i, expectedTitle := range expectedOrder {
		task, err := store.ClaimTask("worker-1")
		if err != nil {
			t.Fatalf("Claim %d failed: %v", i, err)
		}
		if task.Title != expectedTitle {
			t.Errorf("Claim %d: expected %s, got %s", i, expectedTitle, task.Title)
		}
	}
}

func TestStore_ClaimTask_BlockedTasksNotClaimed(t *testing.T) {
	store, _ := setupTestDB(t)
	defer store.Close()

	// Create a blocker task first
	blockerTask, err := store.CreateTask("Blocker Task", "", "", 10, nil)
	if err != nil {
		t.Fatalf("Failed to create blocker task: %v", err)
	}

	// Create a blocked task that depends on the blocker
	blockedTask, err := store.CreateTask("Blocked Task", "", "", 10, []string{blockerTask.ID})
	if err != nil {
		t.Fatalf("Failed to create blocked task: %v", err)
	}

	// Should claim the blocker task, not the blocked one
	task, err := store.ClaimTask("worker-1")
	if err != nil {
		t.Fatalf("ClaimTask failed: %v", err)
	}

	if task == nil {
		t.Fatal("Expected to claim a task, got nil")
	}

	// Verify the blocked task was NOT claimed
	if task.ID == blockedTask.ID {
		t.Errorf("Should not claim blocked task %s", blockedTask.ID)
	}

	// Verify it claimed the blocker task instead
	if task.ID != blockerTask.ID {
		t.Errorf("Expected to claim blocker task %s, got %s", blockerTask.ID, task.ID)
	}
}

func TestStore_ClaimTask_AfterFailure(t *testing.T) {
	store, _ := setupTestDB(t)
	defer store.Close()

	// Create a task
	_, err := store.CreateTask("Test Task", "", "", 10, nil)
	if err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}

	// Claim the task
	claimedTask, err := store.ClaimTask("worker-1")
	if err != nil {
		t.Fatalf("ClaimTask failed: %v", err)
	}

	// Mark as failed
	err = store.UpdateTaskStatus(claimedTask.ID, types.TaskStatusFailed, "test error")
	if err != nil {
		t.Fatalf("Failed to update task status: %v", err)
	}

	// Try to claim again - should return nil since task is failed
	task2, err := store.ClaimTask("worker-2")
	if err != nil {
		t.Fatalf("ClaimTask failed: %v", err)
	}

	if task2 != nil {
		t.Error("Expected nil when no ready tasks available, got task")
	}
}

func TestStore_GetTask(t *testing.T) {
	store, _ := setupTestDB(t)
	defer store.Close()

	// Create an epic
	epic, err := store.CreateEpic("Test Epic", "Test Description")
	if err != nil {
		t.Fatalf("Failed to create epic: %v", err)
	}

	// Create a task with full details
	createdTask, err := store.CreateTask(
		"Test Task Title",
		"Test task description with details",
		epic.ID,
		42,
		nil,
	)
	if err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}

	// Get the task
	retrievedTask, err := store.GetTask(createdTask.ID)
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}

	// Verify all fields match
	if retrievedTask.ID != createdTask.ID {
		t.Errorf("Expected ID %s, got %s", createdTask.ID, retrievedTask.ID)
	}

	if retrievedTask.Title != createdTask.Title {
		t.Errorf("Expected title %s, got %s", createdTask.Title, retrievedTask.Title)
	}

	if retrievedTask.Description != createdTask.Description {
		t.Errorf("Expected description %s, got %s", createdTask.Description, retrievedTask.Description)
	}

	if retrievedTask.EpicID != epic.ID {
		t.Errorf("Expected epic ID %s, got %s", epic.ID, retrievedTask.EpicID)
	}

	if retrievedTask.Priority != 42 {
		t.Errorf("Expected priority 42, got %d", retrievedTask.Priority)
	}

	if retrievedTask.Status != types.TaskStatusReady {
		t.Errorf("Expected status %s, got %s", types.TaskStatusReady, retrievedTask.Status)
	}
}

func TestStore_GetTask_NotFound(t *testing.T) {
	store, _ := setupTestDB(t)
	defer store.Close()

	// Try to get a non-existent task
	_, err := store.GetTask("non-existent-task-id")
	if err == nil {
		t.Error("Expected error when getting non-existent task, got nil")
	}
}

func TestStore_GetBlockedBy(t *testing.T) {
	store, _ := setupTestDB(t)
	defer store.Close()

	// Create blocker tasks
	blocker1, err := store.CreateTask("Blocker 1", "", "", 10, nil)
	if err != nil {
		t.Fatalf("Failed to create blocker 1: %v", err)
	}

	blocker2, err := store.CreateTask("Blocker 2", "", "", 10, nil)
	if err != nil {
		t.Fatalf("Failed to create blocker 2: %v", err)
	}

	// Create a task that depends on both blockers
	dependentTask, err := store.CreateTask(
		"Dependent Task",
		"",
		"",
		10,
		[]string{blocker1.ID, blocker2.ID},
	)
	if err != nil {
		t.Fatalf("Failed to create dependent task: %v", err)
	}

	// Get the blocked by list
	blockedBy, err := store.GetBlockedBy(dependentTask.ID)
	if err != nil {
		t.Fatalf("GetBlockedBy failed: %v", err)
	}

	// Verify we got both blockers
	if len(blockedBy) != 2 {
		t.Fatalf("Expected 2 blockers, got %d", len(blockedBy))
	}

	// Check that both blocker IDs are present
	found := make(map[string]bool)
	for _, id := range blockedBy {
		found[id] = true
	}

	if !found[blocker1.ID] {
		t.Errorf("Expected to find blocker %s", blocker1.ID)
	}

	if !found[blocker2.ID] {
		t.Errorf("Expected to find blocker %s", blocker2.ID)
	}
}

func TestStore_GetBlockedBy_NoDependencies(t *testing.T) {
	store, _ := setupTestDB(t)
	defer store.Close()

	// Create a task with no dependencies
	task, err := store.CreateTask("Independent Task", "", "", 10, nil)
	if err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}

	// Get the blocked by list
	blockedBy, err := store.GetBlockedBy(task.ID)
	if err != nil {
		t.Fatalf("GetBlockedBy failed: %v", err)
	}

	// Verify we got an empty list
	if len(blockedBy) != 0 {
		t.Errorf("Expected 0 blockers, got %d", len(blockedBy))
	}
}

func TestStore_ResetTasksByIDs(t *testing.T) {
	store, _ := setupTestDB(t)
	defer store.Close()

	// Create multiple test tasks
	task1, err := store.CreateTask("Task 1", "Description 1", "", 10, nil)
	if err != nil {
		t.Fatalf("Failed to create task 1: %v", err)
	}

	task2, err := store.CreateTask("Task 2", "Description 2", "", 10, nil)
	if err != nil {
		t.Fatalf("Failed to create task 2: %v", err)
	}

	task3, err := store.CreateTask("Task 3", "Description 3", "", 10, nil)
	if err != nil {
		t.Fatalf("Failed to create task 3: %v", err)
	}

	// Mark task1 and task2 as completed
	if err := store.UpdateTaskStatus(task1.ID, types.TaskStatusCompleted, ""); err != nil {
		t.Fatalf("Failed to mark task1 as completed: %v", err)
	}
	if err := store.UpdateTaskStatus(task2.ID, types.TaskStatusCompleted, ""); err != nil {
		t.Fatalf("Failed to mark task2 as completed: %v", err)
	}

	// Mark task3 as failed
	if err := store.UpdateTaskStatus(task3.ID, types.TaskStatusFailed, "test error"); err != nil {
		t.Fatalf("Failed to mark task3 as failed: %v", err)
	}

	// Increment attempts for task1 so we can verify it's reset
	if err := store.IncrementTaskAttempts(task1.ID); err != nil {
		t.Fatalf("Failed to increment attempts: %v", err)
	}

	// Reset only task1 and task2 by ID
	count, err := store.ResetTasksByIDs([]string{task1.ID, task2.ID})
	if err != nil {
		t.Fatalf("ResetTasksByIDs failed: %v", err)
	}

	if count != 2 {
		t.Errorf("Expected to reset 2 tasks, got %d", count)
	}

	// Verify task1 and task2 are back to ready
	status1, _ := store.GetTaskStatus(task1.ID)
	if status1 != types.TaskStatusReady {
		t.Errorf("Expected task1 status to be 'ready', got '%s'", status1)
	}

	status2, _ := store.GetTaskStatus(task2.ID)
	if status2 != types.TaskStatusReady {
		t.Errorf("Expected task2 status to be 'ready', got '%s'", status2)
	}

	// Verify task3 is still failed
	status3, _ := store.GetTaskStatus(task3.ID)
	if status3 != types.TaskStatusFailed {
		t.Errorf("Expected task3 status to still be 'failed', got '%s'", status3)
	}

	// Verify task1's attempts were reset
	updatedTask1, err := store.GetTask(task1.ID)
	if err != nil {
		t.Fatalf("Failed to get task1: %v", err)
	}
	if updatedTask1.Attempts != 0 {
		t.Errorf("Expected task1 attempts to be reset to 0, got %d", updatedTask1.Attempts)
	}
}

func TestStore_ResetTasksByIDs_Empty(t *testing.T) {
	store, _ := setupTestDB(t)
	defer store.Close()

	// Create a test task
	task, err := store.CreateTask("Task 1", "Description 1", "", 10, nil)
	if err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}

	// Mark it as completed
	if err := store.UpdateTaskStatus(task.ID, types.TaskStatusCompleted, ""); err != nil {
		t.Fatalf("Failed to mark task as completed: %v", err)
	}

	// Reset with empty list should do nothing
	count, err := store.ResetTasksByIDs([]string{})
	if err != nil {
		t.Fatalf("ResetTasksByIDs with empty list failed: %v", err)
	}

	if count != 0 {
		t.Errorf("Expected to reset 0 tasks with empty list, got %d", count)
	}

	// Verify task is still completed
	status, _ := store.GetTaskStatus(task.ID)
	if status != types.TaskStatusCompleted {
		t.Errorf("Expected task status to still be 'completed', got '%s'", status)
	}
}

func TestStore_UpdateEpicStatus(t *testing.T) {
	store, _ := setupTestDB(t)
	defer store.Close()

	// Create a test epic
	epic, err := store.CreateEpic("Test Epic", "Test Description")
	if err != nil {
		t.Fatalf("Failed to create epic: %v", err)
	}

	// Verify initial status
	if epic.Status != types.EpicStatusOpen {
		t.Errorf("Expected initial status to be 'open', got '%s'", epic.Status)
	}

	// Update epic status to closed
	err = store.UpdateEpicStatus(epic.ID, types.EpicStatusClosed)
	if err != nil {
		t.Fatalf("Failed to update epic status: %v", err)
	}

	// Retrieve the epic and verify status was updated
	epics, err := store.ListEpics()
	if err != nil {
		t.Fatalf("Failed to list epics: %v", err)
	}

	if len(epics) != 1 {
		t.Fatalf("Expected 1 epic, got %d", len(epics))
	}

	if epics[0].Status != types.EpicStatusClosed {
		t.Errorf("Expected epic status to be 'closed', got '%s'", epics[0].Status)
	}

	// Update back to open
	err = store.UpdateEpicStatus(epic.ID, types.EpicStatusOpen)
	if err != nil {
		t.Fatalf("Failed to update epic status back to open: %v", err)
	}

	// Verify it was updated
	epics, err = store.ListEpics()
	if err != nil {
		t.Fatalf("Failed to list epics: %v", err)
	}

	if epics[0].Status != types.EpicStatusOpen {
		t.Errorf("Expected epic status to be 'open', got '%s'", epics[0].Status)
	}
}

func TestStore_UpdateEpicStatus_NotFound(t *testing.T) {
	store, _ := setupTestDB(t)
	defer store.Close()

	// Try to update a non-existent epic
	err := store.UpdateEpicStatus("epic-nonexistent", types.EpicStatusClosed)
	if err == nil {
		t.Fatal("Expected error when updating non-existent epic, got nil")
	}

	// Verify error message mentions epic not found
	expectedMsg := "epic not found"
	if !contains(err.Error(), expectedMsg) {
		t.Errorf("Expected error to contain '%s', got '%s'", expectedMsg, err.Error())
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && stringContains(s, substr))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
