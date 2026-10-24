package executor

import (
	"testing"

	"github.com/cloud-shuttle/drover/internal/sessionevent"
	"github.com/cloud-shuttle/drover/pkg/types"
)

func TestBeginWorktreeSessionMatchesSchema(t *testing.T) {
	dir := t.TempDir()
	path, sid := beginWorktreeSession(dir, &types.Task{ID: "CLO-9"})
	if path == "" || sid != "CLO-9" {
		t.Fatalf("path %q sid %q", path, sid)
	}
	got, err := sessionevent.ReadAll(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d events", len(got))
	}
	if got[0].Type != sessionevent.TypeStarted || got[0].V != sessionevent.Version || got[0].IssueID != "CLO-9" || got[0].Phase != "execute" {
		t.Fatalf("started: %+v", got[0])
	}
	if got[1].Type != sessionevent.TypeLinked || got[1].Data["issue_id"] != "CLO-9" {
		t.Fatalf("linked: %+v", got[1])
	}
	if got[2].Type != sessionevent.TypeStatus || got[2].Data["status"] != "running" {
		t.Fatalf("status: %+v", got[2])
	}
}
