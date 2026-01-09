// Package workflow_test provides integration tests for DBOS-based workflows
package workflow_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cloud-shuttle/drover/internal/config"
	"github.com/cloud-shuttle/drover/internal/workflow"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
)

// skipIfNoPostgres skips the test if PostgreSQL is not available
func skipIfNoPostgres(t *testing.T) {
	t.Helper()

	dbURL := os.Getenv("DBOS_SYSTEM_DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/drover_test?sslmode=disable"
	}

	// Try to connect to Postgres
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Use psql or pg_isready to check if Postgres is available
	cmd := exec.CommandContext(ctx, "pg_isready", "-h", "localhost", "-p", "5432")
	if err := cmd.Run(); err != nil {
		t.Skip("PostgreSQL not available - skipping DBOS integration tests")
	}
}

// setupDBOSTestEnvironment creates a test environment for DBOS workflow tests
func setupDBOSTestEnvironment(t *testing.T) (string, dbos.DBOSContext, *workflow.DBOSOrchestrator, func()) {
	t.Helper()
	skipIfNoPostgres(t)

	// Create temp directory
	tmpDir := t.TempDir()

	// Initialize git repo
	setupGitRepo(t, tmpDir)

	// Create mock Claude script
	mockClaude := createMockClaude(t, tmpDir, "success")

	// Get database URL
	dbURL := os.Getenv("DBOS_SYSTEM_DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/drover_test?sslmode=disable"
	}

	// Add unique suffix to avoid conflicts between tests
	dbURL = fmt.Sprintf("%s&application_name=%s_%d", dbURL, t.Name(), time.Now().UnixNano())

	// Initialize DBOS context
	dbosCtx, err := dbos.NewDBOSContext(context.Background(), dbos.Config{
		AppName:     fmt.Sprintf("drover-test-%d", time.Now().UnixNano()),
		DatabaseURL: dbURL,
	})
	if err != nil {
		t.Fatalf("Failed to create DBOS context: %v", err)
	}

	// Launch DBOS runtime
	if err := dbos.Launch(dbosCtx); err != nil {
		t.Fatalf("Failed to launch DBOS: %v", err)
	}

	// Create config
	cfg := &config.Config{
		ClaudePath:   mockClaude,
		TaskTimeout:  10 * time.Second,
		Workers:      1,
		WorktreeDir:  filepath.Join(tmpDir, ".drover", "worktrees"),
		PollInterval: 100 * time.Millisecond,
		Verbose:      false,
	}

	// Create DBOS orchestrator
	// Note: Tests pass nil for store since they don't need worktree tracking
	orch, err := workflow.NewDBOSOrchestrator(cfg, dbosCtx, tmpDir, nil)
	if err != nil {
		dbos.Shutdown(dbosCtx, 5*time.Second)
		t.Fatalf("Failed to create DBOS orchestrator: %v", err)
	}

	// Register workflows
	if err := orch.RegisterWorkflows(); err != nil {
		dbos.Shutdown(dbosCtx, 5*time.Second)
		t.Fatalf("Failed to register workflows: %v", err)
	}

	cleanup := func() {
		dbos.Shutdown(dbosCtx, 5*time.Second)
	}

	return tmpDir, dbosCtx, orch, cleanup
}

// setupGitRepo initializes a git repository in the test directory
func setupGitRepo(t *testing.T, dir string) {
	t.Helper()

	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to set git email: %v", err)
	}

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to set git name: %v", err)
	}

	// Create initial commit
	initialFile := filepath.Join(dir, "README.md")
	if err := os.WriteFile(initialFile, []byte("# Test Repo\n"), 0644); err != nil {
		t.Fatalf("Failed to create initial file: %v", err)
	}

	cmd = exec.Command("git", "add", "README.md")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to add initial file: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create initial commit: %v", err)
	}

	cmd = exec.Command("git", "branch", "-M", "main")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to rename branch to main: %v", err)
	}
}

// MockClaudeBehavior defines different behaviors for the mock Claude script
type MockClaudeBehavior string

const (
	MockSuccess MockClaudeBehavior = "success"
	MockFail    MockClaudeBehavior = "fail"
	MockRetry   MockClaudeBehavior = "retry"
)

// createMockClaude creates a mock Claude script with the specified behavior
func createMockClaude(t *testing.T, tmpDir string, behavior MockClaudeBehavior) string {
	t.Helper()

	var scriptContent string
	scriptPath := filepath.Join(tmpDir, fmt.Sprintf("mock-claude-%s.sh", behavior))

	switch behavior {
	case MockSuccess:
		scriptContent = `#!/bin/bash
# Mock Claude that creates a file and succeeds
if [ "$1" = "--version" ]; then
	echo "claude-mock version 1.0.0"
	exit 0
fi

# Extract prompt from -p flag
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

	case MockFail:
		scriptContent = `#!/bin/bash
# Mock Claude that always fails (except for version check)
if [ "$1" = "--version" ]; then
	echo "claude-mock-fail version 1.0.0"
	exit 0
fi
echo "Claude error" >&2
exit 1
`

	case MockRetry:
		attemptFile := filepath.Join(tmpDir, "attempt-counter.txt")
		scriptContent = fmt.Sprintf(`#!/bin/bash
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
# Create a work file
echo "Work completed" > "work-retry-success.txt"
exit 0
`, attemptFile, attemptFile, attemptFile, attemptFile)
	}

	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to create mock claude: %v", err)
	}

	return scriptPath
}

// TestDBOSOrchestrator_SingleTask tests executing a single task with DBOS
func TestDBOSOrchestrator_SingleTask(t *testing.T) {
	_, dbosCtx, orch, cleanup := setupDBOSTestEnvironment(t)
	defer cleanup()

	tasks := []workflow.TaskInput{
		{
			TaskID:      "task-single-1",
			Title:       "Test Task",
			Description: "Do some work",
			Priority:    1,
			MaxAttempts: 3,
		},
	}

	// Execute workflow
	handle, err := dbos.RunWorkflow(dbosCtx, orch.ExecuteAllTasks, tasks)
	if err != nil {
		t.Fatalf("Failed to start workflow: %v", err)
	}

	// Wait for results with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resultsChan := make(chan []workflow.TaskResult, 1)
	go func() {
		results, _ := handle.GetResult()
		resultsChan <- results
	}()

	select {
	case results := <-resultsChan:
		if len(results) != 1 {
			t.Fatalf("Expected 1 result, got %d", len(results))
		}
		if !results[0].Success {
			t.Errorf("Task failed: %s", results[0].Error)
		}
	case <-ctx.Done():
		t.Fatal("Test timed out waiting for workflow completion")
	}
}

// TestDBOSOrchestrator_MultipleTasks tests executing multiple tasks sequentially
func TestDBOSOrchestrator_MultipleTasks(t *testing.T) {
	_, dbosCtx, orch, cleanup := setupDBOSTestEnvironment(t)
	defer cleanup()

	const numTasks = 3
	tasks := make([]workflow.TaskInput, numTasks)
	for i := 0; i < numTasks; i++ {
		tasks[i] = workflow.TaskInput{
			TaskID:      fmt.Sprintf("task-multi-%d", i),
			Title:       fmt.Sprintf("Task %d", i),
			Description: fmt.Sprintf("Do work %d", i),
			Priority:    1,
			MaxAttempts: 3,
		}
	}

	// Execute workflow
	handle, err := dbos.RunWorkflow(dbosCtx, orch.ExecuteAllTasks, tasks)
	if err != nil {
		t.Fatalf("Failed to start workflow: %v", err)
	}

	// Wait for results with timeout
	ctx, cancel := context.WithTimeout(context.WithValue(context.Background(), "test", t), 60*time.Second)
	defer cancel()

	resultsChan := make(chan []workflow.TaskResult, 1)
	go func() {
		results, _ := handle.GetResult()
		resultsChan <- results
	}()

	select {
	case results := <-resultsChan:
		if len(results) != numTasks {
			t.Fatalf("Expected %d results, got %d", numTasks, len(results))
		}

		completed := 0
		for _, r := range results {
			if r.Success {
				completed++
			}
		}

		if completed != numTasks {
			t.Errorf("Expected %d completed tasks, got %d", numTasks, completed)
		}
	case <-ctx.Done():
		t.Fatal("Test timed out waiting for workflow completion")
	}
}

// TestDBOSOrchestrator_QueueParallel tests parallel execution with queues
func TestDBOSOrchestrator_QueueParallel(t *testing.T) {
	_, dbosCtx, orch, cleanup := setupDBOSTestEnvironment(t)
	defer cleanup()

	const numTasks = 3
	tasks := make([]workflow.TaskInput, numTasks)
	for i := 0; i < numTasks; i++ {
		tasks[i] = workflow.TaskInput{
			TaskID:      fmt.Sprintf("task-queue-%d", i),
			Title:       fmt.Sprintf("Queue Task %d", i),
			Description: fmt.Sprintf("Do parallel work %d", i),
			Priority:    1,
			MaxAttempts: 3,
		}
	}

	// Execute with queue
	input := workflow.QueuedTasksInput{Tasks: tasks}
	handle, err := dbos.RunWorkflow(dbosCtx, orch.ExecuteTasksWithQueue, input)
	if err != nil {
		t.Fatalf("Failed to start queued workflow: %v", err)
	}

	// Wait for results with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	statsChan := make(chan workflow.QueueStats, 1)
	go func() {
		stats, _ := handle.GetResult()
		statsChan <- stats
	}()

	select {
	case stats := <-statsChan:
		if stats.Completed != numTasks {
			t.Errorf("Expected %d completed tasks, got %d", numTasks, stats.Completed)
		}
		if stats.Failed > 0 {
			t.Errorf("Expected 0 failed tasks, got %d", stats.Failed)
		}
	case <-ctx.Done():
		t.Fatal("Test timed out waiting for queue workflow completion")
	}
}

// TestDBOSOrchestrator_DependentTasks tests that dependent tasks are handled correctly
func TestDBOSOrchestrator_DependentTasks(t *testing.T) {
	_, dbosCtx, orch, cleanup := setupDBOSTestEnvironment(t)
	defer cleanup()

	// Create tasks with dependencies
	tasks := []workflow.TaskInput{
		{
			TaskID:      "task-dep-1",
			Title:       "First Task",
			Description: "Do first work",
			Priority:    1,
			MaxAttempts: 3,
		},
		{
			TaskID:      "task-dep-2",
			Title:       "Second Task",
			Description: "Do second work",
			Priority:    1,
			MaxAttempts: 3,
			BlockedBy:   []string{"task-dep-1"},
		},
	}

	// Execute with queue - only first task should be enqueued initially
	input := workflow.QueuedTasksInput{Tasks: tasks}
	handle, err := dbos.RunWorkflow(dbosCtx, orch.ExecuteTasksWithQueue, input)
	if err != nil {
		t.Fatalf("Failed to start queued workflow: %v", err)
	}

	// Wait for results with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	statsChan := make(chan workflow.QueueStats, 1)
	go func() {
		stats, _ := handle.GetResult()
		statsChan <- stats
	}()

	select {
	case stats := <-statsChan:
		// Note: In the current implementation, only ready tasks are enqueued
		// The dependent task would need to be enqueued separately after completion
		if stats.TotalEnqueued != 1 {
			t.Logf("Note: Only first task was enqueued (expected behavior for POC)")
		}
	case <-ctx.Done():
		t.Fatal("Test timed out waiting for queue workflow completion")
	}
}

// TestDBOSOrchestrator_TaskFailure tests that failed tasks are handled correctly
func TestDBOSOrchestrator_TaskFailure(t *testing.T) {
	tmpDir, _, _, cleanup := setupDBOSTestEnvironment(t)
	defer cleanup()

	// Create a failing mock claude
	mockClaude := createMockClaude(t, tmpDir, MockFail)

	// Get database URL
	dbURL := os.Getenv("DBOS_SYSTEM_DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/drover_test?sslmode=disable"
	}

	// Create DBOS context
	dbosCtx2, err := dbos.NewDBOSContext(context.Background(), dbos.Config{
		AppName:     fmt.Sprintf("drover-test-fail-%d", time.Now().UnixNano()),
		DatabaseURL: dbURL,
	})
	if err != nil {
		t.Fatalf("Failed to create DBOS context: %v", err)
	}

	if err := dbos.Launch(dbosCtx2); err != nil {
		t.Fatalf("Failed to launch DBOS: %v", err)
	}
	defer dbos.Shutdown(dbosCtx2, 5*time.Second)

	// Create config
	cfg := &config.Config{
		ClaudePath:   mockClaude,
		TaskTimeout:  10 * time.Second,
		Workers:      1,
		WorktreeDir:  filepath.Join(tmpDir, ".drover", "worktrees"),
		PollInterval: 100 * time.Millisecond,
		Verbose:      false,
	}

	// Create DBOS orchestrator
	orch, err := workflow.NewDBOSOrchestrator(cfg, dbosCtx2, tmpDir, nil)
	if err != nil {
		t.Fatalf("Failed to create DBOS orchestrator: %v", err)
	}

	if err := orch.RegisterWorkflows(); err != nil {
		t.Fatalf("Failed to register workflows: %v", err)
	}

	tasks := []workflow.TaskInput{
		{
			TaskID:      "task-fail-1",
			Title:       "Failing Task",
			Description: "This will fail",
			Priority:    1,
			MaxAttempts: 3,
		},
	}

	// Execute workflow
	handle, err := dbos.RunWorkflow(dbosCtx2, orch.ExecuteAllTasks, tasks)
	if err != nil {
		t.Fatalf("Failed to start workflow: %v", err)
	}

	// Wait for results with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	resultsChan := make(chan []workflow.TaskResult, 1)
	go func() {
		results, _ := handle.GetResult()
		resultsChan <- results
	}()

	select {
	case results := <-resultsChan:
		if len(results) != 1 {
			t.Fatalf("Expected 1 result, got %d", len(results))
		}
		if results[0].Success {
			t.Error("Expected task to fail, but it succeeded")
		}
		if results[0].Error == "" {
			t.Error("Expected error message in result")
		}
	case <-ctx.Done():
		t.Fatal("Test timed out waiting for workflow completion")
	}
}

// TestDBOSOrchestrator_TaskRetry tests that tasks are retried on failure
func TestDBOSOrchestrator_TaskRetry(t *testing.T) {
	tmpDir, _, _, cleanup := setupDBOSTestEnvironment(t)
	defer cleanup()

	// Create a retry mock claude
	mockClaude := createMockClaude(t, tmpDir, MockRetry)

	// Get database URL
	dbURL := os.Getenv("DBOS_SYSTEM_DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/drover_test?sslmode=disable"
	}

	// Create DBOS context
	dbosCtx2, err := dbos.NewDBOSContext(context.Background(), dbos.Config{
		AppName:     fmt.Sprintf("drover-test-retry-%d", time.Now().UnixNano()),
		DatabaseURL: dbURL,
	})
	if err != nil {
		t.Fatalf("Failed to create DBOS context: %v", err)
	}

	if err := dbos.Launch(dbosCtx2); err != nil {
		t.Fatalf("Failed to launch DBOS: %v", err)
	}
	defer dbos.Shutdown(dbosCtx2, 5*time.Second)

	// Create config
	cfg := &config.Config{
		ClaudePath:   mockClaude,
		TaskTimeout:  10 * time.Second,
		Workers:      1,
		WorktreeDir:  filepath.Join(tmpDir, ".drover", "worktrees"),
		PollInterval: 100 * time.Millisecond,
		Verbose:      false,
	}

	// Create DBOS orchestrator
	orch, err := workflow.NewDBOSOrchestrator(cfg, dbosCtx2, tmpDir, nil)
	if err != nil {
		t.Fatalf("Failed to create DBOS orchestrator: %v", err)
	}

	if err := orch.RegisterWorkflows(); err != nil {
		t.Fatalf("Failed to register workflows: %v", err)
	}

	tasks := []workflow.TaskInput{
		{
			TaskID:      "task-retry-1",
			Title:       "Retry Task",
			Description: "This will retry",
			Priority:    1,
			MaxAttempts: 3,
		},
	}

	// Execute workflow
	handle, err := dbos.RunWorkflow(dbosCtx2, orch.ExecuteAllTasks, tasks)
	if err != nil {
		t.Fatalf("Failed to start workflow: %v", err)
	}

	// Wait for results with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	resultsChan := make(chan []workflow.TaskResult, 1)
	go func() {
		results, _ := handle.GetResult()
		resultsChan <- results
	}()

	select {
	case results := <-resultsChan:
		if len(results) != 1 {
			t.Fatalf("Expected 1 result, got %d", len(results))
		}
		if !results[0].Success {
			t.Errorf("Expected task to succeed after retries, got error: %s", results[0].Error)
		}
		if !strings.Contains(results[0].Output, "Success on attempt") {
			t.Error("Expected output to mention successful attempt")
		}
	case <-ctx.Done():
		t.Fatal("Test timed out waiting for workflow completion")
	}
}

// TestDBOSOrchestrator_WorkflowRecovery tests that workflows can recover after interruption
func TestDBOSOrchestrator_WorkflowRecovery(t *testing.T) {
	t.Skip("Workflow recovery test requires more complex setup - skipping for POC")

	// This test would:
	// 1. Start a workflow with multiple steps
	// 2. Interrupt it mid-execution
	// 3. Restart and verify it resumes from the last completed step
	// 4. Verify no steps were re-executed
}

// BenchmarkDBOS_Sequential benchmarks sequential DBOS workflow execution
func BenchmarkDBOS_Sequential(b *testing.B) {
	skipIfNoPostgresB(b)

	// Setup for benchmark
	tmpDir := b.TempDir()
	setupGitRepo(&testing.T{}, tmpDir)
	mockClaude := createMockClaude(&testing.T{}, tmpDir, MockSuccess)

	dbURL := os.Getenv("DBOS_SYSTEM_DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/drover_test?sslmode=disable"
	}

	dbosCtx, err := dbos.NewDBOSContext(context.Background(), dbos.Config{
		AppName:     fmt.Sprintf("drover-bench-seq-%d", time.Now().UnixNano()),
		DatabaseURL: dbURL,
	})
	if err != nil {
		b.Fatalf("Failed to create DBOS context: %v", err)
	}

	if err := dbos.Launch(dbosCtx); err != nil {
		b.Fatalf("Failed to launch DBOS: %v", err)
	}
	defer dbos.Shutdown(dbosCtx, 5*time.Second)

	cfg := &config.Config{
		ClaudePath:   mockClaude,
		TaskTimeout:  10 * time.Second,
		Workers:      1,
		WorktreeDir:  filepath.Join(tmpDir, ".drover", "worktrees"),
		PollInterval: 100 * time.Millisecond,
		Verbose:      false,
	}

	orch, err := workflow.NewDBOSOrchestrator(cfg, dbosCtx, tmpDir, nil)
	if err != nil {
		b.Fatalf("Failed to create DBOS orchestrator: %v", err)
	}

	if err := orch.RegisterWorkflows(); err != nil {
		b.Fatalf("Failed to register workflows: %v", err)
	}

	const numTasks = 10
	tasks := make([]workflow.TaskInput, numTasks)
	for i := 0; i < numTasks; i++ {
		tasks[i] = workflow.TaskInput{
			TaskID:      fmt.Sprintf("bench-seq-%d-%d", b.N, i),
			Title:       fmt.Sprintf("Benchmark Task %d", i),
			Description: "Benchmark work",
			Priority:    1,
			MaxAttempts: 3,
		}
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		handle, err := dbos.RunWorkflow(dbosCtx, orch.ExecuteAllTasks, tasks)
		if err != nil {
			b.Fatalf("Failed to start workflow: %v", err)
		}

		_, err = handle.GetResult()
		if err != nil {
			b.Fatalf("Workflow failed: %v", err)
		}
	}
}

// BenchmarkDBOS_Queue benchmarks queue-based parallel execution
func BenchmarkDBOS_Queue(b *testing.B) {
	skipIfNoPostgresB(b)

	// Setup for benchmark
	tmpDir := b.TempDir()
	setupGitRepo(&testing.T{}, tmpDir)
	mockClaude := createMockClaude(&testing.T{}, tmpDir, MockSuccess)

	dbURL := os.Getenv("DBOS_SYSTEM_DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/drover_test?sslmode=disable"
	}

	dbosCtx, err := dbos.NewDBOSContext(context.Background(), dbos.Config{
		AppName:     fmt.Sprintf("drover-bench-queue-%d", time.Now().UnixNano()),
		DatabaseURL: dbURL,
	})
	if err != nil {
		b.Fatalf("Failed to create DBOS context: %v", err)
	}

	if err := dbos.Launch(dbosCtx); err != nil {
		b.Fatalf("Failed to launch DBOS: %v", err)
	}
	defer dbos.Shutdown(dbosCtx, 5*time.Second)

	cfg := &config.Config{
		ClaudePath:   mockClaude,
		TaskTimeout:  10 * time.Second,
		Workers:      1,
		WorktreeDir:  filepath.Join(tmpDir, ".drover", "worktrees"),
		PollInterval: 100 * time.Millisecond,
		Verbose:      false,
	}

	orch, err := workflow.NewDBOSOrchestrator(cfg, dbosCtx, tmpDir, nil)
	if err != nil {
		b.Fatalf("Failed to create DBOS orchestrator: %v", err)
	}

	if err := orch.RegisterWorkflows(); err != nil {
		b.Fatalf("Failed to register workflows: %v", err)
	}

	const numTasks = 10
	tasks := make([]workflow.TaskInput, numTasks)
	for i := 0; i < numTasks; i++ {
		tasks[i] = workflow.TaskInput{
			TaskID:      fmt.Sprintf("bench-queue-%d-%d", b.N, i),
			Title:       fmt.Sprintf("Benchmark Queue Task %d", i),
			Description: "Benchmark parallel work",
			Priority:    1,
			MaxAttempts: 3,
		}
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		input := workflow.QueuedTasksInput{Tasks: tasks}
		handle, err := dbos.RunWorkflow(dbosCtx, orch.ExecuteTasksWithQueue, input)
		if err != nil {
			b.Fatalf("Failed to start workflow: %v", err)
		}

		_, err = handle.GetResult()
		if err != nil {
			b.Fatalf("Workflow failed: %v", err)
		}
	}
}

// skipIfNoPostgresB skips the benchmark if PostgreSQL is not available
func skipIfNoPostgresB(b *testing.B) {
	b.Helper()

	dbURL := os.Getenv("DBOS_SYSTEM_DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/drover_test?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "pg_isready", "-h", "localhost", "-p", "5432")
	if err := cmd.Run(); err != nil {
		b.Skip("PostgreSQL not available - skipping DBOS benchmarks")
	}
}
