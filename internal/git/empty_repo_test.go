package git_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/cloud-shuttle/drover/internal/git"
	"github.com/cloud-shuttle/drover/pkg/types"
)

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func gitFails(dir string, args ...string) bool {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd.Run() != nil
}

// newEmptyRepo runs `git init -b <branch>` with a local identity and no commits.
func newEmptyRepo(t *testing.T, branch string, withIdentity bool) (string, *git.WorktreeManager) {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-q", "-b", branch)
	if withIdentity {
		runGit(t, dir, "config", "user.email", "test@example.com")
		runGit(t, dir, "config", "user.name", "Test User")
	}
	return dir, git.NewWorktreeManager(dir, filepath.Join(dir, ".drover", "worktrees"))
}

func TestEmptyRepo_CreateCommitMerge(t *testing.T) {
	dir, wm := newEmptyRepo(t, "main", true)

	wt, err := wm.Create(&types.Task{ID: "task-1", Title: "first task"})
	if err != nil {
		t.Fatalf("Create on empty repo: %v", err)
	}
	if got := runGit(t, wt, "rev-parse", "--abbrev-ref", "HEAD"); got != "drover-task-1" {
		t.Fatalf("worktree branch = %q, want drover-task-1", got)
	}
	// main has a single empty root commit and the task branch descends from it.
	root := runGit(t, dir, "rev-parse", "main")
	if n := runGit(t, dir, "rev-list", "--count", "main"); n != "1" {
		t.Fatalf("main should have 1 commit, has %s", n)
	}
	if files := runGit(t, dir, "ls-tree", "-r", "--name-only", "main"); files != "" {
		t.Fatalf("initial commit should be empty, has %q", files)
	}
	runGit(t, dir, "merge-base", "--is-ancestor", root, "drover-task-1")

	if err := os.WriteFile(filepath.Join(wt, "hello.txt"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if changed, err := wm.Commit("task-1", "add hello"); err != nil || !changed {
		t.Fatalf("Commit: changed=%v err=%v", changed, err)
	}
	if err := wm.MergeToMain("task-1"); err != nil {
		t.Fatalf("MergeToMain: %v", err)
	}
	if got := runGit(t, dir, "show", "main:hello.txt"); got != "hi" {
		t.Fatalf("main:hello.txt = %q", got)
	}
	runGit(t, dir, "merge-base", "--is-ancestor", root, "main")
}

func TestEmptyRepo_PreservesStagedAndUntrackedFiles(t *testing.T) {
	dir, wm := newEmptyRepo(t, "main", true)
	if err := os.WriteFile(filepath.Join(dir, "staged.txt"), []byte("keep me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("me too\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "staged.txt")

	created, err := wm.EnsureMainBranch()
	if err != nil || !created {
		t.Fatalf("EnsureMainBranch: created=%v err=%v", created, err)
	}
	if got := runGit(t, dir, "diff", "--cached", "--name-only"); got != "staged.txt" {
		t.Fatalf("staged files after init = %q, want staged.txt", got)
	}
	for _, f := range []string{"staged.txt", "untracked.txt"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatalf("%s was removed: %v", f, err)
		}
	}
	if files := runGit(t, dir, "ls-tree", "-r", "--name-only", "main"); files != "" {
		t.Fatalf("staged file must not be committed, main has %q", files)
	}
}

func TestEmptyRepo_UnbornMasterMovesHEADToMain(t *testing.T) {
	dir, wm := newEmptyRepo(t, "master", true)
	if _, err := wm.Create(&types.Task{ID: "task-1"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if head := runGit(t, dir, "symbolic-ref", "HEAD"); head != "refs/heads/main" {
		t.Fatalf("HEAD = %q, want refs/heads/main", head)
	}
	if !gitFails(dir, "rev-parse", "--verify", "--quiet", "refs/heads/master") {
		t.Fatalf("master should not have been created")
	}
}

func TestEmptyRepo_NoIdentityFallsBackToDrover(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "nonexistent"))
	for _, k := range []string{"GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL", "EMAIL"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
	dir, wm := newEmptyRepo(t, "main", false)
	if _, err := wm.EnsureMainBranch(); err != nil {
		t.Fatalf("EnsureMainBranch without identity: %v", err)
	}
	if got := runGit(t, dir, "log", "-1", "--format=%an <%ae>", "main"); got != "Drover <drover@localhost>" {
		t.Fatalf("author = %q", got)
	}
}

func TestEnsureMainBranch_NoOpOnExistingHistory(t *testing.T) {
	dir, wm := setupTestRepo(t)
	before := runGit(t, dir, "rev-parse", "main")
	created, err := wm.EnsureMainBranch()
	if err != nil || created {
		t.Fatalf("EnsureMainBranch on non-empty repo: created=%v err=%v", created, err)
	}
	if after := runGit(t, dir, "rev-parse", "main"); after != before {
		t.Fatalf("main moved from %s to %s", before, after)
	}

	// Second call on a freshly initialised repo is idempotent too.
	_, wm2 := newEmptyRepo(t, "main", true)
	if c, err := wm2.EnsureMainBranch(); err != nil || !c {
		t.Fatalf("first call: %v %v", c, err)
	}
	if c, err := wm2.EnsureMainBranch(); err != nil || c {
		t.Fatalf("second call should be a no-op: %v %v", c, err)
	}
}

func TestEnsureMainBranch_ConcurrentCallsCreateOneRoot(t *testing.T) {
	dir, wm := newEmptyRepo(t, "main", true)
	var wg sync.WaitGroup
	var mu sync.Mutex
	createdCount := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := wm.EnsureMainBranch()
			if err != nil {
				t.Errorf("EnsureMainBranch: %v", err)
				return
			}
			if c {
				mu.Lock()
				createdCount++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if createdCount != 1 {
		t.Fatalf("created %d initial commits, want 1", createdCount)
	}
	if n := runGit(t, dir, "rev-list", "--count", "--all"); n != "1" {
		t.Fatalf("repo has %s commits, want 1", n)
	}
}

func TestMergeToMain_NoMainBranchKeepsWork(t *testing.T) {
	dir, wm := newEmptyRepo(t, "master", true)
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-q", "-m", "init on master")
	masterBefore := runGit(t, dir, "rev-parse", "master")

	wt, err := wm.Create(&types.Task{ID: "task-1"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !gitFails(dir, "rev-parse", "--verify", "--quiet", "refs/heads/main") {
		t.Fatalf("drover must not create main in a repo that already has history")
	}
	if err := os.WriteFile(filepath.Join(wt, "work.txt"), []byte("w\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := wm.Commit("task-1", "work"); err != nil {
		t.Fatal(err)
	}
	err = wm.MergeToMain("task-1")
	if !errors.Is(err, git.ErrNoMainBranch) {
		t.Fatalf("MergeToMain err = %v, want ErrNoMainBranch", err)
	}
	if got := runGit(t, dir, "rev-parse", "master"); got != masterBefore {
		t.Fatalf("master moved")
	}
	runGit(t, dir, "rev-parse", "--verify", "drover-task-1")
}

func TestMergeToMain_UnrelatedHistoriesNeverResets(t *testing.T) {
	dir, wm := setupTestRepo(t)
	mainBefore := runGit(t, dir, "rev-parse", "main")

	// Build an orphan drover-task-1 branch with its own root commit.
	cmd := exec.Command("git", "hash-object", "-w", "--stdin")
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader("orphan work\n")
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	mk := exec.Command("git", "mktree")
	mk.Dir = dir
	mk.Stdin = strings.NewReader("100644 blob " + strings.TrimSpace(string(out)) + "\torphan.txt\n")
	tree, err := mk.Output()
	if err != nil {
		t.Fatal(err)
	}
	orphan := runGit(t, dir, "commit-tree", strings.TrimSpace(string(tree)), "-m", "orphan root")
	runGit(t, dir, "branch", "drover-task-1", orphan)

	err = wm.MergeToMain("task-1")
	if !errors.Is(err, git.ErrUnrelatedHistories) {
		t.Fatalf("MergeToMain err = %v, want ErrUnrelatedHistories", err)
	}
	if got := runGit(t, dir, "rev-parse", "main"); got != mainBefore {
		t.Fatalf("main was rewritten: %s -> %s", mainBefore, got)
	}
	if _, err := os.Stat(filepath.Join(dir, "README.md")); err != nil {
		t.Fatalf("working tree file lost: %v", err)
	}
	if got := runGit(t, dir, "rev-parse", "drover-task-1"); got != orphan {
		t.Fatalf("task branch was changed or deleted")
	}
	if st := runGit(t, dir, "status", "--porcelain", "--untracked-files=no"); st != "" {
		t.Fatalf("working tree left dirty: %q", st)
	}
}
