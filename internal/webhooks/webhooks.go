// Package webhooks provides HTTP webhook notification system for task events
package webhooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

// EventType represents the type of event that triggered the webhook
type EventType string

const (
	EventTaskCreated  EventType = "task.created"
	EventTaskClaimed  EventType = "task.claimed"
	EventTaskStarted  EventType = "task.started"
	EventTaskPaused   EventType = "task.paused"
	EventTaskResumed  EventType = "task.resumed"
	EventTaskBlocked  EventType = "task.blocked"
	EventTaskCompleted EventType = "task.completed"
	EventTaskFailed   EventType = "task.failed"
	EventWorkerStarted EventType = "worker.started"
	EventWorkerStopped EventType = "worker.stopped"
)

// Webhook represents a configured webhook endpoint
type Webhook struct {
	ID        string            `json:"id"`
	URL       string            `json:"url"`
	Secret    string            `json:"secret,omitempty"` // HMAC secret for verification
	Events    []EventType       `json:"events"`           // Events to subscribe to
	Headers   map[string]string `json:"headers,omitempty"`
	Enabled   bool              `json:"enabled"`
	CreatedAt int64             `json:"created_at"`
}

// Payload represents the webhook payload sent to endpoints
type Payload struct {
	Event      EventType                 `json:"event"`
	Timestamp  int64                     `json:"timestamp"`
	WebhookID  string                    `json:"webhook_id"`
	DeliveryID string                    `json:"delivery_id"` // Unique ID for each delivery attempt
	Data       map[string]interface{}   `json:"data"`
}

// TaskEventData contains task-related event data
type TaskEventData struct {
	TaskID     string `json:"task_id"`
	Title      string `json:"title"`
	EpicID     string `json:"epic_id,omitempty"`
	Status     string `json:"status"`
	WorkerID   string `json:"worker_id,omitempty"`
	Error      string `json:"error,omitempty"`
	Attempts   int    `json:"attempts,omitempty"`
	DurationMS int64  `json:"duration_ms,omitempty"`
}

// WorkerEventData contains worker-related event data
type WorkerEventData struct {
	WorkerID    string `json:"worker_id"`
	WorkerIndex int    `json:"worker_index"`
	TaskCount   int    `json:"task_count"`
}

// DeliveryResult represents the result of a webhook delivery attempt
type DeliveryResult struct {
	WebhookID  string
	DeliveryID string
	Event      EventType
	StatusCode int
	Success    bool
	Error      string
	DurationMS int64
	Timestamp  int64
}

// Manager manages webhook registration and delivery
type Manager struct {
	mu       sync.RWMutex
	webhooks map[string]*Webhook
	logger   *log.Logger
	client   *http.Client
	delivery chan *DeliveryTask
	stopCh   chan struct{}
	wg       sync.WaitGroup

	// Delivery history (circular buffer)
	deliveryHistory []*DeliveryResult
	historyMutex    sync.Mutex
	historySize     int
	historyPos      int
}

// DeliveryTask represents a webhook delivery task
type DeliveryTask struct {
	webhook *Webhook
	payload *Payload
}

// NewManager creates a new webhook manager
func NewManager() *Manager {
	return &Manager{
		webhooks:        make(map[string]*Webhook),
		logger:          log.New(os.Stdout, "[webhooks] ", log.LstdFlags),
		client:          &http.Client{Timeout: 30 * time.Second},
		delivery:        make(chan *DeliveryTask, 1000),
		stopCh:          make(chan struct{}),
		deliveryHistory: make([]*DeliveryResult, 0, 100),
		historySize:     100,
	}
}

// SetLogger sets the logger for the webhook manager
func (m *Manager) SetLogger(logger *log.Logger) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logger = logger
}

// SetTimeout sets the HTTP client timeout
func (m *Manager) SetTimeout(timeout time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.client.Timeout = timeout
}

// Start begins processing webhook deliveries
func (m *Manager) Start(workers int) {
	m.logger.Printf("[webhooks] starting with %d workers", workers)

	for i := 0; i < workers; i++ {
		m.wg.Add(1)
		go m.deliveryWorker(i)
	}
}

// Stop gracefully shuts down the webhook manager
func (m *Manager) Stop(ctx context.Context) error {
	m.logger.Printf("[webhooks] stopping...")

	close(m.stopCh)

	// Wait for workers or timeout
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		m.logger.Printf("[webhooks] stopped")
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Register registers a new webhook
func (m *Manager) Register(webhook *Webhook) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if webhook.ID == "" {
		return fmt.Errorf("webhook ID is required")
	}

	if webhook.URL == "" {
		return fmt.Errorf("webhook URL is required")
	}

	if webhook.Events == nil {
		webhook.Events = []EventType{} // Subscribe to no events by default
	}

	webhook.CreatedAt = time.Now().Unix()
	m.webhooks[webhook.ID] = webhook

	m.logger.Printf("[webhooks] registered webhook %s -> %s", webhook.ID, webhook.URL)
	return nil
}

// Unregister removes a webhook
func (m *Manager) Unregister(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.webhooks[id]; !exists {
		return fmt.Errorf("webhook %s not found", id)
	}

	delete(m.webhooks, id)
	m.logger.Printf("[webhooks] unregistered webhook %s", id)
	return nil
}

// Get retrieves a webhook by ID
func (m *Manager) Get(id string) (*Webhook, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	webhook, exists := m.webhooks[id]
	if !exists {
		return nil, fmt.Errorf("webhook %s not found", id)
	}

	// Return a copy to avoid concurrent modification
	webhookCopy := *webhook
	return &webhookCopy, nil
}

// List returns all registered webhooks
func (m *Manager) List() []*Webhook {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Webhook, 0, len(m.webhooks))
	for _, webhook := range m.webhooks {
		webhookCopy := *webhook
		result = append(result, &webhookCopy)
	}
	return result
}

// Enable enables a webhook
func (m *Manager) Enable(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	webhook, exists := m.webhooks[id]
	if !exists {
		return fmt.Errorf("webhook %s not found", id)
	}

	webhook.Enabled = true
	m.logger.Printf("[webhooks] enabled webhook %s", id)
	return nil
}

// Disable disables a webhook
func (m *Manager) Disable(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	webhook, exists := m.webhooks[id]
	if !exists {
		return fmt.Errorf("webhook %s not found", id)
	}

	webhook.Enabled = false
	m.logger.Printf("[webhooks] disabled webhook %s", id)
	return nil
}

// Emit sends an event to all subscribed webhooks
func (m *Manager) Emit(event EventType, data map[string]interface{}) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, webhook := range m.webhooks {
		if !webhook.Enabled {
			continue
		}

		// Check if webhook is subscribed to this event
		if !m.isSubscribed(webhook, event) {
			continue
		}

		payload := &Payload{
			Event:      event,
			Timestamp:  time.Now().Unix(),
			WebhookID:  webhook.ID,
			DeliveryID: m.generateDeliveryID(),
			Data:       data,
		}

		// Non-blocking send
		select {
		case m.delivery <- &DeliveryTask{webhook: webhook, payload: payload}:
		default:
			m.logger.Printf("[webhooks] delivery queue full, dropping webhook %s", webhook.ID)
		}
	}
}

// EmitTaskCreated emits a task created event
func (m *Manager) EmitTaskCreated(taskID, title, epicID string) {
	m.Emit(EventTaskCreated, map[string]interface{}{
		"task": TaskEventData{
			TaskID: taskID,
			Title:  title,
			EpicID: epicID,
			Status: "created",
		},
	})
}

// EmitTaskClaimed emits a task claimed event
func (m *Manager) EmitTaskClaimed(taskID, title, workerID string) {
	m.Emit(EventTaskClaimed, map[string]interface{}{
		"task": TaskEventData{
			TaskID:   taskID,
			Title:    title,
			Status:   "claimed",
			WorkerID: workerID,
		},
	})
}

// EmitTaskStarted emits a task started event
func (m *Manager) EmitTaskStarted(taskID, title, workerID string) {
	m.Emit(EventTaskStarted, map[string]interface{}{
		"task": TaskEventData{
			TaskID:   taskID,
			Title:    title,
			Status:   "in_progress",
			WorkerID: workerID,
		},
	})
}

// EmitTaskPaused emits a task paused event
func (m *Manager) EmitTaskPaused(taskID, title string) {
	m.Emit(EventTaskPaused, map[string]interface{}{
		"task": TaskEventData{
			TaskID: taskID,
			Title:  title,
			Status: "paused",
		},
	})
}

// EmitTaskResumed emits a task resumed event
func (m *Manager) EmitTaskResumed(taskID, title string) {
	m.Emit(EventTaskResumed, map[string]interface{}{
		"task": TaskEventData{
			TaskID: taskID,
			Title:  title,
			Status: "ready",
		},
	})
}

// EmitTaskBlocked emits a task blocked event
func (m *Manager) EmitTaskBlocked(taskID, title string) {
	m.Emit(EventTaskBlocked, map[string]interface{}{
		"task": TaskEventData{
			TaskID: taskID,
			Title:  title,
			Status: "blocked",
		},
	})
}

// EmitTaskCompleted emits a task completed event
func (m *Manager) EmitTaskCompleted(taskID, title string, durationMS int64) {
	m.Emit(EventTaskCompleted, map[string]interface{}{
		"task": TaskEventData{
			TaskID:    taskID,
			Title:     title,
			Status:    "completed",
			DurationMS: durationMS,
		},
	})
}

// EmitTaskFailed emits a task failed event
func (m *Manager) EmitTaskFailed(taskID, title, errorMsg string, attempts int) {
	m.Emit(EventTaskFailed, map[string]interface{}{
		"task": TaskEventData{
			TaskID:   taskID,
			Title:    title,
			Status:   "failed",
			Error:    errorMsg,
			Attempts: attempts,
		},
	})
}

// EmitWorkerStarted emits a worker started event
func (m *Manager) EmitWorkerStarted(workerID string, workerIndex int) {
	m.Emit(EventWorkerStarted, map[string]interface{}{
		"worker": WorkerEventData{
			WorkerID:    workerID,
			WorkerIndex: workerIndex,
		},
	})
}

// EmitWorkerStopped emits a worker stopped event
func (m *Manager) EmitWorkerStopped(workerID string, workerIndex, taskCount int) {
	m.Emit(EventWorkerStopped, map[string]interface{}{
		"worker": WorkerEventData{
			WorkerID:    workerID,
			WorkerIndex: workerIndex,
			TaskCount:   taskCount,
		},
	})
}

// GetDeliveryHistory returns recent delivery results
func (m *Manager) GetDeliveryHistory(limit int) []*DeliveryResult {
	m.historyMutex.Lock()
	defer m.historyMutex.Unlock()

	if limit <= 0 || limit > len(m.deliveryHistory) {
		limit = len(m.deliveryHistory)
	}

	result := make([]*DeliveryResult, limit)
	// Copy from oldest to newest within the limit
	start := (m.historyPos - limit + len(m.deliveryHistory)) % len(m.deliveryHistory)
	for i := 0; i < limit; i++ {
		pos := (start + i) % len(m.deliveryHistory)
		result[i] = m.deliveryHistory[pos]
	}
	return result
}

// isSubscribed checks if a webhook is subscribed to an event
func (m *Manager) isSubscribed(webhook *Webhook, event EventType) bool {
	if len(webhook.Events) == 0 {
		// Empty event list means subscribe to all events
		return true
	}

	for _, e := range webhook.Events {
		if e == event {
			return true
		}
	}
	return false
}

// deliveryWorker processes webhook deliveries
func (m *Manager) deliveryWorker(workerID int) {
	defer m.wg.Done()

	for {
		select {
		case task := <-m.delivery:
			m.deliver(task)
		case <-m.stopCh:
			return
		}
	}
}

// deliver sends a webhook payload to its endpoint
func (m *Manager) deliver(task *DeliveryTask) {
	start := time.Now()

	result := &DeliveryResult{
		WebhookID:  task.webhook.ID,
		DeliveryID: task.payload.DeliveryID,
		Event:      task.payload.Event,
		Success:    false,
		Timestamp:  start.Unix(),
	}

	// Marshal payload
	body, err := json.Marshal(task.payload)
	if err != nil {
		result.Error = fmt.Sprintf("failed to marshal payload: %v", err)
		m.recordDelivery(result)
		m.logger.Printf("[webhooks] %s", result.Error)
		return
	}

	// Create request
	req, err := http.NewRequest("POST", task.webhook.URL, bytes.NewReader(body))
	if err != nil {
		result.Error = fmt.Sprintf("failed to create request: %v", err)
		m.recordDelivery(result)
		m.logger.Printf("[webhooks] %s", result.Error)
		return
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Drover-Webhooks/1.0")
	req.Header.Set("X-Webhook-ID", task.webhook.ID)
	req.Header.Set("X-Webhook-Delivery-ID", task.payload.DeliveryID)
	req.Header.Set("X-Webhook-Timestamp", fmt.Sprintf("%d", task.payload.Timestamp))
	req.Header.Set("X-Webhook-Event", string(task.payload.Event))

	// Add custom headers
	for k, v := range task.webhook.Headers {
		req.Header.Set(k, v)
	}

	// Add signature if secret is configured
	if task.webhook.Secret != "" {
		signature := m.sign(body, task.webhook.Secret)
		req.Header.Set("X-Webhook-Signature", "sha256="+signature)
	}

	// Send request
	resp, err := m.client.Do(req)
	if err != nil {
		result.Error = fmt.Sprintf("request failed: %v", err)
		m.recordDelivery(result)
		m.logger.Printf("[webhooks] %s delivery to %s failed: %v", task.payload.Event, task.webhook.URL, err)
		return
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode
	result.Success = resp.StatusCode >= 200 && resp.StatusCode < 300
	result.DurationMS = time.Since(start).Milliseconds()

	if !result.Success {
		result.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		m.logger.Printf("[webhooks] %s delivery to %s failed: HTTP %d", task.payload.Event, task.webhook.URL, resp.StatusCode)
	} else {
		m.logger.Printf("[webhooks] %s delivered to %s (HTTP %d, %dms)", task.payload.Event, task.webhook.URL, resp.StatusCode, result.DurationMS)
	}

	m.recordDelivery(result)
}

// sign creates an HMAC signature for the payload
func (m *Manager) sign(payload []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

// recordDelivery records a delivery result in the history buffer
func (m *Manager) recordDelivery(result *DeliveryResult) {
	m.historyMutex.Lock()
	defer m.historyMutex.Unlock()

	if len(m.deliveryHistory) < m.historySize {
		m.deliveryHistory = append(m.deliveryHistory, result)
	} else {
		m.deliveryHistory[m.historyPos] = result
		m.historyPos = (m.historyPos + 1) % m.historySize
	}
}

// generateDeliveryID generates a unique delivery ID
func (m *Manager) generateDeliveryID() string {
	return fmt.Sprintf("%d-%s", time.Now().UnixNano(), randomString(8))
}

// randomString generates a random alphanumeric string
func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}

// VerifySignature verifies an HMAC signature
func VerifySignature(payload []byte, signature, secret string) bool {
	expected := sign(payload, secret)
	return hmac.Equal([]byte(signature), []byte(expected))
}

func sign(payload []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}
