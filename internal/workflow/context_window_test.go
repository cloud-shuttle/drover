package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloud-shuttle/drover/internal/config"
	"github.com/cloud-shuttle/drover/pkg/types"
)

func TestPrepareTaskContextForAgent_NoChangeWhenUnderThreshold(t *testing.T) {
	tmp := t.TempDir()
	cfg := &config.Config{MaxDescriptionSize: 100}
	task := &types.Task{ID: "task-1", Title: "T", Description: "short", EpicID: "epic-1"}

	if err := PrepareTaskContextForAgent(tmp, cfg, task); err != nil {
		t.Fatalf("PrepareTaskContextForAgent: %v", err)
	}
	if task.Description != "short" {
		t.Fatalf("expected description unchanged")
	}
}

func TestPrepareTaskContextForAgent_WritesPayloadAndReplacesDescription(t *testing.T) {
	tmp := t.TempDir()
	cfg := &config.Config{MaxDescriptionSize: 10}
	task := &types.Task{
		ID:          "task-123",
		Title:       "Big Task",
		Description: strings.Repeat("x", 50),
		EpicID:      "epic-xyz",
	}

	if err := PrepareTaskContextForAgent(tmp, cfg, task); err != nil {
		t.Fatalf("PrepareTaskContextForAgent: %v", err)
	}

	// Description should now be the notice with fetch instructions
	if !strings.Contains(task.Description, "Large Content Notice") {
		t.Fatalf("expected Large Content Notice in description")
	}
	if !strings.Contains(task.Description, `cat ".drover/task_payload/task-123.md"`) {
		t.Fatalf("expected cat fetch command in description, got: %s", task.Description)
	}

	// Payload file should exist
	payloadPath := filepath.Join(tmp, ".drover", "task_payload", "task-123.md")
	b, err := os.ReadFile(payloadPath)
	if err != nil {
		t.Fatalf("expected payload file written: %v", err)
	}
	if !strings.Contains(string(b), "## Full Description") {
		t.Fatalf("expected full description section in payload")
	}
	if !strings.Contains(string(b), strings.Repeat("x", 50)) {
		t.Fatalf("expected original description in payload")
	}
}

