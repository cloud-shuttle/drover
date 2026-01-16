package callbacks

import (
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"sync"
	"sync/atomic"
)

// MetricType represents the type of Prometheus metric
type MetricType string

const (
	MetricTypeCounter   MetricType = "counter"
	MetricTypeGauge     MetricType = "gauge"
	MetricTypeHistogram MetricType = "histogram"
)

// Metric represents a single metric
type Metric struct {
	Type   MetricType
	Name   string
	Help   string
	Value  float64
	intVal int64 // For atomic operations on counters
	Labels map[string]string
}

// MetricsCallback implements Callback with Prometheus-compatible metrics tracking.
// It maintains counters, gauges, and histograms for workforce lifecycle events.
type MetricsCallback struct {
	mu     sync.RWMutex
	metrics map[string]*Metric
	logger *log.Logger
}

// NewMetricsCallback creates a new metrics callback
func NewMetricsCallback() *MetricsCallback {
	return &MetricsCallback{
		metrics: make(map[string]*Metric),
		logger:  log.New(os.Stdout, "[metrics] ", log.LstdFlags),
	}
}

// SetLogger sets the logger for the metrics callback
func (m *MetricsCallback) SetLogger(logger *log.Logger) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logger = logger
}

// RegisterMetric registers a new metric
func (m *MetricsCallback) RegisterMetric(metric *Metric) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metrics[metric.Name] = metric
}

// Increment increments a counter metric
func (m *MetricsCallback) Increment(name string, labels map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.metricKey(name, labels)
	if metric, exists := m.metrics[key]; exists {
		metric.intVal++
		metric.Value = float64(metric.intVal)
	} else {
		m.metrics[key] = &Metric{
			Type:   MetricTypeCounter,
			Name:   name,
			Help:   fmt.Sprintf("Counter metric: %s", name),
			Value:  1,
			intVal: 1,
			Labels: labels,
		}
	}
}

// Gauge sets a gauge metric value
func (m *MetricsCallback) Gauge(name string, value float64, labels map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.metricKey(name, labels)
	m.metrics[key] = &Metric{
		Type:   MetricTypeGauge,
		Name:   name,
		Help:   fmt.Sprintf("Gauge metric: %s", name),
		Value:  value,
		Labels: labels,
	}
}

// Histogram records a value in a histogram metric
func (m *MetricsCallback) Histogram(name string, value float64, labels map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.metricKey(name, labels)
	if metric, exists := m.metrics[key]; exists {
		// Simple histogram: just track count and sum
		// A full implementation would track buckets
		metric.Value += value
	} else {
		m.metrics[key] = &Metric{
			Type:   MetricTypeHistogram,
			Name:   name,
			Help:   fmt.Sprintf("Histogram metric: %s", name),
			Value:  value,
			Labels: labels,
		}
	}
}

// metricKey generates a unique key for a metric with labels
func (m *MetricsCallback) metricKey(name string, labels map[string]string) string {
	key := name

	// Sort label keys for deterministic key generation
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		key += fmt.Sprintf(";%s=%s", k, labels[k])
	}
	return key
}

// WritePrometheus writes metrics in Prometheus text format
func (m *MetricsCallback) WritePrometheus(w io.Writer) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Group metrics by name (ignore labels for grouping)
	grouped := make(map[string][]*Metric)
	for _, metric := range m.metrics {
		grouped[metric.Name] = append(grouped[metric.Name], metric)
	}

	// Write each metric group
	for name, metrics := range grouped {
		metric := metrics[0] // All have same type and help

		// Write HELP and TYPE
		fmt.Fprintf(w, "# HELP %s %s\n", name, metric.Help)
		fmt.Fprintf(w, "# TYPE %s %s\n", name, metric.Type)

		// Write each labeled metric
		for _, m := range metrics {
			labels := formatLabels(m.Labels)
			fmt.Fprintf(w, "%s%s %v\n", name, labels, m.Value)
		}

		// Empty line between metrics
		fmt.Fprintln(w)
	}

	return nil
}

// formatLabels formats labels for Prometheus format
func formatLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}

	result := "{"
	first := true
	for k, v := range labels {
		if !first {
			result += ","
		}
		result += fmt.Sprintf(`%s="%s"`, k, escapeLabelValue(v))
		first = false
	}
	result += "}"
	return result
}

// escapeLabelValue escapes special characters in label values
func escapeLabelValue(v string) string {
	// Escape backslashes, double quotes, and newlines
	result := ""
	for _, c := range v {
		switch c {
		case '\\':
			result += "\\\\"
		case '"':
			result += "\\\""
		case '\n':
			result += "\\n"
		default:
			result += string(c)
		}
	}
	return result
}

// GetMetric returns a metric by name and labels
func (m *MetricsCallback) GetMetric(name string, labels map[string]string) (*Metric, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := m.metricKey(name, labels)
	metric, exists := m.metrics[key]
	return metric, exists
}

// GetAllMetrics returns all metrics
func (m *MetricsCallback) GetAllMetrics() map[string]*Metric {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy to avoid concurrent modification
	result := make(map[string]*Metric, len(m.metrics))
	for k, v := range m.metrics {
		metricCopy := *v
		result[k] = &metricCopy
	}
	return result
}

// Reset clears all metrics
func (m *MetricsCallback) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.metrics = make(map[string]*Metric)
}

// Task lifecycle callbacks with metric tracking

// OnTaskCreated implements Callback
func (m *MetricsCallback) OnTaskCreated(ctx *TaskEventContext) error {
	labels := map[string]string{
		"epic": ctx.EpicID,
		"type": ctx.Type,
	}
	m.Increment("drover_tasks_created_total", labels)
	return nil
}

// OnTaskAssigned implements Callback
func (m *MetricsCallback) OnTaskAssigned(ctx *TaskEventContext) error {
	labels := map[string]string{
		"worker_id": ctx.WorkerID,
		"epic":      ctx.EpicID,
	}
	m.Increment("drover_tasks_assigned_total", labels)
	return nil
}

// OnTaskStarted implements Callback
func (m *MetricsCallback) OnTaskStarted(ctx *TaskEventContext) error {
	labels := map[string]string{
		"worker_id": ctx.WorkerID,
		"epic":      ctx.EpicID,
		"type":      ctx.Type,
	}
	m.Increment("drover_tasks_started_total", labels)
	return nil
}

// OnTaskCompleted implements Callback
func (m *MetricsCallback) OnTaskCompleted(ctx *TaskEventContext) error {
	labels := map[string]string{
		"worker_id": ctx.WorkerID,
		"epic":      ctx.EpicID,
		"type":      ctx.Type,
	}
	m.Increment("drover_tasks_completed_total", labels)

	// Track duration if available
	if ctx.Duration != nil {
		durationLabels := map[string]string{
			"epic": ctx.EpicID,
			"type": ctx.Type,
		}
		m.Histogram("drover_task_duration_seconds", ctx.Duration.Seconds(), durationLabels)
	}

	return nil
}

// OnTaskFailed implements Callback
func (m *MetricsCallback) OnTaskFailed(ctx *TaskEventContext) error {
	labels := map[string]string{
		"worker_id":      ctx.WorkerID,
		"epic":           ctx.EpicID,
		"type":           ctx.Type,
		"error_category": ctx.ErrorCategory,
	}
	m.Increment("drover_tasks_failed_total", labels)

	// Track attempt count
	attemptLabels := map[string]string{
		"epic": ctx.EpicID,
		"type": ctx.Type,
	}
	m.Histogram("drover_task_attempts", float64(ctx.Attempt), attemptLabels)

	return nil
}

// OnTaskBlocked implements Callback
func (m *MetricsCallback) OnTaskBlocked(ctx *TaskEventContext) error {
	labels := map[string]string{
		"epic": ctx.EpicID,
	}
	m.Increment("drover_tasks_blocked_total", labels)

	// Track currently blocked tasks
	m.Gauge("drover_tasks_blocked_current", float64(m.incrementBlockedCount()), labels)

	return nil
}

// OnTaskUnblocked implements Callback
func (m *MetricsCallback) OnTaskUnblocked(ctx *TaskEventContext) error {
	labels := map[string]string{
		"epic": ctx.EpicID,
	}
	m.Increment("drover_tasks_unblocked_total", labels)

	// Track currently blocked tasks
	m.Gauge("drover_tasks_blocked_current", float64(m.decrementBlockedCount()), labels)

	return nil
}

// OnTaskRecovered implements Callback
func (m *MetricsCallback) OnTaskRecovered(ctx *RecoveryEventContext) error {
	labels := map[string]string{
		"strategy": ctx.Strategy,
	}
	m.Increment("drover_tasks_recovered_total", labels)
	return nil
}

// Worker lifecycle callbacks with metric tracking

// OnWorkerStarted implements Callback
func (m *MetricsCallback) OnWorkerStarted(ctx *WorkerEventContext) error {
	m.Increment("drover_workers_started_total", nil)

	// Track active workers
	m.Gauge("drover_workers_active", float64(m.incrementActiveWorkers()), nil)

	return nil
}

// OnWorkerStopped implements Callback
func (m *MetricsCallback) OnWorkerStopped(ctx *WorkerEventContext) error {
	m.Increment("drover_workers_stopped_total", nil)

	// Track active workers
	m.Gauge("drover_workers_active", float64(m.decrementActiveWorkers()), nil)

	return nil
}

// OnWorkerStalled implements Callback
func (m *MetricsCallback) OnWorkerStalled(ctx *WorkerEventContext) error {
	labels := map[string]string{
		"reason": ctx.StallReason,
	}
	m.Increment("drover_workers_stalled_total", labels)
	return nil
}

// Atomic counters for gauges
var (
	activeWorkersCount int64
	blockedTasksCount  int64
)

func (m *MetricsCallback) incrementActiveWorkers() int64 {
	return atomic.AddInt64(&activeWorkersCount, 1)
}

func (m *MetricsCallback) decrementActiveWorkers() int64 {
	return atomic.AddInt64(&activeWorkersCount, -1)
}

func (m *MetricsCallback) incrementBlockedCount() int64 {
	return atomic.AddInt64(&blockedTasksCount, 1)
}

func (m *MetricsCallback) decrementBlockedCount() int64 {
	return atomic.AddInt64(&blockedTasksCount, -1)
}
