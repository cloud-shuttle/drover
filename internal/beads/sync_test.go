package beads

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cloud-shuttle/drover/pkg/types"
)

// TestImportFromBeads tests importing tasks from beads.jsonl
func TestImportFromBeads(t *testing.T) {
	// Create temporary beads directory
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("Failed to create beads dir: %v", err)
	}

	// Create test beads.jsonl file
	jsonlPath := filepath.Join(beadsDir, "beads.jsonl")
	content := `{"type":"epic","id":"epic-1","timestamp":"2024-01-01T00:00:00Z","data":{"title":"Test Epic","description":"Test Description","status":"open"}}
{"type":"bead","id":"task-1","timestamp":"2024-01-01T01:00:00Z","data":{"title":"Task 1","description":"First task","status":"open","epic_id":"epic-1"}}
{"type":"bead","id":"task-2","timestamp":"2024-01-01T02:00:00Z","data":{"title":"Task 2","status":"open"}}
{"type":"link","id":"link-task-2-task-1","timestamp":"2024-01-01T03:00:00Z","data":{"from":"task-2","to":"task-1","link_type":"blocked_by"}}
{"type":"bead","id":"task-1.1","timestamp":"2024-01-01T04:00:00Z","data":{"title":"Subtask 1.1","status":"open"}}
{"type":"bead","id":"task-1.1.1","timestamp":"2024-01-01T05:00:00Z","data":{"title":"Deep Subtask","status":"open"}}
`

	if err := os.WriteFile(jsonlPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write beads.jsonl: %v", err)
	}

	config := SyncConfig{BeadsDir: beadsDir}
	epics, tasks, deps, err := ImportFromBeads(config)
	if err != nil {
		t.Fatalf("ImportFromBeads failed: %v", err)
	}

	// Verify epics
	if len(epics) != 1 {
		t.Errorf("Expected 1 epic, got %d", len(epics))
	}
	if epics[0].ID != "epic-1" {
		t.Errorf("Expected epic ID 'epic-1', got %s", epics[0].ID)
	}

	// Verify tasks (should skip deep subtask)
	if len(tasks) != 3 {
		t.Errorf("Expected 3 tasks (deep subtask filtered), got %d", len(tasks))
	}

	// Verify hierarchical task
	var subtask *types.Task
	for _, task := range tasks {
		if task.ID == "task-1.1" {
			subtask = &task
			break
		}
	}
	if subtask == nil {
		t.Fatal("Subtask task-1.1 not found")
	}
	if subtask.ParentID != "task-1" {
		t.Errorf("Expected parent_id 'task-1', got %s", subtask.ParentID)
	}
	if subtask.SequenceNumber != 1 {
		t.Errorf("Expected sequence_number 1, got %d", subtask.SequenceNumber)
	}

	// Verify dependencies
	if len(deps) != 1 {
		t.Errorf("Expected 1 dependency, got %d", len(deps))
	}
	if deps[0].TaskID != "task-2" {
		t.Errorf("Expected task_id 'task-2', got %s", deps[0].TaskID)
	}
	if deps[0].BlockedBy != "task-1" {
		t.Errorf("Expected blocked_by 'task-1', got %s", deps[0].BlockedBy)
	}
}

// TestExportToBeads tests exporting tasks to beads.jsonl
func TestExportToBeads(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")

	epics := []types.Epic{
		{
			ID:          "epic-1",
			Title:       "Test Epic",
			Description: "Test Description",
			Status:      types.EpicStatusOpen,
			CreatedAt:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).Unix(),
		},
	}

	tasks := []types.Task{
		{
			ID:          "task-1",
			Title:       "Task 1",
			Description: "First task",
			EpicID:      "epic-1",
			Status:      types.TaskStatusReady,
			CreatedAt:   time.Date(2024, 1, 1, 1, 0, 0, 0, time.UTC).Unix(),
		},
		{
			ID:             "task-1.1",
			Title:          "Subtask 1.1",
			ParentID:       "task-1",
			SequenceNumber: 1,
			Status:         types.TaskStatusReady,
			CreatedAt:      time.Date(2024, 1, 1, 2, 0, 0, 0, time.UTC).Unix(),
		},
	}

	deps := []types.TaskDependency{
		{
			TaskID:    "task-2",
			BlockedBy: "task-1",
		},
	}

	config := SyncConfig{BeadsDir: beadsDir}
	if err := ExportToBeads(epics, tasks, deps, config); err != nil {
		t.Fatalf("ExportToBeads failed: %v", err)
	}

	// Read back the file
	jsonlPath := filepath.Join(beadsDir, "beads.jsonl")
	content, err := os.ReadFile(jsonlPath)
	if err != nil {
		t.Fatalf("Failed to read beads.jsonl: %v", err)
	}

	contentStr := string(content)
	// Verify some content exists
	if !contains(contentStr, `"type":"epic"`) {
		t.Error("Expected epic record in output")
	}
	if !contains(contentStr, `"type":"bead"`) {
		t.Error("Expected bead record in output")
	}
	if !contains(contentStr, `"type":"link"`) {
		t.Error("Expected link record in output")
	}
}

// TestStatusConversion tests status conversion between Beads and Drover
func TestStatusConversion(t *testing.T) {
	tests := []struct {
		beadsStatus   string
		droverStatus  types.TaskStatus
	}{
		{"open", types.TaskStatusReady},
		{"active", types.TaskStatusInProgress},
		{"closed", types.TaskStatusCompleted},
		{"unknown", types.TaskStatusReady}, // Default
	}

	for _, tt := range tests {
		result := beadsStatusToDrover(tt.beadsStatus)
		if result != tt.droverStatus {
			t.Errorf("beadsStatusToDrover(%q) = %v, want %v", tt.beadsStatus, result, tt.droverStatus)
		}
	}

	// Test Drover to Beads conversion
	tests2 := []struct {
		droverStatus types.TaskStatus
		beadsStatus  string
	}{
		{types.TaskStatusReady, "open"},
		{types.TaskStatusClaimed, "open"},
		{types.TaskStatusBlocked, "open"},
		{types.TaskStatusInProgress, "active"},
		{types.TaskStatusCompleted, "closed"},
		{types.TaskStatusFailed, "closed"},
	}

	for _, tt := range tests2 {
		result := droverStatusToBeads(tt.droverStatus)
		if result != tt.beadsStatus {
			t.Errorf("droverStatusToBeads(%v) = %q, want %q", tt.droverStatus, result, tt.beadsStatus)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
