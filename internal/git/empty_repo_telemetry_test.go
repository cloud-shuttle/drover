package git_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/cloud-shuttle/drover/pkg/telemetry"
	"github.com/cloud-shuttle/drover/pkg/types"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

var (
	spanRecOnce sync.Once
	spanRec     *tracetest.SpanRecorder
)

// recordSpans installs one SDK tracer provider for the whole test binary.
// The otel global tracer delegates to the first provider set, so it can't be
// swapped per test; tests filter spans by task ID or by count instead.
func recordSpans(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	spanRecOnce.Do(func() {
		spanRec = tracetest.NewSpanRecorder()
		otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRec)))
	})
	return spanRec
}

func spanAttrs(s sdktrace.ReadOnlySpan) map[attribute.Key]attribute.Value {
	m := map[attribute.Key]attribute.Value{}
	for _, kv := range s.Attributes() {
		m[kv.Key] = kv.Value
	}
	return m
}

func findSpans(rec *tracetest.SpanRecorder, name, taskID string) []sdktrace.ReadOnlySpan {
	var out []sdktrace.ReadOnlySpan
	for _, s := range rec.Ended() {
		if s.Name() != name {
			continue
		}
		if taskID != "" && spanAttrs(s)[telemetry.KeyTaskID].AsString() != taskID {
			continue
		}
		out = append(out, s)
	}
	return out
}

func TestTelemetry_EnsureMainAndMergeSpans(t *testing.T) {
	rec := recordSpans(t)
	_, wm := newEmptyRepo(t, "main", true)
	before := len(findSpans(rec, telemetry.SpanGitEnsureMain, ""))

	wt, err := wm.Create(&types.Task{ID: "task-otel"})
	if err != nil {
		t.Fatal(err)
	}
	ensure := findSpans(rec, telemetry.SpanGitEnsureMain, "")
	if len(ensure) <= before {
		t.Fatalf("no %s span recorded", telemetry.SpanGitEnsureMain)
	}
	if a := spanAttrs(ensure[len(ensure)-1]); !a[telemetry.KeyGitCreatedInitCommit].AsBool() || a[telemetry.KeyGitBranch].AsString() != "main" {
		t.Errorf("ensure_main attrs = %v", a)
	}

	if err := os.WriteFile(filepath.Join(wt, "f.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := wm.Commit("task-otel", "f"); err != nil {
		t.Fatal(err)
	}
	if err := wm.MergeToMainContext(context.Background(), "task-otel"); err != nil {
		t.Fatal(err)
	}
	merges := findSpans(rec, telemetry.SpanGitWorktreeMerge, "task-otel")
	if len(merges) != 1 {
		t.Fatalf("want 1 merge span, got %d", len(merges))
	}
	a := spanAttrs(merges[0])
	if a[telemetry.KeyTaskID].AsString() != "task-otel" || !a[telemetry.KeyRepoHadCommits].AsBool() {
		t.Errorf("merge attrs = %v", a)
	}
	if merges[0].Status().Code == codes.Error {
		t.Errorf("successful merge span marked as error")
	}
}

func TestTelemetry_MergeSpanRecordsError(t *testing.T) {
	rec := recordSpans(t)
	dir, wm := newEmptyRepo(t, "master", true)
	if err := os.WriteFile(filepath.Join(dir, "a"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "a")
	runGit(t, dir, "commit", "-q", "-m", "a")
	runGit(t, dir, "branch", "drover-task-x")
	runGit(t, dir, "commit", "-q", "--allow-empty", "-m", "ahead")
	runGit(t, dir, "branch", "-f", "drover-task-x", "HEAD")

	if err := wm.MergeToMain("task-x"); err == nil {
		t.Fatal("expected ErrNoMainBranch")
	}
	merges := findSpans(rec, telemetry.SpanGitWorktreeMerge, "task-x")
	if len(merges) != 1 || merges[0].Status().Code != codes.Error {
		t.Fatalf("merge span should be recorded with error status: %+v", merges)
	}
}
