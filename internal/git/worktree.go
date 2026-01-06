// Package git handles git worktree operations for parallel task execution
package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cloud-shuttle/drover/pkg/types"
)

// WorktreeManager creates and manages git worktrees
type WorktreeManager struct {
	baseDir    string // Base repository directory
	worktreeDir string // Where worktrees are created (.drover/worktrees)
}

// NewWorktreeManager creates a new worktree manager
func NewWorktreeManager(baseDir, worktreeDir string) *WorktreeManager {
	return &WorktreeManager{
		baseDir:     baseDir,
		worktreeDir: worktreeDir,
	}
}

// Create creates a new worktree for a task
func (wm *WorktreeManager) Create(task *types.Task) (string, error) {
	worktreePath := filepath.Join(wm.worktreeDir, task.ID)

	// Ensure worktree directory exists
	if err := os.MkdirAll(wm.worktreeDir, 0755); err != nil {
		return "", fmt.Errorf("creating worktree directory: %w", err)
	}

	// Create the worktree
	cmd := exec.Command("git", "worktree", "add", worktreePath)
	cmd.Dir = wm.baseDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("creating worktree: %w\n%s", err, output)
	}

	return worktreePath, nil
}

// Remove removes a worktree
func (wm *WorktreeManager) Remove(taskID string) error {
	worktreePath := filepath.Join(wm.worktreeDir, taskID)

	// Remove the worktree
	cmd := exec.Command("git", "worktree", "remove", worktreePath)
	cmd.Dir = wm.baseDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("removing worktree: %w\n%s", err, output)
	}

	return nil
}

// Commit commits all changes in the worktree
func (wm *WorktreeManager) Commit(taskID, message string) error {
	worktreePath := filepath.Join(wm.worktreeDir, taskID)

	// Stage all changes
	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = worktreePath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("staging changes: %w\n%s", err, output)
	}

	// Commit
	cmd = exec.Command("git", "commit", "-m", message)
	cmd.Dir = worktreePath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("committing: %w\n%s", err, output)
	}

	return nil
}

// MergeToMain merges the worktree changes to main branch
func (wm *WorktreeManager) MergeToMain(taskID string) error {
	worktreePath := filepath.Join(wm.worktreeDir, taskID)
	branchName := fmt.Sprintf("drover-%s", taskID)

	// Create a branch from the worktree
	cmd := exec.Command("git", "checkout", "-b", branchName)
	cmd.Dir = worktreePath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("creating branch: %w\n%s", err, output)
	}

	// Switch to main in base repo
	cmd = exec.Command("git", "checkout", "main")
	cmd.Dir = wm.baseDir
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("checking out main: %w\n%s", err, output)
	}

	// Merge the branch
	cmd = exec.Command("git", "merge", "--no-ff", branchName, "-m", fmt.Sprintf("drover: Merge %s", taskID))
	cmd.Dir = wm.baseDir
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("merging: %w\n%s", err, output)
	}

	// Delete the branch
	cmd = exec.Command("git", "branch", "-d", branchName)
	cmd.Dir = wm.baseDir
	_, _ = cmd.CombinedOutput() // Ignore errors on branch delete

	return nil
}

// Cleanup removes all worktrees
func (wm *WorktreeManager) Cleanup() error {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = wm.baseDir
	output, err := cmd.Output()
	if err != nil {
		return nil // No worktrees to clean
	}

	// Parse and remove each worktree
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		worktreePath := parts[1]

		// Only remove worktrees in our directory
		if filepath.Dir(worktreePath) == wm.worktreeDir || filepath.HasPrefix(worktreePath, wm.worktreeDir+"/") {
			cmd := exec.Command("git", "worktree", "remove", worktreePath)
			cmd.Dir = wm.baseDir
			_ = cmd.Run() // Ignore errors
		}
	}

	return nil
}

// Path returns the worktree path for a task
func (wm *WorktreeManager) Path(taskID string) string {
	return filepath.Join(wm.worktreeDir, taskID)
}
