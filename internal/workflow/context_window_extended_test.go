package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloud-shuttle/drover/internal/config"
	"github.com/cloud-shuttle/drover/pkg/types"
)

// ============================================================================
// PrepareTaskContextForAgent – additional scenarios
// ============================================================================

func TestPrepareTaskContextForAgent_NilConfig(t *testing.T) {
	tmp := t.TempDir()
	task := &types.Task{ID: "t1", Title: "T", Description: "desc"}

	err := PrepareTaskContextForAgent(tmp, nil, task)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if task.Description != "desc" {
		t.Error("expected description unchanged when config is nil")
	}
}

func TestPrepareTaskContextForAgent_NilTask(t *testing.T) {
	tmp := t.TempDir()
	cfg := &config.Config{MaxDescriptionSize: 10}

	err := PrepareTaskContextForAgent(tmp, cfg, nil)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func TestPrepareTaskContextForAgent_DisabledWhenZero(t *testing.T) {
	tmp := t.TempDir()
	cfg := &config.Config{MaxDescriptionSize: 0}
	task := &types.Task{ID: "t1", Title: "T", Description: strings.Repeat("x", 100)}

	err := PrepareTaskContextForAgent(tmp, cfg, task)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if task.Description != strings.Repeat("x", 100) {
		t.Error("expected description unchanged when max is 0 (disabled)")
	}
}

func TestPrepareTaskContextForAgent_DisabledWhenNegative(t *testing.T) {
	tmp := t.TempDir()
	cfg := &config.Config{MaxDescriptionSize: -1}
	task := &types.Task{ID: "t1", Title: "T", Description: strings.Repeat("x", 100)}

	err := PrepareTaskContextForAgent(tmp, cfg, task)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if task.Description != strings.Repeat("x", 100) {
		t.Error("expected description unchanged when max is negative")
	}
}

func TestPrepareTaskContextForAgent_ExactlyAtThreshold(t *testing.T) {
	tmp := t.TempDir()
	cfg := &config.Config{MaxDescriptionSize: 50}
	desc := strings.Repeat("x", 50) // exactly at threshold
	task := &types.Task{ID: "t1", Title: "T", Description: desc}

	err := PrepareTaskContextForAgent(tmp, cfg, task)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if task.Description != desc {
		t.Error("expected description unchanged when exactly at threshold")
	}
}

func TestPrepareTaskContextForAgent_PayloadContainsAllFields(t *testing.T) {
	tmp := t.TempDir()
	cfg := &config.Config{MaxDescriptionSize: 10}
	task := &types.Task{
		ID:          "task-full",
		Title:       "Full Task Title",
		Description: strings.Repeat("detail-", 10),
		EpicID:      "epic-42",
	}

	err := PrepareTaskContextForAgent(tmp, cfg, task)
	if err != nil {
		t.Fatalf("PrepareTaskContextForAgent: %v", err)
	}

	// Read the payload file
	payloadPath := filepath.Join(tmp, ".drover", "task_payload", "task-full.md")
	b, err := os.ReadFile(payloadPath)
	if err != nil {
		t.Fatalf("expected payload file: %v", err)
	}

	content := string(b)
	if !strings.Contains(content, "# Task Payload: task-full") {
		t.Error("expected task ID in payload header")
	}
	if !strings.Contains(content, "Full Task Title") {
		t.Error("expected title in payload")
	}
	if !strings.Contains(content, "epic-42") {
		t.Error("expected epic ID in payload")
	}
	if !strings.Contains(content, "detail-") {
		t.Error("expected original description in payload")
	}
}

func TestPrepareTaskContextForAgent_DescriptionContainsByteCount(t *testing.T) {
	tmp := t.TempDir()
	cfg := &config.Config{MaxDescriptionSize: 10}
	task := &types.Task{
		ID:          "t-bytes",
		Title:       "T",
		Description: strings.Repeat("A", 50),
	}

	PrepareTaskContextForAgent(tmp, cfg, task)

	// Description should mention the byte count and threshold
	if !strings.Contains(task.Description, "50 bytes") {
		t.Errorf("expected '50 bytes' in notice, got: %s", task.Description)
	}
	if !strings.Contains(task.Description, "10 bytes") {
		t.Errorf("expected '10 bytes' threshold in notice, got: %s", task.Description)
	}
}
