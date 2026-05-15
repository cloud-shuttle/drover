package workflow

import (
	"testing"
	"github.com/cloud-shuttle/drover-libs/pkg/clock"
	"time"

	"github.com/cloud-shuttle/drover/internal/config"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
)

// ============================================================================
// MockDBOSContext self-tests
// ============================================================================

func TestMockDBOSContext_ImplementsInterface(t *testing.T) {
	// Compile-time check is in mock_dbos_context.go via:
	//   var _ dbos.DBOSContext = (*MockDBOSContext)(nil)
	// This test verifies runtime construction works.
	mock := NewMockDBOSContext()
	if mock == nil {
		t.Fatal("expected non-nil mock context")
	}
}

func TestMockDBOSContext_LaunchShutdown(t *testing.T) {
	mock := NewMockDBOSContext()

	if mock.WasLaunched() {
		t.Error("should not be launched initially")
	}
	if mock.WasShutdown() {
		t.Error("should not be shut down initially")
	}

	if err := mock.Launch(); err != nil {
		t.Fatalf("Launch() returned error: %v", err)
	}
	if !mock.WasLaunched() {
		t.Error("should be launched after Launch()")
	}

	mock.Shutdown(5 * time.Second)
	if !mock.WasShutdown() {
		t.Error("should be shut down after Shutdown()")
	}
}

func TestMockDBOSContext_Accessors(t *testing.T) {
	mock := NewMockDBOSContext()

	if mock.GetApplicationID() != "drover-test-mock" {
		t.Errorf("unexpected application ID: %q", mock.GetApplicationID())
	}
	if mock.GetExecutorID() != "mock-executor" {
		t.Errorf("unexpected executor ID: %q", mock.GetExecutorID())
	}
	if mock.GetApplicationVersion() != "test-0.0.0" {
		t.Errorf("unexpected app version: %q", mock.GetApplicationVersion())
	}
}

func TestMockDBOSContext_GetWorkflowID(t *testing.T) {
	mock := NewMockDBOSContext()
	id, err := mock.GetWorkflowID()
	if err != nil {
		t.Fatalf("GetWorkflowID() error: %v", err)
	}
	if id != "mock-workflow-id" {
		t.Errorf("expected 'mock-workflow-id', got %q", id)
	}
}

func TestMockDBOSContext_Sleep(t *testing.T) {
	mock := NewMockDBOSContext()

	start := time.Now()
	d, err := mock.Sleep(mock, 10*time.Minute)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Sleep() error: %v", err)
	}
	if d != 10*time.Minute {
		t.Errorf("expected 10m duration returned, got %v", d)
	}
	// Should NOT actually sleep
	if elapsed > 100*time.Millisecond {
		t.Errorf("mock Sleep should not actually sleep, took %v", elapsed)
	}
}

func TestMockDBOSContext_Patch(t *testing.T) {
	mock := NewMockDBOSContext()
	isNew, err := mock.Patch(mock, "v2-feature")
	if err != nil {
		t.Fatalf("Patch() error: %v", err)
	}
	if !isNew {
		t.Error("mock Patch should always return true (new code path)")
	}
}

// ============================================================================
// RegisterWorkflows (no-PostgreSQL)
// ============================================================================

func TestRegisterWorkflows_WithMock(t *testing.T) {
	mock := NewMockDBOSContext()
	cfg := testConfig(t)

	tmpDir := t.TempDir()
	orch, err := NewDBOSOrchestrator(cfg, mock, tmpDir, nil, clock.RealClock{})
	if err != nil {
		t.Fatalf("NewDBOSOrchestrator error: %v", err)
	}

	// RegisterWorkflows should not error with mock context
	if err := orch.RegisterWorkflows(); err != nil {
		t.Fatalf("RegisterWorkflows() error: %v", err)
	}
}

// ============================================================================
// ExecuteAllTasks (sequential workflow)
// ============================================================================

func TestExecuteAllTasks_EmptyTasks(t *testing.T) {
	mock := NewMockDBOSContext()

	orch := newMockOrchestrator(t, mock)

	results, err := orch.ExecuteAllTasks(mock, []TaskInput{})
	if err != nil {
		t.Fatalf("ExecuteAllTasks error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty input, got %d", len(results))
	}
}

// ============================================================================
// OnTaskComplete with mock context
// ============================================================================

func TestOnTaskComplete_NoDependents_WithMock(t *testing.T) {
	mock := NewMockDBOSContext()
	orch := newMockOrchestrator(t, mock)

	tasks := []TaskInput{{TaskID: "t1"}}
	orch.buildDependencyMap(tasks)

	enqueued, err := orch.OnTaskComplete(mock, "t1")
	if err != nil {
		t.Fatalf("OnTaskComplete error: %v", err)
	}
	if enqueued != 0 {
		t.Errorf("expected 0 enqueued, got %d", enqueued)
	}
}

func TestOnTaskComplete_UnknownTask_WithMock(t *testing.T) {
	mock := NewMockDBOSContext()
	orch := newMockOrchestrator(t, mock)

	tasks := []TaskInput{{TaskID: "t1"}}
	orch.buildDependencyMap(tasks)

	// Call with a task ID that doesn't exist in the dependency map
	enqueued, err := orch.OnTaskComplete(mock, "nonexistent")
	if err != nil {
		t.Fatalf("OnTaskComplete error: %v", err)
	}
	if enqueued != 0 {
		t.Errorf("expected 0 enqueued for unknown task, got %d", enqueued)
	}
}

func TestOnTaskComplete_SingleBlocker_WithMock(t *testing.T) {
	mock := NewMockDBOSContext()
	orch := newMockOrchestrator(t, mock)

	tasks := []TaskInput{
		{TaskID: "t1"},
		{TaskID: "t2", Title: "Task 2", BlockedBy: []string{"t1"}},
	}
	orch.buildDependencyMap(tasks)

	// Mark t1 as complete (its only blocker for t2)
	orch.MarkTaskComplete("t1")

	// OnTaskComplete should enqueue t2
	enqueued, err := orch.OnTaskComplete(mock, "t1")
	if err != nil {
		t.Fatalf("OnTaskComplete error: %v", err)
	}
	if enqueued != 1 {
		t.Errorf("expected 1 enqueued (t2), got %d", enqueued)
	}
}

func TestOnTaskComplete_MultiBlocker_PartialComplete_WithMock(t *testing.T) {
	mock := NewMockDBOSContext()
	orch := newMockOrchestrator(t, mock)

	tasks := []TaskInput{
		{TaskID: "t1"},
		{TaskID: "t2"},
		{TaskID: "t3", Title: "Task 3", BlockedBy: []string{"t1", "t2"}},
	}
	orch.buildDependencyMap(tasks)

	// Complete only t1 — t3 still blocked by t2
	orch.MarkTaskComplete("t1")

	enqueued, err := orch.OnTaskComplete(mock, "t1")
	if err != nil {
		t.Fatalf("OnTaskComplete error: %v", err)
	}
	if enqueued != 0 {
		t.Errorf("expected 0 enqueued (t2 still blocking t3), got %d", enqueued)
	}
}

func TestOnTaskComplete_MultiBlocker_AllComplete_WithMock(t *testing.T) {
	mock := NewMockDBOSContext()
	orch := newMockOrchestrator(t, mock)

	tasks := []TaskInput{
		{TaskID: "t1"},
		{TaskID: "t2"},
		{TaskID: "t3", Title: "Task 3", BlockedBy: []string{"t1", "t2"}},
	}
	orch.buildDependencyMap(tasks)

	// Complete both blockers
	orch.MarkTaskComplete("t1")
	orch.MarkTaskComplete("t2")

	// t3 should now be enqueued when either t1 or t2 triggers OnTaskComplete
	enqueued, err := orch.OnTaskComplete(mock, "t2")
	if err != nil {
		t.Fatalf("OnTaskComplete error: %v", err)
	}
	if enqueued != 1 {
		t.Errorf("expected 1 enqueued (t3), got %d", enqueued)
	}
}

func TestOnTaskComplete_DiamondPattern_WithMock(t *testing.T) {
	mock := NewMockDBOSContext()
	orch := newMockOrchestrator(t, mock)

	tasks := []TaskInput{
		{TaskID: "A"},
		{TaskID: "B", Title: "Task B", BlockedBy: []string{"A"}},
		{TaskID: "C", Title: "Task C", BlockedBy: []string{"A"}},
		{TaskID: "D", Title: "Task D", BlockedBy: []string{"B", "C"}},
	}
	orch.buildDependencyMap(tasks)

	// Complete A → should enqueue B and C
	orch.MarkTaskComplete("A")
	enqueued, err := orch.OnTaskComplete(mock, "A")
	if err != nil {
		t.Fatalf("OnTaskComplete(A) error: %v", err)
	}
	if enqueued != 2 {
		t.Errorf("expected 2 enqueued (B, C) after A, got %d", enqueued)
	}

	// Complete B → D is still blocked by C
	orch.MarkTaskComplete("B")
	enqueued, err = orch.OnTaskComplete(mock, "B")
	if err != nil {
		t.Fatalf("OnTaskComplete(B) error: %v", err)
	}
	if enqueued != 0 {
		t.Errorf("expected 0 enqueued (D still blocked by C), got %d", enqueued)
	}

	// Complete C → D should now be enqueued
	orch.MarkTaskComplete("C")
	enqueued, err = orch.OnTaskComplete(mock, "C")
	if err != nil {
		t.Fatalf("OnTaskComplete(C) error: %v", err)
	}
	if enqueued != 1 {
		t.Errorf("expected 1 enqueued (D) after C, got %d", enqueued)
	}
}

// ============================================================================
// PrintResults / PrintQueueStats with mock
// ============================================================================

func TestPrintResults_WithMock(t *testing.T) {
	mock := NewMockDBOSContext()
	orch := newMockOrchestrator(t, mock)

	results := []TaskResult{
		{Success: true, Duration: 1 * time.Second},
		{Success: false, Error: "failed"},
	}

	// Should not panic
	orch.PrintResults(results)
}

func TestPrintQueueStats_WithMock(t *testing.T) {
	mock := NewMockDBOSContext()
	orch := newMockOrchestrator(t, mock)

	stats := QueueStats{
		TotalEnqueued: 5,
		Completed:     3,
		Failed:        2,
		Duration:      10 * time.Second,
	}

	// Should not panic
	orch.PrintQueueStats(stats)
}

// ============================================================================
// Test Helpers
// ============================================================================

func newMockOrchestrator(t *testing.T, mock *MockDBOSContext) *DBOSOrchestrator {
	t.Helper()
	return &DBOSOrchestrator{clock: clock.RealClock{},
		config:         testConfig(t),
		dbosCtx:        mock,
		queue:          mockQueue(),
		verbose:        false,
		dependencyMap:  make(map[string][]string),
		taskRegistry:   make(map[string]TaskInput),
		completedTasks: make(map[string]bool),
	}
}

func mockQueue() dbos.WorkflowQueue {
	return dbos.WorkflowQueue{Name: "test-queue"}
}

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		AgentType:   "claude",
		AgentPath:   "/usr/bin/true",
		TaskTimeout: 10 * time.Second,
		Workers:     1,
		WorktreeDir: t.TempDir(),
		Verbose:     false,
	}
}
