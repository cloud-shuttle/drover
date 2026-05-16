package workflow

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/cloud-shuttle/drover/internal/clock"
	"github.com/cloud-shuttle/drover/internal/config"
	"github.com/cloud-shuttle/drover/internal/db"
	"github.com/cloud-shuttle/drover/internal/executor"
	"github.com/cloud-shuttle/drover/pkg/types"
	"go.opentelemetry.io/otel/trace"
)

// ============================================================================
// buildDependencyMap — extended edge cases
// ============================================================================

func TestBuildDependencyMap_DiamondDependency(t *testing.T) {
	o := &DBOSOrchestrator{
		dependencyMap: make(map[string][]string),
	}

	// Diamond: task-1 -> task-2, task-3 -> task-4
	o.buildDependencyMap([]TaskInput{
		{TaskID: "task-1"},
		{TaskID: "task-2", BlockedBy: []string{"task-1"}},
		{TaskID: "task-3", BlockedBy: []string{"task-1"}},
		{TaskID: "task-4", BlockedBy: []string{"task-2", "task-3"}},
	})

	// task-1 should have 2 dependents
	deps1 := o.dependencyMap["task-1"]
	if len(deps1) != 2 {
		t.Errorf("task-1 should have 2 dependents, got %d", len(deps1))
	}

	// task-2 and task-3 should each have task-4
	for _, blockerID := range []string{"task-2", "task-3"} {
		deps := o.dependencyMap[blockerID]
		if len(deps) != 1 || deps[0] != "task-4" {
			t.Errorf("%s dependents: got %v, want [task-4]", blockerID, deps)
		}
	}

	// task-4 has no dependents
	if deps, ok := o.dependencyMap["task-4"]; ok && len(deps) > 0 {
		t.Errorf("task-4 should have no dependents, got %v", deps)
	}
}

func TestBuildDependencyMap_MultipleBlockers(t *testing.T) {
	o := &DBOSOrchestrator{
		dependencyMap: make(map[string][]string),
	}

	o.buildDependencyMap([]TaskInput{
		{TaskID: "task-1"},
		{TaskID: "task-2"},
		{TaskID: "task-3", BlockedBy: []string{"task-1", "task-2"}},
	})

	// Both task-1 and task-2 should list task-3 as a dependent
	for _, id := range []string{"task-1", "task-2"} {
		deps := o.dependencyMap[id]
		if len(deps) != 1 || deps[0] != "task-3" {
			t.Errorf("%s dependents: got %v, want [task-3]", id, deps)
		}
	}
}

func TestBuildDependencyMap_SelfReference(t *testing.T) {
	o := &DBOSOrchestrator{
		dependencyMap: make(map[string][]string),
	}

	// Edge case: task blocks itself (should still build, even if logically invalid)
	o.buildDependencyMap([]TaskInput{
		{TaskID: "task-1", BlockedBy: []string{"task-1"}},
	})

	deps := o.dependencyMap["task-1"]
	if len(deps) != 1 || deps[0] != "task-1" {
		t.Errorf("self-referencing dependency: got %v", deps)
	}
}

// ============================================================================
// findReadyTasks — extended edge cases
// ============================================================================

func TestFindReadyTasks_EmptyInput(t *testing.T) {
	o := &DBOSOrchestrator{
		dependencyMap: make(map[string][]string),
	}

	ready := o.findReadyTasks(nil)
	if len(ready) != 0 {
		t.Errorf("expected 0 ready tasks for nil input, got %d", len(ready))
	}
}

func TestFindReadyTasks_MixedPriority(t *testing.T) {
	o := &DBOSOrchestrator{
		dependencyMap: make(map[string][]string),
	}

	tasks := []TaskInput{
		{TaskID: "task-1", Priority: 1},                               // Urgent, no blockers
		{TaskID: "task-2", Priority: 4, BlockedBy: []string{"task-1"}}, // Low, blocked
		{TaskID: "task-3", Priority: 2},                               // High, no blockers
	}

	o.buildDependencyMap(tasks)
	ready := o.findReadyTasks(tasks)

	if len(ready) != 2 {
		t.Errorf("expected 2 ready tasks, got %d", len(ready))
	}

	// Ensure blocked task is not in ready list
	for _, r := range ready {
		if r.TaskID == "task-2" {
			t.Error("task-2 should not be ready")
		}
	}
}

// ============================================================================
// recordEvent
// ============================================================================

func TestRecordEvent_WithStore(t *testing.T) {
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	o := &DBOSOrchestrator{
		store: store,
	}

	// Should not panic with valid data
	o.recordEvent("task_claimed", "task-1", "epic-1", map[string]any{
		"worker": "test-worker",
	})
}

func TestRecordEvent_EmptyData(t *testing.T) {
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	o := &DBOSOrchestrator{
		store: store,
	}

	// Should not panic with nil data
	o.recordEvent("task_started", "task-1", "", nil)
}

func TestRecordEvent_DataSerialization(t *testing.T) {
	data := map[string]any{
		"worker":   "dbos-workflow",
		"title":    "Test Task",
		"duration": int64(5000),
	}

	// Verify JSON serialization works for event data
	bytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal event data: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(bytes, &decoded); err != nil {
		t.Fatalf("failed to unmarshal event data: %v", err)
	}

	if decoded["worker"] != "dbos-workflow" {
		t.Errorf("expected worker=dbos-workflow, got %v", decoded["worker"])
	}
}

// ============================================================================
// getProjectTaskContextCount / getProjectTaskContextMaxAge
// ============================================================================

func TestGetProjectTaskContextCount_NoProjectDir(t *testing.T) {
	o := &DBOSOrchestrator{
		config: &config.Config{},
	}

	count := o.getProjectTaskContextCount()
	if count != 5 {
		t.Errorf("expected default count 5, got %d", count)
	}
}

func TestGetProjectTaskContextCount_NilConfig(t *testing.T) {
	o := &DBOSOrchestrator{}

	count := o.getProjectTaskContextCount()
	if count != 5 {
		t.Errorf("expected default count 5 with nil config, got %d", count)
	}
}

func TestGetProjectTaskContextMaxAge_NoProjectDir(t *testing.T) {
	o := &DBOSOrchestrator{
		config: &config.Config{},
	}

	maxAge := o.getProjectTaskContextMaxAge()
	if maxAge != 24*time.Hour {
		t.Errorf("expected default max age 24h, got %v", maxAge)
	}
}

func TestGetProjectTaskContextMaxAge_NilConfig(t *testing.T) {
	o := &DBOSOrchestrator{}

	maxAge := o.getProjectTaskContextMaxAge()
	if maxAge != 24*time.Hour {
		t.Errorf("expected default max age 24h with nil config, got %v", maxAge)
	}
}

// ============================================================================
// generateWorkflowID — additional
// ============================================================================

func TestGenerateWorkflowID_Format(t *testing.T) {
	id := generateWorkflowID()
	if len(id) < 10 {
		t.Errorf("workflow ID too short: %s", id)
	}
	if id[:9] != "workflow-" {
		t.Errorf("workflow ID should start with 'workflow-', got: %s", id)
	}
}

// ============================================================================
// Stop
// ============================================================================

func TestStop_NilPool(t *testing.T) {
	o := &DBOSOrchestrator{
		pool: nil,
	}

	// Should not panic
	o.Stop()
}

// ============================================================================
// TaskResult type validation
// ============================================================================

func TestTaskResult_SuccessResult(t *testing.T) {
	result := TaskResult{
		Success:    true,
		Output:     "completed",
		Duration:   5 * time.Second,
		HasChanges: true,
		CommitHash: "abc123",
	}

	if !result.Success {
		t.Error("expected success=true")
	}
	if result.Duration != 5*time.Second {
		t.Errorf("expected duration=5s, got %v", result.Duration)
	}
	if result.CommitHash != "abc123" {
		t.Errorf("expected commitHash=abc123, got %s", result.CommitHash)
	}
}

func TestTaskResult_FailureResult(t *testing.T) {
	result := TaskResult{
		Success: false,
		Error:   "agent error: timeout",
	}

	if result.Success {
		t.Error("expected success=false")
	}
	if result.Error != "agent error: timeout" {
		t.Errorf("unexpected error: %s", result.Error)
	}
}

// ============================================================================
// Step function edge cases
// ============================================================================

func TestExecuteClaudeStep_AgentTimeout(t *testing.T) {
	// Use a real temp dir so os.Stat(worktreePath) succeeds and doesn't
	// trigger the worktree recreation path (which needs a git manager).
	worktreeDir := t.TempDir()

	mockAgent := &MockAgent{
		ExecuteFunc: func(ctx context.Context, worktreePath string, task *types.Task, parentSpan ...trace.Span) *executor.ExecutionResult {
			return &executor.ExecutionResult{
				Success:  false,
				Output:   "",
				Duration: 30 * time.Second,
				Error:    context.DeadlineExceeded,
			}
		},
	}

	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	o := &DBOSOrchestrator{
		agent: mockAgent,
		clock: clock.RealClock{},
		store: store,
		config: &config.Config{
			AgentType: "claude",
		},
	}

	task := TaskInput{
		TaskID:      "task-timeout",
		Title:       "Timeout Task",
		Description: "Should timeout",
	}

	// executeClaudeStep returns (nil, error) when agent fails
	_, err = o.executeClaudeStep(context.Background(), worktreeDir, task, nil)
	if err == nil {
		t.Fatal("expected error from executeClaudeStep for failed agent")
	}
	if err != context.DeadlineExceeded {
		t.Errorf("expected DeadlineExceeded error, got: %v", err)
	}
}

func TestCreateWorktreeStep_NoPool(t *testing.T) {
	gitMgr := NewSuccessGitManager("/tmp/worktree/task-pool")

	o := &DBOSOrchestrator{
		clock: clock.RealClock{},
		git:   gitMgr,
		pool:  nil,
	}

	task := TaskInput{TaskID: "task-pool", Title: "Pool Test"}
	path, err := o.createWorktreeStep(context.Background(), task)
	if err != nil {
		t.Fatalf("createWorktreeStep failed: %v", err)
	}

	if path != "/tmp/worktree/task-pool" {
		t.Errorf("unexpected path: %s", path)
	}
}

// ============================================================================
// Concurrent access to dependencyMap
// ============================================================================

func TestBuildDependencyMap_ThreadSafety(t *testing.T) {
	o := &DBOSOrchestrator{
		dependencyMap: make(map[string][]string),
	}

	tasks := []TaskInput{
		{TaskID: "task-1"},
		{TaskID: "task-2", BlockedBy: []string{"task-1"}},
		{TaskID: "task-3", BlockedBy: []string{"task-1"}},
	}

	// Concurrent read/write should not race
	done := make(chan bool, 2)
	go func() {
		o.buildDependencyMap(tasks)
		done <- true
	}()
	go func() {
		_ = o.findReadyTasks(tasks)
		done <- true
	}()

	<-done
	<-done
}

// ============================================================================
// allBlockersComplete
// ============================================================================

func TestAllBlockersComplete_AllResolved(t *testing.T) {
	o := &DBOSOrchestrator{
		dependencyMap:  make(map[string][]string),
		completedTasks: make(map[string]bool),
		taskInputMap:   make(map[string]TaskInput),
	}

	tasks := []TaskInput{
		{TaskID: "task-1"},
		{TaskID: "task-2"},
		{TaskID: "task-3", BlockedBy: []string{"task-1", "task-2"}},
	}
	o.buildDependencyMap(tasks)

	// Mark both blockers complete
	o.completedTasks["task-1"] = true
	o.completedTasks["task-2"] = true

	if !o.allBlockersComplete("task-3") {
		t.Error("expected all blockers complete for task-3")
	}
}

func TestAllBlockersComplete_PartiallyResolved(t *testing.T) {
	o := &DBOSOrchestrator{
		dependencyMap:  make(map[string][]string),
		completedTasks: make(map[string]bool),
		taskInputMap:   make(map[string]TaskInput),
	}

	tasks := []TaskInput{
		{TaskID: "task-1"},
		{TaskID: "task-2"},
		{TaskID: "task-3", BlockedBy: []string{"task-1", "task-2"}},
	}
	o.buildDependencyMap(tasks)

	// Only mark one blocker complete
	o.completedTasks["task-1"] = true

	if o.allBlockersComplete("task-3") {
		t.Error("expected blockers NOT complete for task-3 (task-2 still pending)")
	}
}

func TestAllBlockersComplete_NoBlockers(t *testing.T) {
	o := &DBOSOrchestrator{
		dependencyMap:  make(map[string][]string),
		completedTasks: make(map[string]bool),
		taskInputMap:   make(map[string]TaskInput),
	}

	tasks := []TaskInput{
		{TaskID: "task-1"}, // no blockers
	}
	o.buildDependencyMap(tasks)

	if !o.allBlockersComplete("task-1") {
		t.Error("expected allBlockersComplete=true for task with no blockers")
	}
}

func TestAllBlockersComplete_UnknownTask(t *testing.T) {
	o := &DBOSOrchestrator{
		dependencyMap:  make(map[string][]string),
		completedTasks: make(map[string]bool),
		taskInputMap:   make(map[string]TaskInput),
	}

	// Unknown task should return false
	if o.allBlockersComplete("nonexistent") {
		t.Error("expected false for unknown task")
	}
}

// ============================================================================
// OnTaskComplete (unit-level — no DBOS context, tests marking logic)
// ============================================================================

func TestOnTaskComplete_MarksCompleted(t *testing.T) {
	o := &DBOSOrchestrator{
		dependencyMap:  make(map[string][]string),
		completedTasks: make(map[string]bool),
		taskInputMap:   make(map[string]TaskInput),
	}

	tasks := []TaskInput{
		{TaskID: "task-1"},
		{TaskID: "task-2", BlockedBy: []string{"task-1"}},
	}
	o.buildDependencyMap(tasks)

	// Simulate — call the marking logic directly (OnTaskComplete needs DBOS ctx for RunWorkflow)
	o.dependencyMu.Lock()
	o.completedTasks["task-1"] = true
	o.dependencyMu.Unlock()

	if !o.completedTasks["task-1"] {
		t.Error("task-1 should be marked as completed")
	}

	// task-2 should now have all blockers resolved
	if !o.allBlockersComplete("task-2") {
		t.Error("task-2 should be ready after task-1 completes")
	}
}

func TestAllBlockersComplete_DiamondResolution(t *testing.T) {
	o := &DBOSOrchestrator{
		dependencyMap:  make(map[string][]string),
		completedTasks: make(map[string]bool),
		taskInputMap:   make(map[string]TaskInput),
	}

	// Diamond: task-1 -> task-2, task-3 -> task-4
	tasks := []TaskInput{
		{TaskID: "task-1"},
		{TaskID: "task-2", BlockedBy: []string{"task-1"}},
		{TaskID: "task-3", BlockedBy: []string{"task-1"}},
		{TaskID: "task-4", BlockedBy: []string{"task-2", "task-3"}},
	}
	o.buildDependencyMap(tasks)

	// Complete task-1 -> task-2 and task-3 should be ready
	o.completedTasks["task-1"] = true
	if !o.allBlockersComplete("task-2") {
		t.Error("task-2 should be ready after task-1")
	}
	if !o.allBlockersComplete("task-3") {
		t.Error("task-3 should be ready after task-1")
	}
	// task-4 should NOT be ready yet
	if o.allBlockersComplete("task-4") {
		t.Error("task-4 should NOT be ready (task-2 and task-3 not complete)")
	}

	// Complete task-2 -> task-4 still not ready (task-3 pending)
	o.completedTasks["task-2"] = true
	if o.allBlockersComplete("task-4") {
		t.Error("task-4 should NOT be ready (task-3 not complete)")
	}

	// Complete task-3 -> task-4 now ready
	o.completedTasks["task-3"] = true
	if !o.allBlockersComplete("task-4") {
		t.Error("task-4 should be ready after task-2 and task-3")
	}
}

// ============================================================================
// buildDependencyMap — verifies taskInputMap population
// ============================================================================

func TestBuildDependencyMap_PopulatesTaskInputMap(t *testing.T) {
	o := &DBOSOrchestrator{
		dependencyMap:  make(map[string][]string),
		completedTasks: make(map[string]bool),
		taskInputMap:   make(map[string]TaskInput),
	}

	tasks := []TaskInput{
		{TaskID: "task-1", Title: "First", Priority: 1},
		{TaskID: "task-2", Title: "Second", Priority: 2, BlockedBy: []string{"task-1"}},
	}
	o.buildDependencyMap(tasks)

	// Verify taskInputMap was populated
	if len(o.taskInputMap) != 2 {
		t.Errorf("expected 2 entries in taskInputMap, got %d", len(o.taskInputMap))
	}

	t1, ok := o.taskInputMap["task-1"]
	if !ok {
		t.Fatal("task-1 not found in taskInputMap")
	}
	if t1.Title != "First" || t1.Priority != 1 {
		t.Errorf("task-1 input mismatch: got %+v", t1)
	}

	t2, ok := o.taskInputMap["task-2"]
	if !ok {
		t.Fatal("task-2 not found in taskInputMap")
	}
	if t2.Title != "Second" || len(t2.BlockedBy) != 1 {
		t.Errorf("task-2 input mismatch: got %+v", t2)
	}
}
