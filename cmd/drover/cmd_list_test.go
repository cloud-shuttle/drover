package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloud-shuttle/drover/internal/db"
	"github.com/cloud-shuttle/drover/pkg/types"
)

// seedListProject creates two epics (one empty) and four tasks:
// two in epic A (one completed), none in epic B, one with no epic.
func seedListProject(t *testing.T) (epicA, epicB string, doneID string) {
	t.Helper()
	dir := setupTestProject(t)
	store, err := db.Open(filepath.Join(dir, ".drover", "drover.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	a, err := store.CreateEpic("Auth rework", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := store.CreateEpic("Empty epic", "")
	if err != nil {
		t.Fatal(err)
	}
	t1, err := store.CreateTask("Add login handler", "", a.ID, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateTask("Add logout handler", "", a.ID, 0, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateTask("Unrelated chore", "", "", 0, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateTaskStatus(t1.ID, types.TaskStatusCompleted, ""); err != nil {
		t.Fatal(err)
	}
	return a.ID, b.ID, t1.ID
}

func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	var err error
	switch args[0] {
	case "epic":
		c := epicCmd()
		c.SetOut(&buf)
		c.SetArgs(args[1:])
		err = c.Execute()
	case "list":
		c := listCmd()
		c.SetOut(&buf)
		c.SetArgs(args[1:])
		err = c.Execute()
	default:
		t.Fatalf("unknown command %q", args[0])
	}
	return buf.String(), err
}

func TestEpicListTable(t *testing.T) {
	epicA, epicB, _ := seedListProject(t)
	out, err := run(t, "epic", "list")
	if err != nil {
		t.Fatalf("epic list: %v", err)
	}
	for _, want := range []string{"ID", "TASKS", epicA, epicB, "Auth rework", "Empty epic"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestEpicListJSON(t *testing.T) {
	epicA, epicB, _ := seedListProject(t)
	out, err := run(t, "epic", "list", "--json")
	if err != nil {
		t.Fatalf("epic list --json: %v", err)
	}
	var got []epicSummary
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("bad json: %v\n%s", err, out)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 epics, got %d", len(got))
	}
	byID := map[string]epicSummary{}
	for _, e := range got {
		byID[e.ID] = e
	}
	if a := byID[epicA]; a.TaskCount != 2 || a.ByStatus["completed"] != 1 || a.Title != "Auth rework" {
		t.Errorf("epic A summary wrong: %+v", a)
	}
	if b := byID[epicB]; b.TaskCount != 0 || b.ByStatus == nil {
		t.Errorf("epic B summary wrong: %+v", b)
	}
}

func TestEpicListEmpty(t *testing.T) {
	setupTestProject(t)
	out, err := run(t, "epic", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "No epics found") {
		t.Errorf("unexpected output: %s", out)
	}
	out, err = run(t, "epic", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "[]" {
		t.Errorf("want [] for empty json, got %q", out)
	}
}

func TestListAllTasks(t *testing.T) {
	seedListProject(t)
	out, err := run(t, "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var tasks []types.Task
	if err := json.Unmarshal([]byte(out), &tasks); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if len(tasks) != 3 {
		t.Fatalf("want 3 tasks, got %d", len(tasks))
	}
}

func TestListByEpicAndStatus(t *testing.T) {
	epicA, epicB, doneID := seedListProject(t)

	out, err := run(t, "list", "--epic", epicA, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var tasks []types.Task
	_ = json.Unmarshal([]byte(out), &tasks)
	if len(tasks) != 2 {
		t.Fatalf("epic filter: want 2, got %d", len(tasks))
	}

	out, err = run(t, "list", "--epic", epicA, "--status", "completed", "--json")
	if err != nil {
		t.Fatal(err)
	}
	tasks = nil
	_ = json.Unmarshal([]byte(out), &tasks)
	if len(tasks) != 1 || tasks[0].ID != doneID {
		t.Fatalf("epic+status filter: got %+v", tasks)
	}

	out, err = run(t, "list", "--epic", epicB)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "No tasks found") {
		t.Errorf("empty epic: %s", out)
	}

	out, err = run(t, "list", "--status", "ready")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Unrelated chore") || strings.Contains(out, "Add login handler") {
		t.Errorf("status table wrong:\n%s", out)
	}
}

func TestListRejectsBadInput(t *testing.T) {
	seedListProject(t)
	if _, err := run(t, "list", "--status", "bogus"); err == nil || !strings.Contains(err.Error(), "unknown status") {
		t.Errorf("want unknown status error, got %v", err)
	}
	if _, err := run(t, "list", "--epic", "epic-does-not-exist"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("want epic not found error, got %v", err)
	}
	if _, err := run(t, "list", "extra-arg"); err == nil {
		t.Errorf("want error for positional arg")
	}
}

func TestParseTaskStatusNormalises(t *testing.T) {
	got, err := parseTaskStatus(" In-Progress ")
	if err != nil || got != types.TaskStatusInProgress {
		t.Fatalf("got %q, %v", got, err)
	}
}
