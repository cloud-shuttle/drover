package beads

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cloud-shuttle/drover/pkg/types"
)

func TestDefaultSyncConfig(t *testing.T) {
	cfg := DefaultSyncConfig("/some/project")
	expected := filepath.Join("/some/project", ".beads")
	if cfg.BeadsDir != expected {
		t.Errorf("expected BeadsDir %q, got %q", expected, cfg.BeadsDir)
	}
	if cfg.SyncInterval != 5*time.Second {
		t.Errorf("expected SyncInterval 5s, got %v", cfg.SyncInterval)
	}
	if cfg.AutoSync {
		t.Error("expected AutoSync to be false by default")
	}
}

func TestBeadsStatusToDrover(t *testing.T) {
	tests := []struct {
		input    string
		expected types.TaskStatus
	}{
		{"open", types.TaskStatusReady},
		{"active", types.TaskStatusInProgress},
		{"closed", types.TaskStatusCompleted},
		{"unknown", types.TaskStatusReady},
		{"", types.TaskStatusReady},
	}
	for _, tt := range tests {
		got := beadsStatusToDrover(tt.input)
		if got != tt.expected {
			t.Errorf("beadsStatusToDrover(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestDroverStatusToBeads(t *testing.T) {
	tests := []struct {
		input    types.TaskStatus
		expected string
	}{
		{types.TaskStatusReady, "open"},
		{types.TaskStatusClaimed, "open"},
		{types.TaskStatusBlocked, "open"},
		{types.TaskStatusInProgress, "active"},
		{types.TaskStatusCompleted, "closed"},
		{types.TaskStatusFailed, "closed"},
		{types.TaskStatusCancelled, "closed"},
		{"unknown", "open"},
	}
	for _, tt := range tests {
		got := droverStatusToBeads(tt.input)
		if got != tt.expected {
			t.Errorf("droverStatusToBeads(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestExportToBeads_CreatesFile(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := SyncConfig{BeadsDir: tmpDir}

	epics := []types.Epic{
		{ID: "epic-1", Title: "Epic One", Description: "First epic", Status: types.EpicStatusOpen, CreatedAt: time.Now().Unix()},
	}
	tasks := []types.Task{
		{ID: "task-1", Title: "Task One", Description: "First task", EpicID: "epic-1", Status: types.TaskStatusReady, Priority: 10, CreatedAt: time.Now().Unix()},
		{ID: "task-2", Title: "Task Two", Description: "Second task", EpicID: "epic-1", Status: types.TaskStatusCompleted, Priority: 5, CreatedAt: time.Now().Unix()},
	}
	deps := []types.TaskDependency{
		{TaskID: "task-2", BlockedBy: "task-1"},
	}

	err := ExportToBeads(epics, tasks, deps, cfg)
	if err != nil {
		t.Fatalf("ExportToBeads failed: %v", err)
	}

	jsonlPath := filepath.Join(tmpDir, "beads.jsonl")
	if _, err := os.Stat(jsonlPath); os.IsNotExist(err) {
		t.Fatal("beads.jsonl was not created")
	}
}

func TestExportAndImportRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := SyncConfig{BeadsDir: tmpDir}

	now := time.Now().Unix()
	epics := []types.Epic{
		{ID: "epic-1", Title: "Epic One", Description: "First epic", Status: types.EpicStatusOpen, CreatedAt: now},
	}
	tasks := []types.Task{
		{ID: "task-1", Title: "Task One", Description: "First task", EpicID: "epic-1", Status: types.TaskStatusReady, Priority: 10, CreatedAt: now},
		{ID: "task-2", Title: "Task Two", Description: "Second task", EpicID: "epic-1", Status: types.TaskStatusInProgress, Priority: 5, CreatedAt: now},
	}
	deps := []types.TaskDependency{
		{TaskID: "task-2", BlockedBy: "task-1"},
	}

	// Export
	if err := ExportToBeads(epics, tasks, deps, cfg); err != nil {
		t.Fatalf("ExportToBeads failed: %v", err)
	}

	// Import back
	importedEpics, importedTasks, importedDeps, err := ImportFromBeads(cfg)
	if err != nil {
		t.Fatalf("ImportFromBeads failed: %v", err)
	}

	// Verify epics
	if len(importedEpics) != 1 {
		t.Fatalf("expected 1 epic, got %d", len(importedEpics))
	}
	if importedEpics[0].Title != "Epic One" {
		t.Errorf("expected epic title 'Epic One', got %q", importedEpics[0].Title)
	}

	// Verify tasks
	if len(importedTasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(importedTasks))
	}

	// Verify dependencies
	if len(importedDeps) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(importedDeps))
	}
	if importedDeps[0].TaskID != "task-2" || importedDeps[0].BlockedBy != "task-1" {
		t.Errorf("dependency mismatch: got %+v", importedDeps[0])
	}
}

func TestImportFromBeads_NotFound(t *testing.T) {
	cfg := SyncConfig{BeadsDir: "/nonexistent/path"}
	_, _, _, err := ImportFromBeads(cfg)
	if err == nil {
		t.Error("expected error for non-existent beads dir")
	}
}

func TestImportFromBeads_MalformedLines(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := SyncConfig{BeadsDir: tmpDir}

	// Write a file with malformed and valid lines
	jsonlPath := filepath.Join(tmpDir, "beads.jsonl")
	epicData, _ := json.Marshal(BeadEpic{Title: "Valid Epic", Status: "open"})
	validLine := `{"type":"epic","id":"epic-1","timestamp":"2024-01-01T00:00:00Z","data":` + string(epicData) + `}`
	content := "this is not json\n" + validLine + "\nalso bad\n"
	if err := os.WriteFile(jsonlPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	epics, _, _, err := ImportFromBeads(cfg)
	if err != nil {
		t.Fatalf("ImportFromBeads failed on mixed content: %v", err)
	}
	if len(epics) != 1 {
		t.Errorf("expected 1 valid epic parsed, got %d", len(epics))
	}
}

func TestImportFromBeads_HierarchicalTasks(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := SyncConfig{BeadsDir: tmpDir}

	// Create beads.jsonl with hierarchical task IDs
	jsonlPath := filepath.Join(tmpDir, "beads.jsonl")
	parentData, _ := json.Marshal(BeadTask{Title: "Parent Task", Status: "open"})
	childData, _ := json.Marshal(BeadTask{Title: "Child Task", Status: "open"})
	deepChildData, _ := json.Marshal(BeadTask{Title: "Too Deep", Status: "open"})

	lines := ""
	lines += `{"type":"bead","id":"task-1","timestamp":"2024-01-01T00:00:00Z","data":` + string(parentData) + "}\n"
	lines += `{"type":"bead","id":"task-1.1","timestamp":"2024-01-01T00:00:00Z","data":` + string(childData) + "}\n"
	lines += `{"type":"bead","id":"task-1.1.1","timestamp":"2024-01-01T00:00:00Z","data":` + string(deepChildData) + "}\n"

	if err := os.WriteFile(jsonlPath, []byte(lines), 0644); err != nil {
		t.Fatal(err)
	}

	_, tasks, _, err := ImportFromBeads(cfg)
	if err != nil {
		t.Fatalf("ImportFromBeads failed: %v", err)
	}

	// Should have 2 tasks (parent + child), deep child rejected
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks (depth limit rejects deep child), got %d", len(tasks))
	}

	// Verify child's parentID is set
	for _, task := range tasks {
		if task.ID == "task-1.1" {
			if task.ParentID != "task-1" {
				t.Errorf("expected child parentID 'task-1', got %q", task.ParentID)
			}
			if task.SequenceNumber != 1 {
				t.Errorf("expected child sequence 1, got %d", task.SequenceNumber)
			}
		}
	}
}

func TestImportFromBeads_LinksOnlyBlockedBy(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := SyncConfig{BeadsDir: tmpDir}

	taskData, _ := json.Marshal(BeadTask{Title: "T", Status: "open"})
	blockedLink, _ := json.Marshal(BeadLink{From: "task-2", To: "task-1", LinkType: "blocked_by"})
	relatesLink, _ := json.Marshal(BeadLink{From: "task-2", To: "task-1", LinkType: "relates_to"})

	lines := ""
	lines += `{"type":"bead","id":"task-1","timestamp":"2024-01-01T00:00:00Z","data":` + string(taskData) + "}\n"
	lines += `{"type":"bead","id":"task-2","timestamp":"2024-01-01T00:00:00Z","data":` + string(taskData) + "}\n"
	lines += `{"type":"link","id":"link-1","timestamp":"2024-01-01T00:00:00Z","data":` + string(blockedLink) + "}\n"
	lines += `{"type":"link","id":"link-2","timestamp":"2024-01-01T00:00:00Z","data":` + string(relatesLink) + "}\n"

	jsonlPath := filepath.Join(tmpDir, "beads.jsonl")
	if err := os.WriteFile(jsonlPath, []byte(lines), 0644); err != nil {
		t.Fatal(err)
	}

	_, _, deps, err := ImportFromBeads(cfg)
	if err != nil {
		t.Fatalf("ImportFromBeads failed: %v", err)
	}

	// Only blocked_by links should be imported as dependencies
	if len(deps) != 1 {
		t.Fatalf("expected 1 dependency (only blocked_by), got %d", len(deps))
	}
}
