package executor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cloud-shuttle/drover/pkg/types"
)

func TestNewAgent_ResolvesBinaryPerType(t *testing.T) {
	check := func(agentType, path string) Agent {
		t.Helper()
		a, err := NewAgent(&AgentConfig{Type: agentType, Path: path, Timeout: time.Minute})
		if err != nil {
			t.Fatalf("NewAgent(%q): %v", agentType, err)
		}
		return a
	}

	// The config default path is "claude"; other types must not inherit it.
	if a, ok := check("opencode", "claude").(*OpenCodeAgent); !ok || a.opencodePath != "opencode" {
		t.Errorf("opencode: got %T %+v, want *OpenCodeAgent running opencode", a, a)
	}
	if a, ok := check("codex", "claude").(*CodexAgent); !ok || a.codexPath != "codex" {
		t.Errorf("codex: got %T", a)
	}
	if a, ok := check("amp", "").(*AmpAgent); !ok || a.ampPath != "amp" {
		t.Errorf("amp: got %T", a)
	}
	if a, ok := check("opencode", "/opt/opencode").(*OpenCodeAgent); !ok || a.opencodePath != "/opt/opencode" {
		t.Errorf("explicit opencode path not kept")
	}
	if a, ok := check("claude", "claude").(*ClaudeAgent); !ok || a.claudePath != "claude" {
		t.Errorf("claude: got %T", a)
	}
}

func TestNewAgent_SelectsDroverCode(t *testing.T) {
	for _, typ := range []string{"drovercode", "drover-code", "Drover-Code"} {
		a, err := NewAgent(&AgentConfig{Type: typ, Path: "claude", Timeout: time.Minute})
		if err != nil {
			t.Fatal(err)
		}
		dc, ok := a.(*DroverCodeAgent)
		if !ok {
			t.Fatalf("%q: got %T, want *DroverCodeAgent", typ, a)
		}
		if dc.binaryPath != "drover-code" {
			t.Errorf("%q: binaryPath = %q, want drover-code", typ, dc.binaryPath)
		}
	}
	a, _ := NewAgent(&AgentConfig{Type: "drovercode", Path: "/opt/dc", Timeout: time.Minute})
	if dc := a.(*DroverCodeAgent); dc.binaryPath != "/opt/dc" {
		t.Errorf("explicit drover-code path not kept: %q", dc.binaryPath)
	}
}

func TestDroverCodeAgent_BuildPrompt_ContextAndGuidance(t *testing.T) {
	a := NewDroverCodeAgent("fake", time.Minute)
	task := &types.Task{
		ID: "t2", Title: "Add login",
		ExecutionContext: &types.TaskExecutionContext{
			Guidance: []*types.GuidanceMessage{{Message: "use bcrypt"}},
		},
	}
	a.SetTaskContext([]*types.Task{{ID: "t1", Title: "Create users table", Status: types.TaskStatusCompleted}}, 5)
	p := a.buildPrompt(task)
	for _, want := range []string{"HUMAN GUIDANCE", "use bcrypt", "Add login"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q:\n%s", want, p)
		}
	}
}

// TestDroverCodeAgent_ExecuteLeavesWorktreeClean runs a fake drover-code in a
// real git repo and checks the result file and session log never end up in
// what `git add -A` would commit.
func TestDroverCodeAgent_ExecuteLeavesWorktreeClean(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repo := t.TempDir()
	gitRun := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	gitRun("init", "-q", "-b", "main")
	gitRun("-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "--allow-empty", "-m", "init")

	bin := filepath.Join(t.TempDir(), "drover-code")
	script := `#!/bin/sh
cat >/dev/null
echo "work" > work.txt
printf '{"success":true,"output":"done from result"}' > "$DROVER_CODE_RESULT_PATH"
echo '{"type":"tool"}' >> "$DROVER_SESSION_EVENTS"
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	a, err := NewAgent(&AgentConfig{Type: "drover-code", Path: bin, Timeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	res := a.ExecuteWithContext(context.Background(), repo, &types.Task{ID: "t1", Title: "x"})
	if res.Error != nil || !res.Success || res.Output != "done from result" {
		t.Fatalf("result = %+v", res)
	}
	if _, err := os.Stat(filepath.Join(repo, ".drover", "session-events.jsonl")); err != nil {
		t.Fatalf("session events not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, ".drover-result.json")); err == nil {
		t.Fatal("result file must not be written into the worktree")
	}
	if st := gitRun("status", "--porcelain", "--untracked-files=all"); st != "?? work.txt" {
		t.Fatalf("only the agent's work should be committable, got:\n%s", st)
	}
}

func TestEnsureSessionEventsIgnored_RespectsExisting(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".drover")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := []byte("custom\n")
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), existing, 0o644); err != nil {
		t.Fatal(err)
	}
	ensureSessionEventsIgnored(dir)
	got, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if string(got) != string(existing) {
		t.Fatalf("existing .gitignore was modified: %q", got)
	}
}
