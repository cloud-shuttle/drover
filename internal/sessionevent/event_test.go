package sessionevent

import (
	"path/filepath"
	"testing"
)

func TestAppendAndReadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions", "s.jsonl")
	started := New(TypeStarted, "sess-1", nil)
	started.IssueID = "CLO-1"
	started.Phase = "research"
	if err := Append(path, started); err != nil {
		t.Fatal(err)
	}
	if err := Append(path, New(TypeStatus, "sess-1", map[string]any{"status": "running"})); err != nil {
		t.Fatal(err)
	}
	if err := Append(path, New(TypeToolRequested, "sess-1", map[string]any{"name": "read_file", "id": "t1"})); err != nil {
		t.Fatal(err)
	}

	got, err := ReadAll(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("lines: got %d", len(got))
	}
	if got[0].V != Version || got[0].Type != TypeStarted || got[0].SessionID != "sess-1" || got[0].IssueID != "CLO-1" || got[0].Phase != "research" {
		t.Fatalf("started: %+v", got[0])
	}
	if got[1].Type != TypeStatus || got[1].Data["status"] != "running" {
		t.Fatalf("status: %+v", got[1])
	}
	if got[2].Type != TypeToolRequested || got[2].Data["name"] != "read_file" {
		t.Fatalf("tool: %+v", got[2])
	}
}
