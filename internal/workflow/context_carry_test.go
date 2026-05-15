package workflow

import (
	"strings"
	"testing"

	"github.com/cloud-shuttle/drover/internal/config"
	"github.com/cloud-shuttle/drover/internal/db"
	"github.com/cloud-shuttle/drover/pkg/types"
)

func TestInjectRecentTaskContext_PrependsBlock(t *testing.T) {
	// Create a temporary DB
	tmp := t.TempDir()
	store, err := db.Open(tmp + "/test.db")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	if err := store.InitSchema(); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}

	// Seed completed tasks
	a, _ := store.CreateTask("A", "", "", 0, nil)
	b, _ := store.CreateTask("B", "", "", 0, nil)
	_ = store.UpdateTaskStatus(a.ID, types.TaskStatusCompleted, "did X")
	_ = store.UpdateTaskStatus(b.ID, types.TaskStatusCompleted, "")

	cfg := &config.Config{TaskContextCount: 2}
	task := &types.Task{ID: "task-current", Title: "Now", Description: "Do the thing"}

	InjectRecentTaskContext(store, cfg, task)

	if !strings.HasPrefix(task.Description, "## Recent Task Context") {
		t.Fatalf("expected context block prepended, got: %s", task.Description)
	}
	if !strings.Contains(task.Description, "Do the thing") {
		t.Fatalf("expected original description preserved")
	}
}

