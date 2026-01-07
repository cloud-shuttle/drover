// Package workflow_test provides integration tests for the workflow package
package workflow_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/cloud-shuttle/drover/internal/config"
	"github.com/cloud-shuttle/drover/internal/db"
	"github.com/cloud-shuttle/drover/internal/workflow"
)

// setupTestWorkflow creates a complete test environment for workflow integration tests
func setupTestWorkflow(t *testing.T) (string, *db.Store, *workflow.Orchestrator, func()) {
	t.Helper()

	// Create temp directory
	tmpDir := t.TempDir()

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Configure git
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to set git email: %v", err)
	}

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to set git name: %v", err)
	}

	// Create initial commit
	initialFile := filepath.Join(tmpDir, "README.md")
	if err := os.WriteFile(initialFile, []byte("# Test Repo\n"), 0644); err != nil {
		t.Fatalf("Failed to create initial file: %v", err)
	}

	cmd = exec.Command("git", "add", "README.md")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to add initial file: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create initial commit: %v", err)
	}

	// Rename branch to main
	cmd = exec.Command("git", "branch", "-M", "main")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to rename branch to main: %v", err)
	}

	// Create database
	dbPath := filepath.Join(tmpDir, ".drover", "drover.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		t.Fatalf("Failed to create db directory: %v", err)
	}

	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open db: %v", err)
	}

	if err := store.InitSchema(); err != nil {
		t.Fatalf("Failed to init schema: %v", err)
	}

	// Create a mock claude script that actually makes file changes
	mockClaude := filepath.Join(tmpDir, "mock-claude.sh")
	scriptContent := `#!/bin/bash
# Mock Claude that creates a file based on the task prompt
# Handle --version flag for Claude check
if [ "$1" = "--version" ]; then
	echo "claude-mock version 1.0.0"
	exit 0
fi

# Extract prompt from -p flag (Drover passes prompt this way)
PROMPT=""
while [[ $# -gt 0 ]]; do
	case $1 in
		-p)
			PROMPT="$2"
			shift 2
			;;
		*)
			shift
			;;
	esac
done

# Create a unique file based on prompt hash
FILE_HASH=$(echo "$PROMPT" | md5sum | cut -c1-8)
echo "Work done for: $PROMPT" > "work-$FILE_HASH.txt"
exit 0
`
	if err := os.WriteFile(mockClaude, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to create mock claude: %v", err)
	}

	// Create config
	cfg := &config.Config{
		ClaudePath:   mockClaude,
		TaskTimeout:  5 * time.Second,
		Workers:      1,
		WorktreeDir:  filepath.Join(tmpDir, ".drover", "worktrees"),
		PollInterval: 100 * time.Millisecond,
		Verbose:      true,
	}

	// Create orchestrator
	orch, err := workflow.NewOrchestrator(cfg, store, tmpDir)
	if err != nil {
		store.Close()
		t.Fatalf("Failed to create orchestrator: %v", err)
	}

	cleanup := func() {
		store.Close()
	}

	return tmpDir, store, orch, cleanup
}

// TestOrchestrator_SingleTask verifies a single task can be executed end-to-end
func TestOrchestrator_SingleTask(t *testing.T) {
	_, store, orch, cleanup := setupTestWorkflow(t)
	defer cleanup()

	// Create a task
	task, err := store.CreateTask("Test Task", "Do some work", "", 10, nil)
	if err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}

	// Run the orchestrator with a timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Run in a goroutine since it runs until all tasks are complete
	errChan := make(chan error, 1)
	go func() {
		errChan <- orch.Run(ctx)
	}()

	// Wait for task to complete or timeout
	select {
	case err := <-errChan:
		if err != nil && err != context.Canceled && err != context.DeadlineExceeded {
			t.Errorf("Orchestrator failed: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Test timed out waiting for task completion")
	}

	// Verify task was marked as completed
	status, err := store.GetTaskStatus(task.ID)
	if err != nil {
		t.Fatalf("Failed to get task status: %v", err)
	}

	if status != "completed" {
		t.Errorf("Expected task status 'completed', got '%s'", status)
	}

	// Verify the work was done (file was created)
	// This is a bit tricky since the worktree is cleaned up, but we can check git log
}

// TestOrchestrator_MultipleTasks verifies multiple tasks are processed
func TestOrchestrator_MultipleTasks(t *testing.T) {
	_, store, orch, cleanup := setupTestWorkflow(t)
	defer cleanup()

	// Create multiple tasks
	const numTasks = 3
	for i := 0; i < numTasks; i++ {
		_, err := store.CreateTask(fmt.Sprintf("Task %d", i), fmt.Sprintf("Do work %d", i), "", 10, nil)
		if err != nil {
			t.Fatalf("Failed to create task %d: %v", i, err)
		}
	}

	// Run the orchestrator
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	errChan := make(chan error, 1)
	go func() {
		errChan <- orch.Run(ctx)
	}()

	// Wait for completion
	select {
	case err := <-errChan:
		if err != nil && err != context.Canceled && err != context.DeadlineExceeded {
			t.Errorf("Orchestrator failed: %v", err)
		}
	case <-time.After(35 * time.Second):
		t.Fatal("Test timed out")
	}

	// Verify all tasks were completed
	projectStatus, err := store.GetProjectStatus()
	if err != nil {
		t.Fatalf("Failed to get project status: %v", err)
	}

	if projectStatus.Completed != numTasks {
		t.Errorf("Expected %d completed tasks, got %d", numTasks, projectStatus.Completed)
	}
}

// TestOrchestrator_DependentTasks verifies dependent tasks are processed in order
func TestOrchestrator_DependentTasks(t *testing.T) {
	_, store, orch, cleanup := setupTestWorkflow(t)
	defer cleanup()

	// Create tasks with dependencies
	task1, err := store.CreateTask("First Task", "Do first work", "", 10, nil)
	if err != nil {
		t.Fatalf("Failed to create first task: %v", err)
	}

	_, err = store.CreateTask("Second Task", "Do second work", "", 10, []string{task1.ID})
	if err != nil {
		t.Fatalf("Failed to create second task: %v", err)
	}

	// Run the orchestrator
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	errChan := make(chan error, 1)
	go func() {
		errChan <- orch.Run(ctx)
	}()

	// Wait for completion
	select {
	case err := <-errChan:
		if err != nil && err != context.Canceled && err != context.DeadlineExceeded {
			t.Errorf("Orchestrator failed: %v", err)
		}
	case <-time.After(25 * time.Second):
		t.Fatal("Test timed out")
	}

	// Verify both tasks were completed
	projectStatus, err := store.GetProjectStatus()
	if err != nil {
		t.Fatalf("Failed to get project status: %v", err)
	}

	if projectStatus.Completed != 2 {
		t.Errorf("Expected 2 completed tasks, got %d", projectStatus.Completed)
	}

	// Verify task2 was blocked initially (we can't easily test this without querying history)
}

// TestOrchestrator_TaskFailure verifies failed tasks are handled correctly
func TestOrchestrator_TaskFailure(t *testing.T) {
	tmpDir, store, _, cleanup := setupTestWorkflow(t)
	defer cleanup()

	// Create a mock claude that fails
	mockClaude := filepath.Join(tmpDir, "mock-claude-fail.sh")
	scriptContent := `#!/bin/bash
# Mock Claude that always fails (except for version check)
if [ "$1" = "--version" ]; then
	echo "claude-mock-fail version 1.0.0"
	exit 0
fi
echo "Claude error" >&2
exit 1
`
	if err := os.WriteFile(mockClaude, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to create mock claude: %v", err)
	}

	// Create config with failing claude
	cfg := &config.Config{
		ClaudePath:   mockClaude,
		TaskTimeout:  5 * time.Second,
		Workers:      1,
		WorktreeDir:  filepath.Join(tmpDir, ".drover", "worktrees"),
		PollInterval: 100 * time.Millisecond,
		Verbose:      true,
	}

	orch, err := workflow.NewOrchestrator(cfg, store, tmpDir)
	if err != nil {
		t.Fatalf("Failed to create orchestrator: %v", err)
	}

	// Create a task
	task, err := store.CreateTask("Failing Task", "This will fail", "", 10, nil)
	if err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}

	// Run the orchestrator
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	errChan := make(chan error, 1)
	go func() {
		errChan <- orch.Run(ctx)
	}()

	// Wait for completion or timeout
	select {
	case <-errChan:
		// Expected to complete (with failed task)
	case <-time.After(25 * time.Second):
		t.Fatal("Test timed out")
	}

	// Verify task was marked as failed
	status, err := store.GetTaskStatus(task.ID)
	if err != nil {
		t.Fatalf("Failed to get task status: %v", err)
	}

	if status != "failed" {
		t.Errorf("Expected task status 'failed', got '%s'", status)
	}

	// Verify the task was attempted
	projectStatus, err := store.GetProjectStatus()
	if err != nil {
		t.Fatalf("Failed to get project status: %v", err)
	}

	if projectStatus.Failed != 1 {
		t.Errorf("Expected 1 failed task, got %d", projectStatus.Failed)
	}
}

// TestOrchestrator_Retry verifies failed tasks are retried
func TestOrchestrator_Retry(t *testing.T) {
	tmpDir, store, _, cleanup := setupTestWorkflow(t)
	defer cleanup()

	// Track attempts
	attemptFile := filepath.Join(tmpDir, "attempts.txt")

	// Create a mock claude that fails twice then succeeds
	mockClaude := filepath.Join(tmpDir, "mock-claude-retry.sh")
	scriptContent := fmt.Sprintf(`#!/bin/bash
# Mock Claude that fails twice then succeeds
if [ "$1" = "--version" ]; then
	echo "claude-mock-retry version 1.0.0"
	exit 0
fi
if [ ! -f "%s" ]; then
	echo "1" > "%s"
	echo "First attempt, failing" >&2
	exit 1
fi
ATTEMPTS=$(cat "%s")
if [ "$ATTEMPTS" -lt 3 ]; then
	echo $((ATTEMPTS + 1)) > "%s"
	echo "Attempt $ATTEMPTS, failing" >&2
	exit 1
fi
echo "Success on attempt $ATTEMPTS"
exit 0
`, attemptFile, attemptFile, attemptFile, attemptFile)

	if err := os.WriteFile(mockClaude, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to create mock claude: %v", err)
	}

	// Create config
	cfg := &config.Config{
		ClaudePath:   mockClaude,
		TaskTimeout:  5 * time.Second,
		Workers:      1,
		WorktreeDir:  filepath.Join(tmpDir, ".drover", "worktrees"),
		PollInterval: 100 * time.Millisecond,
		Verbose:      true,
	}

	orch, err := workflow.NewOrchestrator(cfg, store, tmpDir)
	if err != nil {
		t.Fatalf("Failed to create orchestrator: %v", err)
	}

	// Create a task
	task, err := store.CreateTask("Retry Task", "This will retry", "", 10, nil)
	if err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}

	// Run the orchestrator
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	errChan := make(chan error, 1)
	go func() {
		errChan <- orch.Run(ctx)
	}()

	// Wait for completion
	select {
	case err := <-errChan:
		if err != nil && err != context.Canceled && err != context.DeadlineExceeded {
			t.Errorf("Orchestrator failed: %v", err)
		}
	case <-time.After(65 * time.Second):
		t.Fatal("Test timed out")
	}

	// Verify task was eventually completed
	status, err := store.GetTaskStatus(task.ID)
	if err != nil {
		t.Fatalf("Failed to get task status: %v", err)
	}

	if status != "completed" {
		t.Errorf("Expected task status 'completed' after retries, got '%s'", status)
	}
}
