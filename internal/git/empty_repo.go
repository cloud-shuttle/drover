package git

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// mainBranch is the branch drover merges task branches into.
const mainBranch = "main"

// ErrNoMainBranch is returned by MergeToMain when the repository has no
// "main" branch to merge into.
var ErrNoMainBranch = errors.New("target branch 'main' does not exist")

// ErrUnrelatedHistories is returned by MergeToMain when the task branch and
// main share no history. Drover never resolves this by resetting main; the
// task branch is left in place so no work is lost.
var ErrUnrelatedHistories = errors.New("task branch and main have unrelated histories")

// initMutex serialises EnsureMainBranch so parallel workers don't race to
// create the initial commit.
var initMutex sync.Mutex

// gitOK runs a git command in dir and reports whether it exited 0.
func gitOK(dir string, args ...string) bool {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd.Run() == nil
}

// gitOut runs a git command in dir and returns trimmed stdout.
func gitOut(dir string, env []string, stdin []byte, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}

// branchExists reports whether refs/heads/<name> points at a commit.
func branchExists(dir, name string) bool {
	return gitOK(dir, "rev-parse", "--verify", "--quiet", "refs/heads/"+name+"^{commit}")
}

// repoHasCommits reports whether any ref in the repository points at a commit.
func repoHasCommits(dir string) bool {
	out, err := gitOut(dir, nil, nil, "rev-list", "-n", "1", "--all")
	return err == nil && out != ""
}

// EnsureMainBranch makes a brand-new, empty repository usable by drover.
//
// If the repository has no commits at all, it creates an empty root commit
// on "main" and, if HEAD is an unborn branch, points HEAD at main. It does
// this with plumbing (mktree, commit-tree, update-ref) so the user's working
// tree and index are never touched: anything they have staged stays staged.
//
// If the repository already has commits, EnsureMainBranch does nothing,
// even when there is no "main" branch. It never resets, rewrites or deletes
// existing history. It returns true when it created the initial commit.
func (wm *WorktreeManager) EnsureMainBranch() (bool, error) {
	initMutex.Lock()
	defer initMutex.Unlock()

	if branchExists(wm.baseDir, mainBranch) || repoHasCommits(wm.baseDir) {
		return false, nil
	}
	if !gitOK(wm.baseDir, "rev-parse", "--git-dir") {
		return false, fmt.Errorf("%s is not a git repository", wm.baseDir)
	}

	emptyTree, err := gitOut(wm.baseDir, nil, []byte{}, "mktree")
	if err != nil {
		return false, fmt.Errorf("creating empty tree: %w", err)
	}

	// Use the user's identity when configured; otherwise fall back to a
	// drover identity for this one commit rather than failing.
	var env []string
	if !gitOK(wm.baseDir, "var", "GIT_COMMITTER_IDENT") || !gitOK(wm.baseDir, "var", "GIT_AUTHOR_IDENT") {
		env = []string{
			"GIT_AUTHOR_NAME=Drover", "GIT_AUTHOR_EMAIL=drover@localhost",
			"GIT_COMMITTER_NAME=Drover", "GIT_COMMITTER_EMAIL=drover@localhost",
		}
	}
	commit, err := gitOut(wm.baseDir, env, nil, "commit-tree", emptyTree, "-m", "Initial commit (created by drover for an empty repository)")
	if err != nil {
		return false, fmt.Errorf("creating initial commit: %w", err)
	}

	// Create refs/heads/main only if it does not already exist. The all-zero
	// old value makes update-ref refuse to overwrite an existing ref.
	zero := strings.Repeat("0", len(commit))
	if _, err := gitOut(wm.baseDir, nil, nil, "update-ref", "-m", "drover: initial commit", "refs/heads/"+mainBranch, commit, zero); err != nil {
		if branchExists(wm.baseDir, mainBranch) {
			return false, nil
		}
		return false, fmt.Errorf("creating %s: %w", mainBranch, err)
	}

	// If HEAD is a symbolic ref to an unborn branch (e.g. "master" from an
	// older init.defaultBranch), move it to main. An unborn branch has no
	// commits, so nothing can be lost.
	if head, err := gitOut(wm.baseDir, nil, nil, "symbolic-ref", "-q", "HEAD"); err == nil && head != "refs/heads/"+mainBranch {
		if !gitOK(wm.baseDir, "rev-parse", "--verify", "--quiet", head+"^{commit}") {
			if _, err := gitOut(wm.baseDir, nil, nil, "symbolic-ref", "HEAD", "refs/heads/"+mainBranch); err != nil {
				return true, fmt.Errorf("pointing HEAD at %s: %w", mainBranch, err)
			}
		}
	}

	log.Printf("🌱 Empty repository: created initial commit %s on %s", commit[:min(12, len(commit))], mainBranch)
	return true, nil
}
