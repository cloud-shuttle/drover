// Package analytics provides metrics tracking and visualization
package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// MetricType defines the type of metric
type MetricType string

const (
	MetricTypeCounter   MetricType = "counter"
	MetricTypeGauge     MetricType = "gauge"
	MetricTypeHistogram MetricType = "histogram"
	MetricTypeSummary   MetricType = "summary"
)

// Metric represents a single metric data point
type Metric struct {
	Name      string                 `json:"name"`
	Type      MetricType             `json:"type"`
	Value     float64                `json:"value"`
	Timestamp int64                  `json:"timestamp"`
	Labels    map[string]string      `json:"labels,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// TaskMetrics tracks metrics for a specific task execution
type TaskMetrics struct {
	TaskID       string                 `json:"task_id"`
	Title        string                 `json:"title"`
	AgentType    string                 `json:"agent_type"`
	ProjectID    string                 `json:"project_id"`
	StartTime    int64                  `json:"start_time"`
	EndTime      int64                  `json:"end_time,omitempty"`
	Duration     int64                  `json:"duration_ms"`
	Status       string                 `json:"status"`
	Error        string                 `json:"error,omitempty"`
	TokenCount   int                    `json:"token_count"`
	LLMCalls     int                    `json:"llm_calls"`
	FilesCreated int                    `json:"files_created"`
	FilesModified int                   `json:"files_modified"`
	FilesDeleted int                    `json:"files_deleted"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// AggregatedMetrics represents aggregated metrics over a time period
type AggregatedMetrics struct {
	StartTime    int64     `json:"start_time"`
	EndTime      int64     `json:"end_time"`
	TotalTasks   int       `json:"total_tasks"`
	SuccessTasks int       `json:"success_tasks"`
	FailedTasks  int       `json:"failed_tasks"`
	AvgDuration  float64   `json:"avg_duration_ms"`
	TotalTokens  int       `json:"total_tokens"`
	TotalLLMCalls int      `json:"total_llm_calls"`
	ByAgentType  map[string]int `json:"by_agent_type"`
	ByStatus     map[string]int `json:"by_status"`
}

// TimeSeriesData represents a time series data point
type TimeSeriesData struct {
	Timestamp int64   `json:"timestamp"`
	Value     float64 `json:"value"`
	Labels    map[string]string `json:"labels,omitempty"`
}

// Manager manages analytics and metrics collection
type Manager struct {
	mu            sync.RWMutex
	metrics       []*Metric
	taskMetrics   []*TaskMetrics
	logger        *log.Logger
	configPath    string
	maxMetrics    int
	flushInterval time.Duration
	stopCh        chan struct{}
	wg            sync.WaitGroup
}

// Config holds manager configuration
type Config struct {
	ConfigPath    string
	Logger        *log.Logger
	MaxMetrics    int           // Maximum metrics to keep in memory
	FlushInterval time.Duration // How often to flush metrics to disk
}

// NewManager creates a new analytics manager
func NewManager(cfg Config) (*Manager, error) {
	if cfg.MaxMetrics == 0 {
		cfg.MaxMetrics = 10000
	}
	if cfg.FlushInterval == 0 {
		cfg.FlushInterval = 5 * time.Minute
	}

	m := &Manager{
		metrics:       make([]*Metric, 0, cfg.MaxMetrics),
		taskMetrics:   make([]*TaskMetrics, 0, 1000),
		logger:        cfg.Logger,
		configPath:    cfg.ConfigPath,
		maxMetrics:    cfg.MaxMetrics,
		flushInterval: cfg.FlushInterval,
		stopCh:        make(chan struct{}),
	}

	if m.logger == nil {
		m.logger = log.New(os.Stdout, "[analytics] ", log.LstdFlags)
	}

	// Load existing metrics if config path exists
	if cfg.ConfigPath != "" {
		if err := m.LoadFromFile(cfg.ConfigPath); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("load analytics: %w", err)
		}
	}

	// Start background flusher
	m.startFlusher()

	m.logger.Printf("[analytics] manager initialized (maxMetrics=%d, flushInterval=%v)",
		cfg.MaxMetrics, cfg.FlushInterval)

	return m, nil
}

// SetLogger sets the logger for the manager
func (m *Manager) SetLogger(logger *log.Logger) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logger = logger
}

// RecordMetric records a single metric
func (m *Manager) RecordMetric(metric *Metric) {
	m.mu.Lock()
	defer m.mu.Unlock()

	metric.Timestamp = time.Now().Unix()
	m.metrics = append(m.metrics, metric)

	// Trim if we exceed max metrics
	if len(m.metrics) > m.maxMetrics {
		m.metrics = m.metrics[len(m.metrics)-m.maxMetrics:]
	}
}

// IncrementCounter increments a counter metric
func (m *Manager) IncrementCounter(name string, value float64, labels map[string]string) {
	m.RecordMetric(&Metric{
		Name:  name,
		Type:  MetricTypeCounter,
		Value: value,
		Labels: labels,
	})
}

// SetGauge sets a gauge metric
func (m *Manager) SetGauge(name string, value float64, labels map[string]string) {
	m.RecordMetric(&Metric{
		Name:  name,
		Type:  MetricTypeGauge,
		Value: value,
		Labels: labels,
	})
}

// RecordHistogram records a histogram metric
func (m *Manager) RecordHistogram(name string, value float64, labels map[string]string) {
	m.RecordMetric(&Metric{
		Name:  name,
		Type:  MetricTypeHistogram,
		Value: value,
		Labels: labels,
	})
}

// StartTask starts tracking a task execution
func (m *Manager) StartTask(taskID, title, agentType, projectID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	taskMetric := &TaskMetrics{
		TaskID:    taskID,
		Title:     title,
		AgentType: agentType,
		ProjectID: projectID,
		StartTime: time.Now().Unix(),
		Status:    "running",
		Metadata:  make(map[string]interface{}),
	}

	m.taskMetrics = append(m.taskMetrics, taskMetric)
	m.logger.Printf("[analytics] started tracking task: %s", taskID)
}

// EndTask completes tracking a task execution
func (m *Manager) EndTask(taskID string, status string, errorMsg string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, tm := range m.taskMetrics {
		if tm.TaskID == taskID && tm.EndTime == 0 {
			tm.EndTime = time.Now().Unix()
			tm.Duration = tm.EndTime - tm.StartTime
			tm.Status = status
			if errorMsg != "" {
				tm.Error = errorMsg
			}

			// Record completion metrics
			m.metrics = append(m.metrics, &Metric{
				Name:  "task_duration_ms",
				Type:  MetricTypeHistogram,
				Value: float64(tm.Duration),
				Labels: map[string]string{
					"agent_type": tm.AgentType,
					"status":     tm.Status,
					"project_id": tm.ProjectID,
				},
			})

			m.metrics = append(m.metrics, &Metric{
				Name:  "task_completed",
				Type:  MetricTypeCounter,
				Value: 1,
				Labels: map[string]string{
					"agent_type": tm.AgentType,
					"status":     tm.Status,
					"project_id": tm.ProjectID,
				},
			})

			m.logger.Printf("[analytics] completed task: %s (status=%s, duration=%dms)",
				taskID, status, tm.Duration)
			return
		}
	}
}

// UpdateTaskTokens updates token usage for a task
func (m *Manager) UpdateTaskTokens(taskID string, tokenCount int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, tm := range m.taskMetrics {
		if tm.TaskID == taskID && tm.EndTime == 0 {
			tm.TokenCount = tokenCount

			m.metrics = append(m.metrics, &Metric{
				Name:  "llm_tokens_used",
				Type:  MetricTypeCounter,
				Value: float64(tokenCount),
				Labels: map[string]string{
					"agent_type": tm.AgentType,
					"project_id": tm.ProjectID,
				},
			})
			return
		}
	}
}

// IncrementTaskLLMCalls increments the LLM call count for a task
func (m *Manager) IncrementTaskLLMCalls(taskID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, tm := range m.taskMetrics {
		if tm.TaskID == taskID && tm.EndTime == 0 {
			tm.LLMCalls++

			m.metrics = append(m.metrics, &Metric{
				Name:  "llm_calls",
				Type:  MetricTypeCounter,
				Value: 1,
				Labels: map[string]string{
					"agent_type": tm.AgentType,
					"project_id": tm.ProjectID,
				},
			})
			return
		}
	}
}

// UpdateTaskFiles updates file operation counts for a task
func (m *Manager) UpdateTaskFiles(taskID string, created, modified, deleted int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, tm := range m.taskMetrics {
		if tm.TaskID == taskID && tm.EndTime == 0 {
			tm.FilesCreated += created
			tm.FilesModified += modified
			tm.FilesDeleted += deleted
			return
		}
	}
}

// GetMetrics retrieves metrics matching filters
func (m *Manager) GetMetrics(name string, labels map[string]string, startTime, endTime int64) []*Metric {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*Metric
	for _, metric := range m.metrics {
		// Filter by name
		if name != "" && metric.Name != name {
			continue
		}

		// Filter by time range
		if startTime > 0 && metric.Timestamp < startTime {
			continue
		}
		if endTime > 0 && metric.Timestamp > endTime {
			continue
		}

		// Filter by labels
		if len(labels) > 0 {
			match := true
			for k, v := range labels {
				if metric.Labels == nil || metric.Labels[k] != v {
					match = false
					break
				}
			}
			if !match {
				continue
			}
		}

		result = append(result, metric)
	}

	return result
}

// GetTaskMetrics retrieves task metrics
func (m *Manager) GetTaskMetrics(taskID string) *TaskMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, tm := range m.taskMetrics {
		if tm.TaskID == taskID {
			copy := *tm
			return &copy
		}
	}
	return nil
}

// GetAllTaskMetrics retrieves all task metrics
func (m *Manager) GetAllTaskMetrics() []*TaskMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*TaskMetrics, len(m.taskMetrics))
	for i, tm := range m.taskMetrics {
		copy := *tm
		result[i] = &copy
	}
	return result
}

// AggregateMetrics aggregates metrics over a time period
func (m *Manager) AggregateMetrics(startTime, endTime int64) *AggregatedMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agg := &AggregatedMetrics{
		StartTime:    startTime,
		EndTime:      endTime,
		ByAgentType:  make(map[string]int),
		ByStatus:     make(map[string]int),
	}

	totalDuration := int64(0)

	for _, tm := range m.taskMetrics {
		if tm.StartTime < startTime || tm.StartTime > endTime {
			continue
		}

		agg.TotalTasks++
		agg.ByAgentType[tm.AgentType]++
		agg.ByStatus[tm.Status]++

		if tm.Status == "success" {
			agg.SuccessTasks++
		} else if tm.Status == "failed" {
			agg.FailedTasks++
		}

		totalDuration += tm.Duration
		agg.TotalTokens += tm.TokenCount
		agg.TotalLLMCalls += tm.LLMCalls
	}

	if agg.TotalTasks > 0 {
		agg.AvgDuration = float64(totalDuration) / float64(agg.TotalTasks)
	}

	return agg
}

// GetTimeSeries retrieves time series data for a metric
func (m *Manager) GetTimeSeries(name string, labels map[string]string, startTime, endTime int64, interval time.Duration) []*TimeSeriesData {
	metrics := m.GetMetrics(name, labels, startTime, endTime)

	if len(metrics) == 0 {
		return nil
	}

	// Create buckets based on interval
	buckets := make(map[int64][]float64)
	for _, metric := range metrics {
		bucketTime := (metric.Timestamp / int64(interval.Seconds())) * int64(interval.Seconds())
		buckets[bucketTime] = append(buckets[bucketTime], metric.Value)
	}

	// Aggregate each bucket
	result := make([]*TimeSeriesData, 0, len(buckets))
	for bucketTime, values := range buckets {
		var avg float64
		for _, v := range values {
			avg += v
		}
		avg /= float64(len(values))

		result = append(result, &TimeSeriesData{
			Timestamp: bucketTime,
			Value:     avg,
			Labels:    labels,
		})
	}

	return result
}

// LoadFromFile loads metrics from a JSON file
func (m *Manager) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var storage struct {
		Metrics     []*Metric      `json:"metrics"`
		TaskMetrics []*TaskMetrics `json:"task_metrics"`
	}

	if err := json.Unmarshal(data, &storage); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.metrics = storage.Metrics
	m.taskMetrics = storage.TaskMetrics

	m.logger.Printf("[analytics] loaded %d metrics and %d task metrics from %s",
		len(m.metrics), len(m.taskMetrics), path)
	return nil
}

// SaveToFile saves metrics to a JSON file
func (m *Manager) SaveToFile(path string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	storage := struct {
		Metrics     []*Metric      `json:"metrics"`
		TaskMetrics []*TaskMetrics `json:"task_metrics"`
	}{
		Metrics:     m.metrics,
		TaskMetrics: m.taskMetrics,
	}

	data, err := json.MarshalIndent(storage, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// startFlusher starts the background flusher
func (m *Manager) startFlusher() {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()

		ticker := time.NewTicker(m.flushInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if m.configPath != "" {
					if err := m.SaveToFile(m.configPath); err != nil {
						m.logger.Printf("[analytics] flush error: %v", err)
					}
				}
			case <-m.stopCh:
				return
			}
		}
	}()
}

// Stop stops the analytics manager
func (m *Manager) Stop(ctx context.Context) error {
	close(m.stopCh)
	m.wg.Wait()

	// Final flush
	if m.configPath != "" {
		if err := m.SaveToFile(m.configPath); err != nil {
			return err
		}
	}

	m.logger.Printf("[analytics] manager stopped")
	return nil
}
