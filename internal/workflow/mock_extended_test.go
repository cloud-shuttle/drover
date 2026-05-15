package workflow

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/cloud-shuttle/drover-libs/pkg/clock"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
)

// ============================================================================
// MockDBOSContext — Additional Coverage
// ============================================================================

func TestMockDBOSContext_WithParent(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	mock := NewMockDBOSContextWithParent(parent)
	if mock.Context != parent {
		t.Error("expected parent context to be preserved")
	}
	if mock.applicationID != "drover-test-mock" {
		t.Errorf("unexpected application ID: %s", mock.applicationID)
	}
}

func TestMockDBOSContext_StepCallCount(t *testing.T) {
	mock := NewMockDBOSContext()
	if mock.StepCallCount() != 0 {
		t.Errorf("expected 0 initial step calls, got %d", mock.StepCallCount())
	}

	// Execute a step
	mock.RunAsStep(mock, func(ctx context.Context) (any, error) {
		return "result", nil
	})
	if mock.StepCallCount() != 1 {
		t.Errorf("expected 1 step call, got %d", mock.StepCallCount())
	}
}

func TestMockDBOSContext_WorkflowCallCount(t *testing.T) {
	mock := NewMockDBOSContext()
	if mock.WorkflowCallCount() != 0 {
		t.Errorf("expected 0 initial workflow calls, got %d", mock.WorkflowCallCount())
	}

	// Execute a workflow
	mock.RunWorkflow(mock, func(ctx dbos.DBOSContext, input any) (any, error) {
		return nil, nil
	}, "input")
	if mock.WorkflowCallCount() != 1 {
		t.Errorf("expected 1 workflow call, got %d", mock.WorkflowCallCount())
	}
}

func TestMockDBOSContext_GoCallCount(t *testing.T) {
	mock := NewMockDBOSContext()
	if mock.GoCallCount() != 0 {
		t.Errorf("expected 0 initial go calls, got %d", mock.GoCallCount())
	}

	ch, err := mock.Go(mock, func(ctx context.Context) (any, error) {
		return "async-result", nil
	})
	if err != nil {
		t.Fatalf("Go returned error: %v", err)
	}
	// Wait for result
	outcome := <-ch
	if outcome.Err != nil {
		t.Fatalf("unexpected error in outcome: %v", outcome.Err)
	}
	if mock.GoCallCount() != 1 {
		t.Errorf("expected 1 go call, got %d", mock.GoCallCount())
	}
}

func TestMockDBOSContext_RunAsStep_WithHook(t *testing.T) {
	mock := NewMockDBOSContext()
	mock.OnRunAsStep = func(fn dbos.StepFunc) (any, error) {
		return "hooked", nil
	}

	result, err := mock.RunAsStep(mock, func(ctx context.Context) (any, error) {
		return "original", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hooked" {
		t.Errorf("expected 'hooked', got %v", result)
	}
}

func TestMockDBOSContext_RunAsStep_DefaultExecution(t *testing.T) {
	mock := NewMockDBOSContext()
	result, err := mock.RunAsStep(mock, func(ctx context.Context) (any, error) {
		return 42, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != 42 {
		t.Errorf("expected 42, got %v", result)
	}
}

func TestMockDBOSContext_RunAsStep_ErrorPropagation(t *testing.T) {
	mock := NewMockDBOSContext()
	expectedErr := fmt.Errorf("step failed")

	_, err := mock.RunAsStep(mock, func(ctx context.Context) (any, error) {
		return nil, expectedErr
	})
	if err != expectedErr {
		t.Errorf("expected error to propagate, got %v", err)
	}
	if mock.StepCallCount() != 1 {
		t.Error("step call should be recorded even on error")
	}
}

func TestMockDBOSContext_RunWorkflow_WithHook(t *testing.T) {
	mock := NewMockDBOSContext()
	mock.OnRunWorkflow = func(fn dbos.WorkflowFunc, input any) (any, error) {
		return "executed", nil
	}

	handle, err := mock.RunWorkflow(mock, func(ctx dbos.DBOSContext, input any) (any, error) {
		return nil, nil
	}, "some-input")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	result, _ := handle.GetResult()
	if result != "executed" {
		t.Errorf("expected 'executed', got %v", result)
	}
}

func TestMockDBOSContext_RunWorkflow_DefaultNoOp(t *testing.T) {
	mock := NewMockDBOSContext()
	handle, err := mock.RunWorkflow(mock, func(ctx dbos.DBOSContext, input any) (any, error) {
		panic("should not be called")
	}, "input")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	result, _ := handle.GetResult()
	if result != nil {
		t.Errorf("expected nil result from default no-op, got %v", result)
	}
}

func TestMockDBOSContext_Select(t *testing.T) {
	mock := NewMockDBOSContext()

	ch := make(chan dbos.StepOutcome[any], 1)
	ch <- dbos.StepOutcome[any]{Result: "selected", Err: nil}

	result, err := mock.Select(mock, []<-chan dbos.StepOutcome[any]{ch})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "selected" {
		t.Errorf("expected 'selected', got %v", result)
	}
}

func TestMockDBOSContext_Select_Empty(t *testing.T) {
	mock := NewMockDBOSContext()
	_, err := mock.Select(mock, []<-chan dbos.StepOutcome[any]{})
	if err == nil {
		t.Error("expected error for empty channels")
	}
}

// ============================================================================
// Mock context — messaging stubs
// ============================================================================

func TestMockDBOSContext_Send(t *testing.T) {
	mock := NewMockDBOSContext()
	err := mock.Send(mock, "dest", "msg", "topic")
	if err != nil {
		t.Errorf("Send should return nil: %v", err)
	}
}

func TestMockDBOSContext_Recv(t *testing.T) {
	mock := NewMockDBOSContext()
	result, err := mock.Recv(mock, "topic", time.Second)
	if err != nil || result != nil {
		t.Errorf("Recv should return nil, nil: %v, %v", result, err)
	}
}

func TestMockDBOSContext_SetEvent(t *testing.T) {
	mock := NewMockDBOSContext()
	err := mock.SetEvent(mock, "key", "value")
	if err != nil {
		t.Errorf("SetEvent should return nil: %v", err)
	}
}

func TestMockDBOSContext_GetEvent(t *testing.T) {
	mock := NewMockDBOSContext()
	result, err := mock.GetEvent(mock, "workflow-1", "key", time.Second)
	if err != nil || result != nil {
		t.Errorf("GetEvent should return nil, nil: %v, %v", result, err)
	}
}

// ============================================================================
// Mock context — context derivation
// ============================================================================

func TestMockDBOSContext_WithValue(t *testing.T) {
	mock := NewMockDBOSContext()
	type ctxKey string
	derived := mock.WithValue(ctxKey("key"), "val")
	if derived == nil {
		t.Fatal("WithValue returned nil")
	}
	if derived.Value(ctxKey("key")) != "val" {
		t.Error("value not propagated")
	}
}

func TestMockDBOSContext_WithCancelCause(t *testing.T) {
	mock := NewMockDBOSContext()
	derived, cancel := mock.WithCancelCause()
	if derived == nil {
		t.Fatal("WithCancelCause returned nil")
	}
	cancel(fmt.Errorf("test reason"))
	<-derived.Done()
}

// ============================================================================
// Mock context — queue/schedule stubs
// ============================================================================

func TestMockDBOSContext_ListenQueues(t *testing.T) {
	mock := NewMockDBOSContext()
	mock.ListenQueues(mock) // Should not panic
}

func TestMockDBOSContext_ScheduleStubs(t *testing.T) {
	mock := NewMockDBOSContext()

	if err := mock.CreateSchedule(mock, nil, dbos.CreateScheduleRequest{}); err != nil {
		t.Errorf("CreateSchedule: %v", err)
	}
	if err := mock.ApplySchedules(mock, nil); err != nil {
		t.Errorf("ApplySchedules: %v", err)
	}
	if err := mock.PauseSchedule(mock, "s1"); err != nil {
		t.Errorf("PauseSchedule: %v", err)
	}
	if err := mock.ResumeSchedule(mock, "s1"); err != nil {
		t.Errorf("ResumeSchedule: %v", err)
	}
	if err := mock.DeleteSchedule(mock, "s1"); err != nil {
		t.Errorf("DeleteSchedule: %v", err)
	}
	sched, err := mock.GetSchedule(mock, "s1")
	if err != nil || sched != nil {
		t.Errorf("GetSchedule: %v, %v", sched, err)
	}
	scheds, err := mock.ListSchedules(mock)
	if err != nil || scheds != nil {
		t.Errorf("ListSchedules: %v, %v", scheds, err)
	}
	ids, err := mock.BackfillSchedule(mock, "s1", time.Now(), time.Now())
	if err != nil || ids != nil {
		t.Errorf("BackfillSchedule: %v, %v", ids, err)
	}
	handle, err := mock.TriggerSchedule(mock, "s1")
	if err != nil || handle != nil {
		t.Errorf("TriggerSchedule: %v, %v", handle, err)
	}
}

func TestMockDBOSContext_SetAlertHandler(t *testing.T) {
	mock := NewMockDBOSContext()
	mock.SetAlertHandler(nil) // Should not panic
}

// ============================================================================
// mockWorkflowHandle
// ============================================================================

func TestMockWorkflowHandle_GetResult(t *testing.T) {
	h := &mockWorkflowHandle[any]{result: "data", err: nil}
	result, err := h.GetResult()
	if err != nil || result != "data" {
		t.Errorf("expected 'data', nil — got %v, %v", result, err)
	}
}

func TestMockWorkflowHandle_GetResult_Error(t *testing.T) {
	expectedErr := fmt.Errorf("failed")
	h := &mockWorkflowHandle[any]{result: nil, err: expectedErr}
	_, err := h.GetResult()
	if err != expectedErr {
		t.Errorf("expected error, got %v", err)
	}
}

func TestMockWorkflowHandle_GetWorkflowID(t *testing.T) {
	h := &mockWorkflowHandle[any]{}
	id := h.GetWorkflowID()
	if id != "mock-workflow-handle-id" {
		t.Errorf("unexpected ID: %s", id)
	}
}

func TestMockWorkflowHandle_GetStatus(t *testing.T) {
	h := &mockWorkflowHandle[any]{}
	status, err := h.GetStatus()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Status != dbos.WorkflowStatusSuccess {
		t.Errorf("expected Success status, got %s", status.Status)
	}
}

// ============================================================================
// DBOSOrchestrator — dependency & completion logic
// ============================================================================

func TestDBOSWorkflowIDForTask_Format(t *testing.T) {
	id := DBOSWorkflowIDForTask("task-42")
	if id != "drover-task-task-42" {
		t.Errorf("expected 'drover-task-task-42', got %s", id)
	}
}

func TestDBOSWorkflowIDForTask_EmptyInput(t *testing.T) {
	id := DBOSWorkflowIDForTask("")
	if id != "drover-task-" {
		t.Errorf("expected 'drover-task-', got %s", id)
	}
}

func TestGenerateWorkflowID_DeterministicWithMockClock(t *testing.T) {
	mockClock := clock.NewMockClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	o := &DBOSOrchestrator{clock: mockClock}

	id1 := o.generateWorkflowID()
	id2 := o.generateWorkflowID()
	// Atomic counter ensures unique IDs even with frozen clock
	if id1 == id2 {
		t.Errorf("expected unique IDs with frozen clock (atomic counter), got %q and %q", id1, id2)
	}

	// Both should share the same timestamp prefix
	if !containsStr(id1, "workflow-") || !containsStr(id2, "workflow-") {
		t.Errorf("expected 'workflow-' prefix in both IDs: %q, %q", id1, id2)
	}

	// Advance clock → still different from both
	mockClock.Add(time.Millisecond)
	id3 := o.generateWorkflowID()
	if id1 == id3 || id2 == id3 {
		t.Error("expected different ID after advancing clock")
	}
}

// ============================================================================
// Orchestrator dependency map + completion — additional patterns
// ============================================================================

func TestBuildDependencyMap_ChainOfThree(t *testing.T) {
	o := &DBOSOrchestrator{
		completedTasks: make(map[string]bool),
	}
	tasks := []TaskInput{
		{TaskID: "A", Title: "First"},
		{TaskID: "B", Title: "Second", BlockedBy: []string{"A"}},
		{TaskID: "C", Title: "Third", BlockedBy: []string{"B"}},
	}

	o.buildDependencyMap(tasks)

	// A should unblock B
	if deps, ok := o.dependencyMap["A"]; !ok || len(deps) != 1 || deps[0] != "B" {
		t.Errorf("A should unblock B, got %v", o.dependencyMap["A"])
	}
	// B should unblock C
	if deps, ok := o.dependencyMap["B"]; !ok || len(deps) != 1 || deps[0] != "C" {
		t.Errorf("B should unblock C, got %v", o.dependencyMap["B"])
	}
	// All 3 should be in registry
	if len(o.taskRegistry) != 3 {
		t.Errorf("expected 3 tasks in registry, got %d", len(o.taskRegistry))
	}
}

func TestFindReadyTasks_WithPartialCompletion(t *testing.T) {
	o := &DBOSOrchestrator{
		completedTasks: make(map[string]bool),
	}
	tasks := []TaskInput{
		{TaskID: "A", Title: "Ready"},
		{TaskID: "B", Title: "Blocked", BlockedBy: []string{"A", "C"}},
		{TaskID: "C", Title: "Also Ready"},
	}
	o.buildDependencyMap(tasks)

	// Mark A as complete
	o.MarkTaskComplete("A")

	ready := o.findReadyTasks(tasks)
	// Only C should be ready (B still blocked by C, A already complete)
	readyIDs := make(map[string]bool)
	for _, r := range ready {
		readyIDs[r.TaskID] = true
	}
	if readyIDs["B"] {
		t.Error("B should not be ready — still blocked by C")
	}
	if !readyIDs["C"] {
		t.Error("C should be ready — no blockers")
	}
}

func TestMarkTaskComplete_MultipleAndIdempotent(t *testing.T) {
	o := &DBOSOrchestrator{
		completedTasks: make(map[string]bool),
	}

	o.MarkTaskComplete("t1")
	o.MarkTaskComplete("t2")
	o.MarkTaskComplete("t1") // duplicate

	if !o.isTaskComplete("t1") || !o.isTaskComplete("t2") {
		t.Error("both tasks should be marked complete")
	}
	if o.isTaskComplete("t3") {
		t.Error("t3 should not be complete")
	}
}

func TestFindTaskInput_RegistryLookup(t *testing.T) {
	o := &DBOSOrchestrator{
		completedTasks: make(map[string]bool),
	}
	tasks := []TaskInput{
		{TaskID: "x1", Title: "X1 Task", Priority: 5},
	}
	o.buildDependencyMap(tasks)

	input, found := o.findTaskInput("x1")
	if !found {
		t.Fatal("expected to find x1")
	}
	if input.Priority != 5 {
		t.Errorf("expected priority 5, got %d", input.Priority)
	}

	_, found = o.findTaskInput("nonexistent")
	if found {
		t.Error("should not find nonexistent task")
	}
}

// ============================================================================
// TaskInput / TaskResult / QueueStats
// ============================================================================

func TestTaskResult_SuccessFields(t *testing.T) {
	r := TaskResult{
		Success:    true,
		Output:     "all good",
		Duration:   3 * time.Second,
		HasChanges: true,
	}
	if !r.Success || r.Output != "all good" || !r.HasChanges {
		t.Errorf("unexpected fields: %+v", r)
	}
}

func TestTaskResult_FailureFields(t *testing.T) {
	r := TaskResult{
		Success: false,
		Error:   "boom",
	}
	if r.Success || r.Error != "boom" {
		t.Errorf("unexpected fields: %+v", r)
	}
}

func TestQueueStats_Computation(t *testing.T) {
	s := QueueStats{
		TotalEnqueued: 10,
		Completed:     7,
		Failed:        2,
		Duration:      10 * time.Second,
	}
	if s.TotalEnqueued != s.Completed+s.Failed+1 {
		t.Errorf("expected TotalEnqueued 10 = 7+2+1, got %d", s.TotalEnqueued)
	}
	if s.Duration != 10*time.Second {
		t.Errorf("unexpected duration: %v", s.Duration)
	}
}

// ============================================================================
// Mock context — stream stubs (all at 0%)
// ============================================================================

func TestMockDBOSContext_WriteStream(t *testing.T) {
	mock := NewMockDBOSContext()
	err := mock.WriteStream(mock, "stream-key", "value")
	if err != nil {
		t.Errorf("WriteStream should return nil: %v", err)
	}
}

func TestMockDBOSContext_CloseStream(t *testing.T) {
	mock := NewMockDBOSContext()
	err := mock.CloseStream(mock, "stream-key")
	if err != nil {
		t.Errorf("CloseStream should return nil: %v", err)
	}
}

func TestMockDBOSContext_ReadStream(t *testing.T) {
	mock := NewMockDBOSContext()
	vals, closed, err := mock.ReadStream(mock, "wf-1", "key")
	if err != nil || vals != nil || closed {
		t.Errorf("ReadStream should return nil, false, nil: %v, %v, %v", vals, closed, err)
	}
}

func TestMockDBOSContext_ReadStreamAsync(t *testing.T) {
	mock := NewMockDBOSContext()
	ch, err := mock.ReadStreamAsync(mock, "wf-1", "key")
	if err != nil || ch != nil {
		t.Errorf("ReadStreamAsync should return nil, nil: %v, %v", ch, err)
	}
}

// ============================================================================
// Mock context — durable ops stubs (0%)
// ============================================================================

func TestMockDBOSContext_DeprecatePatch(t *testing.T) {
	mock := NewMockDBOSContext()
	err := mock.DeprecatePatch(mock, "migration-1")
	if err != nil {
		t.Errorf("DeprecatePatch should return nil: %v", err)
	}
}

func TestMockDBOSContext_GetStepID(t *testing.T) {
	mock := NewMockDBOSContext()
	id, err := mock.GetStepID()
	if err != nil || id != 0 {
		t.Errorf("GetStepID should return 0, nil: %d, %v", id, err)
	}
}

// ============================================================================
// Mock context — workflow management stubs (all at 0%)
// ============================================================================

func TestMockDBOSContext_RetrieveWorkflow(t *testing.T) {
	mock := NewMockDBOSContext()
	handle, err := mock.RetrieveWorkflow(mock, "wf-123")
	if err == nil {
		t.Error("RetrieveWorkflow should return error")
	}
	if handle != nil {
		t.Error("RetrieveWorkflow should return nil handle")
	}
}

func TestMockDBOSContext_CancelWorkflow(t *testing.T) {
	mock := NewMockDBOSContext()
	err := mock.CancelWorkflow(mock, "wf-123")
	if err != nil {
		t.Errorf("CancelWorkflow should return nil: %v", err)
	}
}

func TestMockDBOSContext_SetWorkflowDelay(t *testing.T) {
	mock := NewMockDBOSContext()
	err := mock.SetWorkflowDelay(mock, "wf-123")
	if err != nil {
		t.Errorf("SetWorkflowDelay should return nil: %v", err)
	}
}

func TestMockDBOSContext_ResumeWorkflow(t *testing.T) {
	mock := NewMockDBOSContext()
	handle, err := mock.ResumeWorkflow(mock, "wf-123")
	if err == nil {
		t.Error("ResumeWorkflow should return error")
	}
	if handle != nil {
		t.Error("ResumeWorkflow should return nil handle")
	}
}

func TestMockDBOSContext_ResumeWorkflows(t *testing.T) {
	mock := NewMockDBOSContext()
	handles, err := mock.ResumeWorkflows(mock, []string{"wf-1", "wf-2"})
	if err == nil {
		t.Error("ResumeWorkflows should return error")
	}
	if handles != nil {
		t.Error("ResumeWorkflows should return nil handles")
	}
}

func TestMockDBOSContext_ForkWorkflow(t *testing.T) {
	mock := NewMockDBOSContext()
	handle, err := mock.ForkWorkflow(mock, dbos.ForkWorkflowInput{})
	if err == nil {
		t.Error("ForkWorkflow should return error")
	}
	if handle != nil {
		t.Error("ForkWorkflow should return nil handle")
	}
}

func TestMockDBOSContext_ListWorkflows(t *testing.T) {
	mock := NewMockDBOSContext()
	statuses, err := mock.ListWorkflows(mock)
	if err != nil || statuses != nil {
		t.Errorf("ListWorkflows should return nil, nil: %v, %v", statuses, err)
	}
}

func TestMockDBOSContext_GetWorkflowSteps(t *testing.T) {
	mock := NewMockDBOSContext()
	steps, err := mock.GetWorkflowSteps(mock, "wf-1")
	if err != nil || steps != nil {
		t.Errorf("GetWorkflowSteps should return nil, nil: %v, %v", steps, err)
	}
}

func TestMockDBOSContext_ListRegisteredWorkflows(t *testing.T) {
	mock := NewMockDBOSContext()
	entries, err := mock.ListRegisteredWorkflows(mock)
	if err != nil || entries != nil {
		t.Errorf("ListRegisteredWorkflows should return nil, nil: %v, %v", entries, err)
	}
}

func TestMockDBOSContext_ListRegisteredQueues(t *testing.T) {
	mock := NewMockDBOSContext()
	queues, err := mock.ListRegisteredQueues(mock)
	if err != nil || queues != nil {
		t.Errorf("ListRegisteredQueues should return nil, nil: %v, %v", queues, err)
	}
}

func TestMockDBOSContext_DeleteWorkflows(t *testing.T) {
	mock := NewMockDBOSContext()
	err := mock.DeleteWorkflows(mock, []string{"wf-1", "wf-2"})
	if err != nil {
		t.Errorf("DeleteWorkflows should return nil: %v", err)
	}
}

// ============================================================================
// Mock context — context management (From, WithoutCancel, WithTimeout)
// ============================================================================

func TestMockDBOSContext_From(t *testing.T) {
	mock := NewMockDBOSContext()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	derived := mock.From(mock, ctx)
	if derived == nil {
		t.Fatal("From returned nil")
	}
}

func TestMockDBOSContext_WithoutCancel(t *testing.T) {
	mock := NewMockDBOSContext()
	derived := mock.WithoutCancel(mock)
	if derived != mock {
		t.Error("WithoutCancel should return same mock")
	}
}

func TestMockDBOSContext_WithTimeout(t *testing.T) {
	mock := NewMockDBOSContext()
	derived, cancel := mock.WithTimeout(mock, time.Second)
	defer cancel()
	if derived == nil {
		t.Fatal("WithTimeout returned nil")
	}
}
