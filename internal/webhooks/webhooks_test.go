package webhooks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestWebhookManager tests basic webhook manager functionality
func TestWebhookManager(t *testing.T) {
	m := NewManager()

	// Create test server to receive webhooks.
	// mu guards receivedPayload and receivedHeaders which are written by the
	// HTTP server goroutine and read by the test goroutine.
	var mu sync.Mutex
	var receivedPayload *Payload
	receivedHeaders := make(map[string]string)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Capture headers
		mu.Lock()
		defer mu.Unlock()
		for k, v := range r.Header {
			if strings.HasPrefix(k, "X-Webhook-") || strings.HasPrefix(k, "X-") {
				receivedHeaders[k] = v[0]
			}
		}

		// Decode payload
		var payload Payload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("Failed to decode payload: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		receivedPayload = &payload

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Register webhook
	webhook := &Webhook{
		ID:      "test-webhook",
		URL:     server.URL,
		Secret:  "test-secret",
		Events:  []EventType{EventTaskCreated, EventTaskCompleted},
		Enabled: true,
	}

	if err := m.Register(webhook); err != nil {
		t.Fatalf("Failed to register webhook: %v", err)
	}

	// Start webhook manager
	m.Start(1)
	defer m.Stop(context.Background())

	// Emit event
	m.EmitTaskCreated("task-1", "Test Task", "epic-1")

	// Wait for delivery
	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	gotPayload := receivedPayload
	gotHeaders := make(map[string]string, len(receivedHeaders))
	for k, v := range receivedHeaders {
		gotHeaders[k] = v
	}
	mu.Unlock()

	// Verify payload
	if gotPayload == nil {
		t.Fatal("No payload received")
	}

	if gotPayload.Event != EventTaskCreated {
		t.Errorf("Expected event %s, got %s", EventTaskCreated, gotPayload.Event)
	}

	// Verify headers
	if gotHeaders["X-Webhook-Id"] != "test-webhook" {
		t.Errorf("Expected X-Webhook-Id header 'test-webhook', got %s", gotHeaders["X-Webhook-Id"])
	}

	if gotHeaders["X-Webhook-Event"] != string(EventTaskCreated) {
		t.Errorf("Expected X-Webhook-Event %s, got %s", EventTaskCreated, gotHeaders["X-Webhook-Event"])
	}

	// Verify signature
	signature := gotHeaders["X-Webhook-Signature"]
	if !strings.HasPrefix(signature, "sha256=") {
		t.Errorf("Expected signature to start with 'sha256=', got %s", signature)
	}
}

// TestWebhookSignature tests HMAC signature verification
func TestWebhookSignature(t *testing.T) {
	secret := "test-secret"
	payload := []byte(`{"test": "data"}`)

	// Generate signature
	sig := sign(payload, secret)

	// Verify signature
	if !VerifySignature(payload, sig, secret) {
		t.Error("Signature verification failed")
	}

	// Test wrong secret
	if VerifySignature(payload, sig, "wrong-secret") {
		t.Error("Signature should fail with wrong secret")
	}

	// Test tampered payload
	if VerifySignature([]byte(`{"test": "tampered"}`), sig, secret) {
		t.Error("Signature should fail with tampered payload")
	}
}

// TestWebhookFiltering tests event filtering
func TestWebhookFiltering(t *testing.T) {
	m := NewManager()

	// receivedEvent is written by the HTTP handler goroutine and read by the
	// test goroutine; guard with a mutex.
	var mu sync.Mutex
	var receivedEvent EventType
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload Payload
		json.NewDecoder(r.Body).Decode(&payload)
		mu.Lock()
		receivedEvent = payload.Event
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Register webhook that only listens to task.completed events
	webhook := &Webhook{
		ID:      "filtered-webhook",
		URL:     server.URL,
		Events:  []EventType{EventTaskCompleted},
		Enabled: true,
	}

	m.Register(webhook)
	m.Start(1)
	defer m.Stop(context.Background())

	// Emit task.created event (should not be delivered)
	m.EmitTaskCreated("task-1", "Test", "epic-1")
	time.Sleep(100 * time.Millisecond)

	// Emit task.completed event (should be delivered)
	m.EmitTaskCompleted("task-1", "Test", 1000)
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	got := receivedEvent
	mu.Unlock()
	if got != EventTaskCompleted {
		t.Errorf("Expected only EventTaskCompleted to be delivered, got %s", got)
	}
}

// TestWebhookDeliveryHistory tests delivery history tracking
func TestWebhookDeliveryHistory(t *testing.T) {
	m := NewManager()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	webhook := &Webhook{
		ID:      "history-webhook",
		URL:     server.URL,
		Enabled: true,
	}

	m.Register(webhook)
	m.Start(1)
	defer m.Stop(context.Background())

	// Emit several events
	for i := 0; i < 5; i++ {
		m.EmitTaskCreated("task-"+string(rune(i)), "Test", "epic-1")
	}

	time.Sleep(500 * time.Millisecond)

	// Get history
	history := m.GetDeliveryHistory(10)
	if len(history) < 5 {
		t.Errorf("Expected at least 5 delivery results, got %d", len(history))
	}

	// Verify all were successful
	for _, result := range history {
		if !result.Success {
			t.Errorf("Expected successful delivery, got error: %s", result.Error)
		}
	}
}

// TestWebhookDisable tests enabling/disabling webhooks
func TestWebhookDisable(t *testing.T) {
	m := NewManager()

	var callCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	webhook := &Webhook{
		ID:      "disable-test",
		URL:     server.URL,
		Enabled: true,
	}

	m.Register(webhook)
	m.Start(1)
	defer m.Stop(context.Background())

	// Emit while enabled
	m.EmitTaskCreated("task-1", "Test", "epic-1")
	time.Sleep(100 * time.Millisecond)

	if n := callCount.Load(); n != 1 {
		t.Errorf("Expected 1 call while enabled, got %d", n)
	}

	// Disable webhook
	m.Disable("disable-test")

	// Emit while disabled
	m.EmitTaskCreated("task-2", "Test", "epic-2")
	time.Sleep(100 * time.Millisecond)

	if n := callCount.Load(); n != 1 {
		t.Errorf("Expected no additional calls while disabled, got %d", n)
	}

	// Re-enable
	m.Enable("disable-test")

	// Emit while re-enabled
	m.EmitTaskCreated("task-3", "Test", "epic-3")
	time.Sleep(100 * time.Millisecond)

	if n := callCount.Load(); n != 2 {
		t.Errorf("Expected 2 calls total (before disable + after enable), got %d", n)
	}
}
