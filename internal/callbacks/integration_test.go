package callbacks

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestMetricsCallback tests the metrics callback implementation
func TestMetricsCallback(t *testing.T) {
	m := NewMetricsCallback()

	ctx := &TaskEventContext{
		TaskID:    "task-1",
		Title:     "Test Task",
		EpicID:    "epic-1",
		Type:      "test",
		Priority:  1,
		WorkerID:  "worker-1",
		Timestamp: time.Now(),
	}

	// Test task lifecycle events
	if err := m.OnTaskCreated(ctx); err != nil {
		t.Fatalf("OnTaskCreated failed: %v", err)
	}

	if err := m.OnTaskAssigned(ctx); err != nil {
		t.Fatalf("OnTaskAssigned failed: %v", err)
	}

	if err := m.OnTaskStarted(ctx); err != nil {
		t.Fatalf("OnTaskStarted failed: %v", err)
	}

	duration := 5 * time.Second
	ctx.Duration = &duration
	if err := m.OnTaskCompleted(ctx); err != nil {
		t.Fatalf("OnTaskCompleted failed: %v", err)
	}

	// Verify metrics were recorded
	metric, exists := m.GetMetric("drover_tasks_created_total", map[string]string{"epic": "epic-1", "type": "test"})
	if !exists {
		t.Fatal("Metric not found")
	}
	if metric.Value != 1 {
		t.Errorf("Expected metric value 1, got %v", metric.Value)
	}

	// Test Prometheus export
	var sb strings.Builder
	if err := m.WritePrometheus(&sb); err != nil {
		t.Fatalf("WritePrometheus failed: %v", err)
	}

	output := sb.String()
	if !strings.Contains(output, "drover_tasks_created_total") {
		t.Error("Prometheus output missing metric")
	}
}

// TestLifecycleCallback tests the lifecycle callback implementation
func TestLifecycleCallback(t *testing.T) {
	lc := NewLifecycleCallback()

	ctx := &TaskEventContext{
		TaskID:    "task-1",
		Title:     "Test Task",
		EpicID:    "epic-1",
		Type:      "test",
		Priority:  1,
		WorkerID:  "worker-1",
		Timestamp: time.Now(),
	}

	// Test lifecycle transitions
	if err := lc.OnTaskCreated(ctx); err != nil {
		t.Fatalf("OnTaskCreated failed: %v", err)
	}

	state, exists := lc.stateMachine.GetState(ctx.TaskID)
	if !exists {
		t.Fatal("Task state not found")
	}
	if state.Phase != PhaseInitializing {
		t.Errorf("Expected phase %s, got %s", PhaseInitializing, state.Phase)
	}

	if err := lc.OnTaskAssigned(ctx); err != nil {
		t.Fatalf("OnTaskAssigned failed: %v", err)
	}

	state, _ = lc.stateMachine.GetState(ctx.TaskID)
	if state.Phase != PhaseRunning {
		t.Errorf("Expected phase %s, got %s", PhaseRunning, state.Phase)
	}

	// Test invalid transition
	err := lc.stateMachine.Transition(ctx.TaskID, PhaseInitializing, "invalid")
	if err == nil {
		t.Error("Expected error for invalid transition")
	}

	// Test pre/post hooks
	hookCalled := false
	lc.RegisterPreHook("completed", func(ctx *TaskEventContext) error {
		hookCalled = true
		return nil
	})

	if err := lc.OnTaskCompleted(ctx); err != nil {
		t.Fatalf("OnTaskCompleted failed: %v", err)
	}

	if !hookCalled {
		t.Error("Pre-hook was not called")
	}
}

// TestHealthCallback tests the health callback implementation
func TestHealthCallback(t *testing.T) {
	hc := NewHealthCallback()

	// Register a simple health check
	checkCalled := false
	hc.RegisterCheck("test_component", func(ctx context.Context) (*ComponentHealth, error) {
		checkCalled = true
		return &ComponentHealth{
			Name:      "test_component",
			Status:    StatusHealthy,
			Message:   "all good",
			CheckedAt: time.Now(),
		}, nil
	})

	// Run health checks
	ctx := context.Background()
	health, err := hc.RunChecks(ctx)
	if err != nil {
		t.Fatalf("RunChecks failed: %v", err)
	}

	if !checkCalled {
		t.Error("Health check was not called")
	}

	if health.Status != StatusHealthy {
		t.Errorf("Expected status %s, got %s", StatusHealthy, health.Status)
	}

	if len(health.Components) != 1 {
		t.Errorf("Expected 1 component, got %d", len(health.Components))
	}

	// Test IsHealthy and IsReady
	if !hc.IsHealthy() {
		t.Error("Expected system to be healthy")
	}

	if !hc.IsReady() {
		t.Error("Expected system to be ready")
	}

	// Unregister check
	hc.UnregisterCheck("test_component")
	health = hc.GetStatus()
	if health.Status != StatusUnknown {
		t.Errorf("Expected status %s after unregistering all checks, got %s", StatusUnknown, health.Status)
	}
}

// TestCompositeCallback tests the composite callback implementation
func TestCompositeCallback(t *testing.T) {
	m1 := NewMetricsCallback()
	m2 := NewMetricsCallback()
	cc := NewCompositeCallback(m1, m2)

	ctx := &TaskEventContext{
		TaskID:    "task-1",
		Title:     "Test Task",
		EpicID:    "epic-1",
		Type:      "test",
		Priority:  1,
		Timestamp: time.Now(),
	}

	// Both callbacks should be called
	if err := cc.OnTaskCreated(ctx); err != nil {
		t.Fatalf("OnTaskCreated failed: %v", err)
	}

	// Verify both metrics callbacks recorded the event
	metric1, _ := m1.GetMetric("drover_tasks_created_total", map[string]string{"epic": "epic-1", "type": "test"})
	if metric1.Value != 1 {
		t.Errorf("Expected m1 metric value 1, got %v", metric1.Value)
	}

	metric2, _ := m2.GetMetric("drover_tasks_created_total", map[string]string{"epic": "epic-1", "type": "test"})
	if metric2.Value != 1 {
		t.Errorf("Expected m2 metric value 1, got %v", metric2.Value)
	}

	// Test count
	if cc.Count() != 2 {
		t.Errorf("Expected 2 callbacks, got %d", cc.Count())
	}

	// Test remove
	cc.Remove(m1)
	if cc.Count() != 1 {
		t.Errorf("Expected 1 callback after remove, got %d", cc.Count())
	}
}

// TestTimeoutMiddleware tests the timeout middleware
func TestTimeoutMiddleware(t *testing.T) {
	tm := NewTimeoutMiddleware(100 * time.Millisecond)

	// Create a slow callback
	slowCallback := &testSlowCallback{delay: 200 * time.Millisecond}

	wrapped := tm.Wrap(slowCallback)

	ctx := &TaskEventContext{
		TaskID:    "task-1",
		Title:     "Test Task",
		EpicID:    "epic-1",
		Type:      "test",
		Priority:  1,
		Timestamp: time.Now(),
	}

	err := wrapped.OnTaskCreated(ctx)
	if err == nil {
		t.Error("Expected timeout error")
	}

	// Test with fast callback
	fastCallback := &testSlowCallback{delay: 50 * time.Millisecond}
	wrapped = tm.Wrap(fastCallback)

	err = wrapped.OnTaskCreated(ctx)
	if err != nil {
		t.Errorf("Unexpected error with fast callback: %v", err)
	}
}

// TestRetryMiddleware tests the retry middleware
func TestRetryMiddleware(t *testing.T) {
	rm := NewRetryMiddleware(3)

	// Create a failing callback
	failingCallback := &testFailingCallback{failCount: 2, succeedOn: 3}

	wrapped := rm.Wrap(failingCallback)

	ctx := &TaskEventContext{
		TaskID:    "task-1",
		Title:     "Test Task",
		EpicID:    "epic-1",
		Type:      "test",
		Priority:  1,
		Timestamp: time.Now(),
	}

	err := wrapped.OnTaskCreated(ctx)
	if err != nil {
		t.Errorf("Unexpected error after retries: %v", err)
	}
}

// TestFilterMiddleware tests the filter middleware
func TestFilterMiddleware(t *testing.T) {
	fm := NewFilterMiddleware()
	fm.FilterByEpic("epic-1")

	m := NewMetricsCallback()
	wrapped := fm.Wrap(m)

	ctx1 := &TaskEventContext{
		TaskID:    "task-1",
		Title:     "Test Task",
		EpicID:    "epic-1",
		Type:      "test",
		Priority:  1,
		Timestamp: time.Now(),
	}

	ctx2 := &TaskEventContext{
		TaskID:    "task-2",
		Title:     "Test Task",
		EpicID:    "epic-2", // Different epic
		Type:      "test",
		Priority:  1,
		Timestamp: time.Now(),
	}

	// Only epic-1 should be recorded
	wrapped.OnTaskCreated(ctx1)
	wrapped.OnTaskCreated(ctx2)

	// Verify only epic-1 metric exists
	metric, exists := m.GetMetric("drover_tasks_created_total", map[string]string{"epic": "epic-1", "type": "test"})
	if !exists {
		t.Error("Expected epic-1 metric to exist")
	}
	if metric.Value != 1 {
		t.Errorf("Expected epic-1 metric value 1, got %v", metric.Value)
	}

	// epic-2 should not exist
	_, exists = m.GetMetric("drover_tasks_created_total", map[string]string{"epic": "epic-2", "type": "test"})
	if exists {
		t.Error("Expected epic-2 metric to be filtered out")
	}
}

// TestRegistry tests the callback registry
func TestRegistry(t *testing.T) {
	r := NewRegistry()

	m := NewMetricsCallback()
	l := NewLifecycleCallback()

	// Test register - both callbacks for all task events
	allEvents := []EventType{
		EventTaskCreated, EventTaskAssigned, EventTaskStarted,
		EventTaskCompleted, EventTaskFailed, EventTaskBlocked,
		EventTaskUnblocked,
	}
	r.Register(m, allEvents, PriorityMedium, "metrics")
	r.Register(l, allEvents, PriorityMedium, "lifecycle")

	// Test count
	count := r.Count(EventTaskCreated)
	if count != 2 {
		t.Errorf("Expected 2 callbacks registered, got %d", count)
	}

	// Test registered names
	names := r.RegisteredNames(EventTaskCreated)
	if len(names) != 2 {
		t.Errorf("Expected 2 registered names, got %d", len(names))
	}

	// Test unregister
	r.Unregister("metrics", nil) // Remove from all events
	count = r.Count(EventTaskCreated)
	if count != 1 {
		t.Errorf("Expected 1 callback after unregister, got %d", count)
	}

	// Test dispatch
	ctx := &TaskEventContext{
		TaskID:    "task-1",
		Title:     "Test Task",
		EpicID:    "epic-1",
		Type:      "test",
		Priority:  1,
		Timestamp: time.Now(),
	}

	if err := r.Dispatch(EventTaskCreated, ctx); err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	state, _ := l.stateMachine.GetState(ctx.TaskID)
	if state == nil {
		t.Error("Expected lifecycle callback to have recorded state")
	}
}

// Test helpers

type testSlowCallback struct {
	delay time.Duration
	Callback
}

func (t *testSlowCallback) OnTaskCreated(ctx *TaskEventContext) error {
	time.Sleep(t.delay)
	return nil
}

type testFailingCallback struct {
	failCount int
	succeedOn int
	Callback
}

func (t *testFailingCallback) OnTaskCreated(ctx *TaskEventContext) error {
	t.failCount++
	if t.failCount < t.succeedOn {
		return errors.New("simulated failure")
	}
	return nil
}
