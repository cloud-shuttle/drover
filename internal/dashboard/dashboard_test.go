package dashboard

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	_ "github.com/glebarez/go-sqlite"
)

// ============================================================================
// Test Helpers
// ============================================================================

// newTestDB creates an in-memory SQLite database with schema.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	schema := `
	CREATE TABLE IF NOT EXISTS epics (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		description TEXT,
		status TEXT DEFAULT 'open',
		created_at INTEGER NOT NULL
	);
	CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		description TEXT,
		epic_id TEXT,
		parent_id TEXT,
		sequence_number INTEGER DEFAULT 0,
		priority INTEGER DEFAULT 0,
		status TEXT DEFAULT 'ready',
		attempts INTEGER DEFAULT 0,
		max_attempts INTEGER DEFAULT 3,
		last_error TEXT,
		claimed_by TEXT,
		claimed_at INTEGER,
		operator TEXT DEFAULT '',
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	);
	CREATE TABLE IF NOT EXISTS task_dependencies (
		task_id TEXT NOT NULL,
		blocked_by TEXT NOT NULL,
		PRIMARY KEY (task_id, blocked_by)
	);`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return db
}

// newTestServer creates a dashboard server backed by an in-memory DB.
func newTestServer(t *testing.T) (*Server, *sql.DB) {
	t.Helper()
	db := newTestDB(t)
	s, err := New(Config{DB: db, Addr: ":0"})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	return s, db
}

func seedTasks(t *testing.T, db *sql.DB) {
	t.Helper()
	now := time.Now().Unix()
	db.Exec(`INSERT INTO epics (id, title, status, created_at) VALUES ('e1','Epic 1','open',?)`, now)
	db.Exec(`INSERT INTO tasks (id,title,epic_id,status,priority,created_at,updated_at) VALUES ('t1','Task 1','e1','ready',2,?,?)`, now, now)
	db.Exec(`INSERT INTO tasks (id,title,epic_id,status,priority,created_at,updated_at) VALUES ('t2','Task 2','e1','completed',1,?,?)`, now, now)
	db.Exec(`INSERT INTO tasks (id,title,epic_id,status,priority,claimed_by,claimed_at,created_at,updated_at) VALUES ('t3','Task 3','e1','in_progress',3,'worker-1',?,?,?)`, now, now, now)
	db.Exec(`INSERT INTO task_dependencies (task_id,blocked_by) VALUES ('t1','t2')`)
}

// ============================================================================
// Server Construction
// ============================================================================

func TestNew_WithDB(t *testing.T) {
	s, _ := newTestServer(t)
	if s == nil {
		t.Fatal("expected non-nil server")
	}
	if s.hub == nil {
		t.Fatal("expected non-nil hub")
	}
}

func TestNew_WithDatabaseURL(t *testing.T) {
	// Use the "sqlite" driver (glebarez/go-sqlite) which is already imported
	s, err := New(Config{DatabaseURL: ":memory:", Addr: ":0"})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if s == nil {
		t.Fatal("expected non-nil server")
	}
}

func TestShutdown_NilServer(t *testing.T) {
	s, _ := newTestServer(t)
	// s.server is nil (Start not called) — should return nil
	if err := s.Shutdown(nil); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

// ============================================================================
// Hub (WebSocket broadcast infrastructure)
// ============================================================================

func TestHub_RegisterUnregister(t *testing.T) {
	h := newHub()
	go h.run()

	client := &Client{hub: h, send: make(chan []byte, 8)}
	h.register <- client

	// Give hub time to process
	time.Sleep(10 * time.Millisecond)

	h.mu.RLock()
	if !h.clients[client] {
		t.Error("client not registered")
	}
	h.mu.RUnlock()

	h.unregister <- client
	time.Sleep(10 * time.Millisecond)

	h.mu.RLock()
	if h.clients[client] {
		t.Error("client still registered after unregister")
	}
	h.mu.RUnlock()
}

func TestHub_Broadcast(t *testing.T) {
	h := newHub()
	go h.run()

	client := &Client{hub: h, send: make(chan []byte, 8)}
	h.register <- client
	time.Sleep(10 * time.Millisecond)

	h.broadcast <- Event{Type: "test", Data: "hello"}
	time.Sleep(10 * time.Millisecond)

	select {
	case msg := <-client.send:
		var evt Event
		if err := json.Unmarshal(msg, &evt); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if evt.Type != "test" {
			t.Errorf("expected type 'test', got %q", evt.Type)
		}
	default:
		t.Error("expected message in client send channel")
	}
}

func TestHub_BroadcastDropsSlow(t *testing.T) {
	h := newHub()
	go h.run()

	// Client with zero-buffer channel — will be dropped
	slow := &Client{hub: h, send: make(chan []byte)}
	h.register <- slow
	time.Sleep(10 * time.Millisecond)

	h.broadcast <- Event{Type: "drop_test", Data: nil}
	time.Sleep(20 * time.Millisecond)

	h.mu.RLock()
	_, exists := h.clients[slow]
	h.mu.RUnlock()

	if exists {
		t.Error("slow client should have been removed")
	}
}

// ============================================================================
// Event Serialization
// ============================================================================

func TestEvent_JSON(t *testing.T) {
	evt := Event{Type: EventTaskCompleted, Data: TaskEvent{TaskID: "t1", Title: "Fix", Status: "completed"}}
	data, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty JSON")
	}

	var decoded Event
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Type != EventTaskCompleted {
		t.Errorf("expected %q, got %q", EventTaskCompleted, decoded.Type)
	}
}

func TestTaskEvent_JSON(t *testing.T) {
	te := TaskEvent{TaskID: "t1", Title: "Bug fix", Status: "failed", Error: "timeout", Worker: "w1", EpicID: "e1"}
	data, err := json.Marshal(te)
	if err != nil {
		t.Fatal(err)
	}
	var decoded TaskEvent
	json.Unmarshal(data, &decoded)
	if decoded.TaskID != "t1" || decoded.Error != "timeout" || decoded.Worker != "w1" {
		t.Error("field mismatch after roundtrip")
	}
}

// ============================================================================
// Hooks (global dashboard broadcasting)
// ============================================================================

func TestSetGetGlobal(t *testing.T) {
	defer SetGlobal(nil)

	if g := GetGlobal(); g != nil {
		t.Error("expected nil initial global")
	}

	s, _ := newTestServer(t)
	SetGlobal(s)
	if g := GetGlobal(); g != s {
		t.Error("expected set server to be returned")
	}
}

func TestBroadcastHooks_NilDashboard(t *testing.T) {
	SetGlobal(nil)
	// None should panic
	BroadcastTaskClaimed("t1", "test", "w1")
	BroadcastTaskStarted("t1", "test", "w1")
	BroadcastTaskCompleted("t1", "test")
	BroadcastTaskFailed("t1", "test", "err")
	BroadcastTaskBlocked("t1", "test")
	BroadcastStatsUpdate()
}

func TestBroadcastHooks_WithDashboard(t *testing.T) {
	s, _ := newTestServer(t)
	go s.hub.run()
	defer SetGlobal(nil)
	SetGlobal(s)

	client := &Client{hub: s.hub, send: make(chan []byte, 16)}
	s.hub.register <- client
	time.Sleep(10 * time.Millisecond)

	BroadcastTaskClaimed("t1", "Test", "w1")
	time.Sleep(10 * time.Millisecond)

	select {
	case msg := <-client.send:
		var evt Event
		json.Unmarshal(msg, &evt)
		if evt.Type != EventTaskClaimed {
			t.Errorf("expected %q, got %q", EventTaskClaimed, evt.Type)
		}
	default:
		t.Error("expected broadcast message")
	}
}

// ============================================================================
// Queries — getStatus
// ============================================================================

func TestGetStatus_Empty(t *testing.T) {
	s, _ := newTestServer(t)
	stats, err := s.getStatus()
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if stats.Total != 0 {
		t.Errorf("expected 0 total, got %d", stats.Total)
	}
}

func TestGetStatus_WithTasks(t *testing.T) {
	s, db := newTestServer(t)
	seedTasks(t, db)

	stats, err := s.getStatus()
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if stats.Total != 3 {
		t.Errorf("expected 3 total, got %d", stats.Total)
	}
	if stats.Ready != 1 {
		t.Errorf("expected 1 ready, got %d", stats.Ready)
	}
	if stats.Completed != 1 {
		t.Errorf("expected 1 completed, got %d", stats.Completed)
	}
	if stats.InProgress != 1 {
		t.Errorf("expected 1 in_progress, got %d", stats.InProgress)
	}
	if stats.Progress != 33 {
		t.Errorf("expected ~33%% progress, got %d%%", stats.Progress)
	}
}

// ============================================================================
// Queries — getEpics
// ============================================================================

func TestGetEpics_Empty(t *testing.T) {
	s, _ := newTestServer(t)
	epics, err := s.getEpics()
	if err != nil {
		t.Fatal(err)
	}
	if len(epics) != 0 {
		t.Errorf("expected 0 epics, got %d", len(epics))
	}
}

func TestGetEpics_WithData(t *testing.T) {
	s, db := newTestServer(t)
	seedTasks(t, db)

	epics, err := s.getEpics()
	if err != nil {
		t.Fatal(err)
	}
	if len(epics) != 1 {
		t.Fatalf("expected 1 epic, got %d", len(epics))
	}
	if epics[0].TaskCount != 3 {
		t.Errorf("expected 3 tasks, got %d", epics[0].TaskCount)
	}
	if epics[0].Completed != 1 {
		t.Errorf("expected 1 completed, got %d", epics[0].Completed)
	}
}

// ============================================================================
// Queries — getTasks
// ============================================================================

func TestGetTasks_All(t *testing.T) {
	s, db := newTestServer(t)
	seedTasks(t, db)

	tasks, err := s.getTasks("", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 3 {
		t.Errorf("expected 3 tasks, got %d", len(tasks))
	}
}

func TestGetTasks_FilterByStatus(t *testing.T) {
	s, db := newTestServer(t)
	seedTasks(t, db)

	tasks, err := s.getTasks("", "ready")
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].ID != "t1" {
		t.Errorf("expected 1 ready task (t1), got %d", len(tasks))
	}
}

func TestGetTasks_FilterByEpic(t *testing.T) {
	s, db := newTestServer(t)
	seedTasks(t, db)

	tasks, err := s.getTasks("e1", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 3 {
		t.Errorf("expected 3 tasks in epic e1, got %d", len(tasks))
	}
}

func TestGetTasks_FilterByEpicAndStatus(t *testing.T) {
	s, db := newTestServer(t)
	seedTasks(t, db)

	tasks, err := s.getTasks("e1", "completed")
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].ID != "t2" {
		t.Errorf("expected 1 completed task (t2), got %v", tasks)
	}
}

// ============================================================================
// Queries — getTask
// ============================================================================

func TestGetTask_Found(t *testing.T) {
	s, db := newTestServer(t)
	seedTasks(t, db)

	task, err := s.getTask("t1")
	if err != nil {
		t.Fatal(err)
	}
	if task.Title != "Task 1" {
		t.Errorf("expected 'Task 1', got %q", task.Title)
	}
}

func TestGetTask_NotFound(t *testing.T) {
	s, _ := newTestServer(t)
	_, err := s.getTask("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent task")
	}
}

// ============================================================================
// Queries — getWorkers
// ============================================================================

func TestGetWorkers(t *testing.T) {
	s, db := newTestServer(t)
	seedTasks(t, db)

	workers, err := s.getWorkers()
	if err != nil {
		t.Fatal(err)
	}
	if len(workers) != 1 {
		t.Fatalf("expected 1 active worker, got %d", len(workers))
	}
	if workers[0].WorkerID != "worker-1" {
		t.Errorf("expected 'worker-1', got %q", workers[0].WorkerID)
	}
}

// ============================================================================
// Queries — getGraph
// ============================================================================

func TestGetGraph(t *testing.T) {
	s, db := newTestServer(t)
	seedTasks(t, db)

	graph, err := s.getGraph()
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Nodes) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(graph.Nodes))
	}
	if len(graph.Edges) != 1 {
		t.Errorf("expected 1 edge, got %d", len(graph.Edges))
	}
	// Edge: t2 -> t1 (t1 is blocked by t2)
	if graph.Edges[0].From != "t2" || graph.Edges[0].To != "t1" {
		t.Errorf("expected edge t2->t1, got %s->%s", graph.Edges[0].From, graph.Edges[0].To)
	}
}

// ============================================================================
// HTTP Handlers
// ============================================================================

func TestHandleStatus(t *testing.T) {
	s, db := newTestServer(t)
	seedTasks(t, db)

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	w := httptest.NewRecorder()
	s.handleStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var stats Stats
	json.NewDecoder(w.Body).Decode(&stats)
	if stats.Total != 3 {
		t.Errorf("expected 3 total, got %d", stats.Total)
	}
}

func TestHandleTasks_InvalidStatus(t *testing.T) {
	s, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/tasks?status=invalid", nil)
	w := httptest.NewRecorder()
	s.handleTasks(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTasks_ValidStatus(t *testing.T) {
	s, db := newTestServer(t)
	seedTasks(t, db)

	req := httptest.NewRequest(http.MethodGet, "/api/tasks?status=ready", nil)
	w := httptest.NewRecorder()
	s.handleTasks(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleTask_Found(t *testing.T) {
	s, db := newTestServer(t)
	seedTasks(t, db)

	req := httptest.NewRequest(http.MethodGet, "/api/tasks/t1", nil)
	w := httptest.NewRecorder()
	s.handleTask(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleTask_NotFound(t *testing.T) {
	s, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/tasks/nonexistent", nil)
	w := httptest.NewRecorder()
	s.handleTask(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleTask_BadPath(t *testing.T) {
	s, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/wrong/path", nil)
	w := httptest.NewRecorder()
	s.handleTask(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleWorkers(t *testing.T) {
	s, db := newTestServer(t)
	seedTasks(t, db)

	req := httptest.NewRequest(http.MethodGet, "/api/workers", nil)
	w := httptest.NewRecorder()
	s.handleWorkers(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleGraph(t *testing.T) {
	s, db := newTestServer(t)
	seedTasks(t, db)

	req := httptest.NewRequest(http.MethodGet, "/api/graph", nil)
	w := httptest.NewRecorder()
	s.handleGraph(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleEpics(t *testing.T) {
	s, db := newTestServer(t)
	seedTasks(t, db)

	req := httptest.NewRequest(http.MethodGet, "/api/epics", nil)
	w := httptest.NewRecorder()
	s.handleEpics(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// ============================================================================
// jsonResponse
// ============================================================================

func TestJsonResponse(t *testing.T) {
	w := httptest.NewRecorder()
	jsonResponse(w, map[string]string{"key": "value"})

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}
	if w.Body.Len() == 0 {
		t.Error("expected non-empty body")
	}
}
