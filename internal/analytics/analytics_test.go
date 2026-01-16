package analytics

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "analytics.json")

	m, err := NewManager(Config{ConfigPath: configPath})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer m.Stop(context.Background())

	if m.maxMetrics != 10000 {
		t.Errorf("Expected maxMetrics=10000, got %d", m.maxMetrics)
	}

	if m.flushInterval != 5*time.Minute {
		t.Errorf("Expected flushInterval=5m, got %v", m.flushInterval)
	}
}

func TestRecordMetric(t *testing.T) {
	m, err := NewManager(Config{})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer m.Stop(context.Background())

	metric := &Metric{
		Name:  "test_metric",
		Type:  MetricTypeCounter,
		Value: 42,
		Labels: map[string]string{
			"label1": "value1",
		},
	}

	m.RecordMetric(metric)

	metrics := m.GetMetrics("test_metric", nil, 0, 0)
	if len(metrics) != 1 {
		t.Fatalf("Expected 1 metric, got %d", len(metrics))
	}

	if metrics[0].Value != 42 {
		t.Errorf("Expected value=42, got %f", metrics[0].Value)
	}

	if metrics[0].Labels["label1"] != "value1" {
		t.Errorf("Expected label1=value1, got %s", metrics[0].Labels["label1"])
	}
}

func TestIncrementCounter(t *testing.T) {
	m, err := NewManager(Config{})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer m.Stop(context.Background())

	m.IncrementCounter("test_counter", 1, map[string]string{"env": "test"})
	m.IncrementCounter("test_counter", 2, map[string]string{"env": "test"})

	metrics := m.GetMetrics("test_counter", nil, 0, 0)
	if len(metrics) != 2 {
		t.Fatalf("Expected 2 metrics, got %d", len(metrics))
	}
}

func TestSetGauge(t *testing.T) {
	m, err := NewManager(Config{})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer m.Stop(context.Background())

	m.SetGauge("temperature", 98.6, map[string]string{"unit": "fahrenheit"})

	metrics := m.GetMetrics("temperature", nil, 0, 0)
	if len(metrics) != 1 {
		t.Fatalf("Expected 1 metric, got %d", len(metrics))
	}

	if metrics[0].Value != 98.6 {
		t.Errorf("Expected value=98.6, got %f", metrics[0].Value)
	}
}

func TestRecordHistogram(t *testing.T) {
	m, err := NewManager(Config{})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer m.Stop(context.Background())

	m.RecordHistogram("request_duration", 100, map[string]string{"endpoint": "/api"})
	m.RecordHistogram("request_duration", 150, map[string]string{"endpoint": "/api"})
	m.RecordHistogram("request_duration", 200, map[string]string{"endpoint": "/api"})

	metrics := m.GetMetrics("request_duration", nil, 0, 0)
	if len(metrics) != 3 {
		t.Fatalf("Expected 3 metrics, got %d", len(metrics))
	}
}

func TestStartAndEndTask(t *testing.T) {
	m, err := NewManager(Config{})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer m.Stop(context.Background())

	taskID := "task-123"
	m.StartTask(taskID, "Test Task", "claude", "proj-1")

	// Verify task is in progress
	taskMetric := m.GetTaskMetrics(taskID)
	if taskMetric == nil {
		t.Fatal("Task metric not found")
	}

	if taskMetric.Status != "running" {
		t.Errorf("Expected status=running, got %s", taskMetric.Status)
	}

	if taskMetric.TaskID != taskID {
		t.Errorf("Expected taskID=%s, got %s", taskID, taskMetric.TaskID)
	}

	// End the task
	// Note: Duration is in seconds, so very fast tasks may have 0 duration
	// Adding a small sleep to ensure at least 1 second duration
	time.Sleep(1100 * time.Millisecond)
	m.EndTask(taskID, "success", "")

	// Verify task is completed
	taskMetric = m.GetTaskMetrics(taskID)
	if taskMetric.Status != "success" {
		t.Errorf("Expected status=success, got %s", taskMetric.Status)
	}

	if taskMetric.Duration == 0 {
		t.Error("Expected duration > 0")
	}
}

func TestUpdateTaskTokens(t *testing.T) {
	m, err := NewManager(Config{})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer m.Stop(context.Background())

	taskID := "task-456"
	m.StartTask(taskID, "Test Task", "claude", "proj-1")
	m.UpdateTaskTokens(taskID, 1000)
	m.EndTask(taskID, "success", "")

	taskMetric := m.GetTaskMetrics(taskID)
	if taskMetric.TokenCount != 1000 {
		t.Errorf("Expected tokenCount=1000, got %d", taskMetric.TokenCount)
	}
}

func TestIncrementTaskLLMCalls(t *testing.T) {
	m, err := NewManager(Config{})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer m.Stop(context.Background())

	taskID := "task-789"
	m.StartTask(taskID, "Test Task", "claude", "proj-1")
	m.IncrementTaskLLMCalls(taskID)
	m.IncrementTaskLLMCalls(taskID)
	m.EndTask(taskID, "success", "")

	taskMetric := m.GetTaskMetrics(taskID)
	if taskMetric.LLMCalls != 2 {
		t.Errorf("Expected llmCalls=2, got %d", taskMetric.LLMCalls)
	}
}

func TestUpdateTaskFiles(t *testing.T) {
	m, err := NewManager(Config{})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer m.Stop(context.Background())

	taskID := "task-abc"
	m.StartTask(taskID, "Test Task", "claude", "proj-1")
	m.UpdateTaskFiles(taskID, 3, 2, 1)
	m.EndTask(taskID, "success", "")

	taskMetric := m.GetTaskMetrics(taskID)
	if taskMetric.FilesCreated != 3 {
		t.Errorf("Expected filesCreated=3, got %d", taskMetric.FilesCreated)
	}
	if taskMetric.FilesModified != 2 {
		t.Errorf("Expected filesModified=2, got %d", taskMetric.FilesModified)
	}
	if taskMetric.FilesDeleted != 1 {
		t.Errorf("Expected filesDeleted=1, got %d", taskMetric.FilesDeleted)
	}
}

func TestGetMetricsWithFilters(t *testing.T) {
	m, err := NewManager(Config{})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer m.Stop(context.Background())

	now := time.Now().Unix()

	// Record different metrics
	m.IncrementCounter("metric_a", 1, map[string]string{"env": "prod"})
	m.IncrementCounter("metric_b", 1, map[string]string{"env": "prod"})
	m.IncrementCounter("metric_a", 1, map[string]string{"env": "dev"})

	// Test filter by name
	metrics := m.GetMetrics("metric_a", nil, 0, 0)
	if len(metrics) != 2 {
		t.Errorf("Expected 2 metrics for metric_a, got %d", len(metrics))
	}

	// Test filter by labels
	metrics = m.GetMetrics("metric_a", map[string]string{"env": "prod"}, 0, 0)
	if len(metrics) != 1 {
		t.Errorf("Expected 1 metric with env=prod, got %d", len(metrics))
	}

	// Test filter by time range
	startTime := now + 10 // Future time
	metrics = m.GetMetrics("metric_a", nil, startTime, 0)
	if len(metrics) != 0 {
		t.Errorf("Expected 0 metrics with future start time, got %d", len(metrics))
	}
}

func TestAggregateMetrics(t *testing.T) {
	m, err := NewManager(Config{})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer m.Stop(context.Background())

	now := time.Now().Unix()

	// Create some tasks
	m.StartTask("task-1", "Task 1", "claude", "proj-1")
	m.UpdateTaskTokens("task-1", 100)
	m.EndTask("task-1", "success", "")

	m.StartTask("task-2", "Task 2", "claude", "proj-1")
	m.UpdateTaskTokens("task-2", 200)
	m.EndTask("task-2", "failed", "error")

	m.StartTask("task-3", "Task 3", "opencode", "proj-1")
	m.UpdateTaskTokens("task-3", 150)
	m.EndTask("task-3", "success", "")

	// Aggregate
	agg := m.AggregateMetrics(now-100, now+100)

	if agg.TotalTasks != 3 {
		t.Errorf("Expected totalTasks=3, got %d", agg.TotalTasks)
	}

	if agg.SuccessTasks != 2 {
		t.Errorf("Expected successTasks=2, got %d", agg.SuccessTasks)
	}

	if agg.FailedTasks != 1 {
		t.Errorf("Expected failedTasks=1, got %d", agg.FailedTasks)
	}

	if agg.TotalTokens != 450 {
		t.Errorf("Expected totalTokens=450, got %d", agg.TotalTokens)
	}

	if agg.ByAgentType["claude"] != 2 {
		t.Errorf("Expected claude count=2, got %d", agg.ByAgentType["claude"])
	}

	if agg.ByAgentType["opencode"] != 1 {
		t.Errorf("Expected opencode count=1, got %d", agg.ByAgentType["opencode"])
	}
}

func TestGetTimeSeries(t *testing.T) {
	m, err := NewManager(Config{})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer m.Stop(context.Background())

	now := time.Now().Unix()
	hour := int64(time.Hour.Seconds())

	// Record metrics over time
	for i := 0; i < 5; i++ {
		timestamp := now - int64(i)*hour
		m.RecordMetric(&Metric{
			Name:      "cpu_usage",
			Type:      MetricTypeGauge,
			Value:     float64(50 + i*10),
			Timestamp: timestamp,
			Labels:    map[string]string{"host": "server1"},
		})
	}

	// Get time series with 2-hour intervals
	series := m.GetTimeSeries("cpu_usage", map[string]string{"host": "server1"}, now-5*hour, now+hour, 2*time.Hour)

	if len(series) == 0 {
		t.Fatal("Expected time series data, got none")
	}

	// Verify we have data points
	for _, point := range series {
		if point.Timestamp < now-5*hour || point.Timestamp > now+hour {
			t.Errorf("Timestamp %d out of expected range", point.Timestamp)
		}
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "analytics.json")

	// Create manager and add data
	m1, err := NewManager(Config{ConfigPath: configPath})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	m1.StartTask("task-1", "Task 1", "claude", "proj-1")
	m1.IncrementCounter("test_metric", 42, nil)

	// Save
	if err := m1.SaveToFile(configPath); err != nil {
		t.Fatalf("SaveToFile failed: %v", err)
	}

	// Stop first manager
	if err := m1.Stop(context.Background()); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Load into new manager
	m2, err := NewManager(Config{ConfigPath: configPath})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer m2.Stop(context.Background())

	// Verify metrics were loaded
	metrics := m2.GetMetrics("test_metric", nil, 0, 0)
	if len(metrics) != 1 {
		t.Errorf("Expected 1 metric, got %d", len(metrics))
	}

	// Verify task metrics were loaded
	taskMetric := m2.GetTaskMetrics("task-1")
	if taskMetric == nil {
		t.Error("Task metric not found after load")
	}
}

func TestMaxMetricsLimit(t *testing.T) {
	m, err := NewManager(Config{MaxMetrics: 10})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer m.Stop(context.Background())

	// Add more metrics than the limit
	for i := 0; i < 20; i++ {
		m.IncrementCounter("test", float64(i), nil)
	}

	// Should only have 10 metrics (the last 10)
	metrics := m.GetMetrics("test", nil, 0, 0)
	if len(metrics) != 10 {
		t.Errorf("Expected 10 metrics (max limit), got %d", len(metrics))
	}

	// The first metric should be from iteration 10, not 0
	if metrics[0].Value != 10 {
		t.Errorf("Expected first value to be 10, got %f", metrics[0].Value)
	}
}

func TestGetAllTaskMetrics(t *testing.T) {
	m, err := NewManager(Config{})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer m.Stop(context.Background())

	// Create multiple tasks
	for i := 1; i <= 3; i++ {
		taskID := fmt.Sprintf("task-%d", i)
		m.StartTask(taskID, fmt.Sprintf("Task %d", i), "claude", "proj-1")
		m.EndTask(taskID, "success", "")
	}

	// Get all metrics
	allMetrics := m.GetAllTaskMetrics()
	if len(allMetrics) != 3 {
		t.Errorf("Expected 3 task metrics, got %d", len(allMetrics))
	}
}
