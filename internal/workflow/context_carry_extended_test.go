package workflow

import (
	"strings"
	"testing"

	"github.com/cloud-shuttle/drover/internal/config"
	"github.com/cloud-shuttle/drover/internal/db"
	"github.com/cloud-shuttle/drover/pkg/types"
)

// ============================================================================
// InjectRecentTaskContext – additional scenarios
// ============================================================================

func TestInjectRecentTaskContext_NilStore(t *testing.T) {
	cfg := &config.Config{TaskContextCount: 5}
	task := &types.Task{ID: "t1", Title: "T", Description: "original"}

	InjectRecentTaskContext(nil, cfg, task)

	if task.Description != "original" {
		t.Error("expected description unchanged when store is nil")
	}
}

func TestInjectRecentTaskContext_NilConfig(t *testing.T) {
	tmp := t.TempDir()
	store, _ := db.Open(tmp + "/test.db")
	defer store.Close()
	store.InitSchema()

	task := &types.Task{ID: "t1", Title: "T", Description: "original"}

	InjectRecentTaskContext(store, nil, task)

	if task.Description != "original" {
		t.Error("expected description unchanged when config is nil")
	}
}

func TestInjectRecentTaskContext_NilTask(t *testing.T) {
	tmp := t.TempDir()
	store, _ := db.Open(tmp + "/test.db")
	defer store.Close()
	store.InitSchema()

	cfg := &config.Config{TaskContextCount: 5}

	// Should not panic
	InjectRecentTaskContext(store, cfg, nil)
}

func TestInjectRecentTaskContext_ZeroCount(t *testing.T) {
	tmp := t.TempDir()
	store, _ := db.Open(tmp + "/test.db")
	defer store.Close()
	store.InitSchema()

	cfg := &config.Config{TaskContextCount: 0}
	task := &types.Task{ID: "t1", Title: "T", Description: "original"}

	InjectRecentTaskContext(store, cfg, task)

	if task.Description != "original" {
		t.Error("expected description unchanged when count is 0")
	}
}

func TestInjectRecentTaskContext_NegativeCount(t *testing.T) {
	tmp := t.TempDir()
	store, _ := db.Open(tmp + "/test.db")
	defer store.Close()
	store.InitSchema()

	cfg := &config.Config{TaskContextCount: -1}
	task := &types.Task{ID: "t1", Title: "T", Description: "original"}

	InjectRecentTaskContext(store, cfg, task)

	if task.Description != "original" {
		t.Error("expected description unchanged when count is negative")
	}
}

func TestInjectRecentTaskContext_NoCompletedTasks(t *testing.T) {
	tmp := t.TempDir()
	store, _ := db.Open(tmp + "/test.db")
	defer store.Close()
	store.InitSchema()

	// Only create in-progress tasks
	a, _ := store.CreateTask("A", "", "", 0, nil)
	store.UpdateTaskStatus(a.ID, types.TaskStatusInProgress, "")

	cfg := &config.Config{TaskContextCount: 5}
	task := &types.Task{ID: "current", Title: "T", Description: "original"}

	InjectRecentTaskContext(store, cfg, task)

	if task.Description != "original" {
		t.Error("expected description unchanged when no completed tasks")
	}
}

func TestInjectRecentTaskContext_IncludesLastError(t *testing.T) {
	tmp := t.TempDir()
	store, _ := db.Open(tmp + "/test.db")
	defer store.Close()
	store.InitSchema()

	a, _ := store.CreateTask("A", "", "", 0, nil)
	store.UpdateTaskStatus(a.ID, types.TaskStatusCompleted, "had some issue")

	cfg := &config.Config{TaskContextCount: 5}
	task := &types.Task{ID: "current", Title: "T", Description: "original"}

	InjectRecentTaskContext(store, cfg, task)

	if !strings.Contains(task.Description, "had some issue") {
		t.Error("expected last error included in context block")
	}
}

func TestInjectRecentTaskContext_ExcludesCurrentTask(t *testing.T) {
	tmp := t.TempDir()
	store, _ := db.Open(tmp + "/test.db")
	defer store.Close()
	store.InitSchema()

	current, _ := store.CreateTask("Current", "", "", 0, nil)
	store.UpdateTaskStatus(current.ID, types.TaskStatusCompleted, "")

	cfg := &config.Config{TaskContextCount: 5}
	task := &types.Task{ID: current.ID, Title: "Current", Description: "original"}

	InjectRecentTaskContext(store, cfg, task)

	// Should not inject context about the current task itself
	if task.Description != "original" {
		// If the only completed task is excluded, description should be unchanged
		if strings.HasPrefix(task.Description, "## Recent Task Context") {
			if strings.Contains(task.Description, current.ID) {
				t.Error("should not include current task in context block")
			}
		}
	}
}

func TestInjectRecentTaskContext_EpicFilter(t *testing.T) {
	tmp := t.TempDir()
	store, _ := db.Open(tmp + "/test.db")
	defer store.Close()
	store.InitSchema()

	e1, _ := store.CreateEpic("E1", "")
	a, _ := store.CreateTask("A-E1", "", e1.ID, 0, nil)
	b, _ := store.CreateTask("B-NoEpic", "", "", 0, nil)
	store.UpdateTaskStatus(a.ID, types.TaskStatusCompleted, "")
	store.UpdateTaskStatus(b.ID, types.TaskStatusCompleted, "")

	cfg := &config.Config{TaskContextCount: 5}
	task := &types.Task{ID: "current", Title: "T", Description: "original", EpicID: e1.ID}

	InjectRecentTaskContext(store, cfg, task)

	if !strings.HasPrefix(task.Description, "## Recent Task Context") {
		t.Fatal("expected context block prepended")
	}
	// When epic is set, it should prefer same-epic tasks
	if !strings.Contains(task.Description, "A-E1") {
		t.Error("expected task from same epic in context")
	}
}
