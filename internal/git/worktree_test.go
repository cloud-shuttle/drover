// Package git_test provides tests for the git package
package git_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloud-shuttle/drover/internal/git"
	"github.com/cloud-shuttle/drover/pkg/types"
)

// setupTestRepo creates a temporary git repository for testing
func setupTestRepo(t *testing.T) (string, *git.WorktreeManager) {
	t.Helper()

	// Create temp directory
	tmpDir := t.TempDir()

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Configure git
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to set git email: %v", err)
	}

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to set git name: %v", err)
	}

	// Create initial commit
	initialFile := filepath.Join(tmpDir, "README.md")
	if err := os.WriteFile(initialFile, []byte("# Test Repo\n"), 0644); err != nil {
		t.Fatalf("Failed to create initial file: %v", err)
	}

	cmd = exec.Command("git", "add", "README.md")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to add initial file: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create initial commit: %v", err)
	}

	// Rename branch to main (in case git init created master or another name)
	cmd = exec.Command("git", "branch", "-M", "main")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to rename branch to main: %v", err)
	}

	// Create worktree directory
	worktreeDir := filepath.Join(tmpDir, ".drover", "worktrees")

	worktreeMgr := git.NewWorktreeManager(tmpDir, worktreeDir)
	worktreeMgr.SetVerbose(true)

	return tmpDir, worktreeMgr
}

// TestWorktreeManager_Create verifies worktree creation
func TestWorktreeManager_Create(t *testing.T) {
	baseDir, wm := setupTestRepo(t)

	task := &types.Task{
		ID:    "task-123",
		Title: "Test Task",
	}

	worktreePath, err := wm.Create(task)
	if err != nil {
		t.Fatalf("Failed to create worktree: %v", err)
	}

	// Verify worktree directory exists
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		t.Error("Worktree directory was not created")
	}

	// Verify worktree contains the same files
	expectedFile := filepath.Join(worktreePath, "README.md")
	if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
		t.Error("Worktree does not contain expected files")
	}

	// Verify git recognizes the worktree
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = baseDir
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to list worktrees: %v", err)
	}

	if !strings.Contains(string(output), worktreePath) {
		t.Errorf("Worktree %s not found in git worktree list", worktreePath)
	}

	// Cleanup
	wm.Remove(task.ID)
}

// TestWorktreeManager_Remove verifies worktree removal
func TestWorktreeManager_Remove(t *testing.T) {
	_, wm := setupTestRepo(t)

	task := &types.Task{
		ID:    "task-456",
		Title: "Test Task",
	}

	// Create worktree
	worktreePath, err := wm.Create(task)
	if err != nil {
		t.Fatalf("Failed to create worktree: %v", err)
	}

	// Verify it exists
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		t.Fatal("Worktree was not created")
	}

	// Remove worktree
	err = wm.Remove(task.ID)
	if err != nil {
		t.Fatalf("Failed to remove worktree: %v", err)
	}

	// Verify it's gone
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Error("Worktree directory still exists after removal")
	}

	// Removing a non-existent worktree should not error
	err = wm.Remove(task.ID)
	if err != nil {
		t.Errorf("Removing non-existent worktree should not error, got: %v", err)
	}
}

// TestWorktreeManager_Commit_WithChanges verifies committing actual changes
func TestWorktreeManager_Commit_WithChanges(t *testing.T) {
	_, wm := setupTestRepo(t)

	task := &types.Task{
		ID:    "task-789",
		Title: "Test Task",
	}

	// Create worktree
	worktreePath, err := wm.Create(task)
	if err != nil {
		t.Fatalf("Failed to create worktree: %v", err)
	}
	defer wm.Remove(task.ID)

	// Make changes in the worktree
	testFile := filepath.Join(worktreePath, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content\n"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Commit changes
	commitMsg := "test commit"
	hasChanges, err := wm.Commit(task.ID, commitMsg)
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}
	if !hasChanges {
		t.Fatal("Expected hasChanges to be true when we made changes")
	}

	// Verify commit was created
	cmd := exec.Command("git", "log", "--oneline", "-1")
	cmd.Dir = worktreePath
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to get log: %v", err)
	}

	if !strings.Contains(string(output), "test commit") {
		t.Errorf("Expected commit message not found in log: %s", output)
	}
}

// TestWorktreeManager_Commit_NoChanges verifies that committing without changes succeeds
func TestWorktreeManager_Commit_NoChanges(t *testing.T) {
	_, wm := setupTestRepo(t)

	task := &types.Task{
		ID:    "task-nochanges",
		Title: "Test Task",
	}

	// Create worktree
	worktreePath, err := wm.Create(task)
	if err != nil {
		t.Fatalf("Failed to create worktree: %v", err)
	}
	defer wm.Remove(task.ID)

	// Don't make any changes - commit should succeed anyway
	commitMsg := "test commit"
	hasChanges, err := wm.Commit(task.ID, commitMsg)
	if err != nil {
		t.Fatalf("Commit with no changes should succeed, got: %v", err)
	}
	if hasChanges {
		t.Error("Expected hasChanges to be false when no changes were made")
	}

	// Verify no new commit was created
	cmd := exec.Command("git", "log", "--oneline")
	cmd.Dir = worktreePath
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to get log: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	// Should still have just 1 commit (the initial one from setup)
	if len(lines) != 1 {
		t.Errorf("Expected 1 commit, got %d: %s", len(lines), output)
	}
}

// TestWorktreeManager_MergeToMain_WithChanges verifies merging changes to main
func TestWorktreeManager_MergeToMain_WithChanges(t *testing.T) {
	baseDir, wm := setupTestRepo(t)

	task := &types.Task{
		ID:    "task-merge",
		Title: "Test Task",
	}

	// Create worktree
	worktreePath, err := wm.Create(task)
	if err != nil {
		t.Fatalf("Failed to create worktree: %v", err)
	}
	defer wm.Remove(task.ID)

	// Make and commit changes in the worktree
	testFile := filepath.Join(worktreePath, "merge-test.txt")
	if err := os.WriteFile(testFile, []byte("merge test content\n"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	commitMsg := "test commit for merge"
	_, err = wm.Commit(task.ID, commitMsg)
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Merge to main
	err = wm.MergeToMain(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("Failed to merge to main: %v", err)
	}

	// Verify file exists in main
	mainFile := filepath.Join(baseDir, "merge-test.txt")
	if _, err := os.Stat(mainFile); os.IsNotExist(err) {
		t.Error("File was not merged to main branch")
	}

	// Verify the commit is in main's history
	cmd := exec.Command("git", "log", "--oneline")
	cmd.Dir = baseDir
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to get log: %v", err)
	}

	if !strings.Contains(string(output), "test commit for merge") {
		t.Errorf("Merge commit not found in main history: %s", output)
	}
}

// TestWorktreeManager_MergeToMain_NoChanges verifies merge with no commits
func TestWorktreeManager_MergeToMain_NoChanges(t *testing.T) {
	_, wm := setupTestRepo(t)

	task := &types.Task{
		ID:    "task-nomerge",
		Title: "Test Task",
	}

	// Create worktree but don't make any changes
	worktreePath, err := wm.Create(task)
	if err != nil {
		t.Fatalf("Failed to create worktree: %v", err)
	}
	defer wm.Remove(task.ID)

	// Merge should succeed even with no changes
	err = wm.MergeToMain(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("Merge with no changes should succeed, got: %v", err)
	}

	// Verify no new commits in main
	cmd := exec.Command("git", "log", "--oneline")
	cmd.Dir = worktreePath
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to get log: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) != 1 {
		t.Errorf("Expected 1 commit in worktree, got %d: %s", len(lines), output)
	}
}

// TestWorktreeManager_MultipleWorktrees verifies multiple concurrent worktrees
func TestWorktreeManager_MultipleWorktrees(t *testing.T) {
	baseDir, wm := setupTestRepo(t)

	tasks := []*types.Task{
		{ID: "task-1", Title: "Task 1"},
		{ID: "task-2", Title: "Task 2"},
		{ID: "task-3", Title: "Task 3"},
	}

	worktreePaths := make([]string, 0, len(tasks))

	// Create multiple worktrees
	for _, task := range tasks {
		path, err := wm.Create(task)
		if err != nil {
			t.Fatalf("Failed to create worktree %s: %v", task.ID, err)
		}
		worktreePaths = append(worktreePaths, path)

		// Verify each worktree is independent by creating a unique file
		testFile := filepath.Join(path, task.ID+".txt")
		if err := os.WriteFile(testFile, []byte(task.ID+"\n"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
	}

	// Verify all worktrees exist in git
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = baseDir
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to list worktrees: %v", err)
	}

	for _, path := range worktreePaths {
		if !strings.Contains(string(output), path) {
			t.Errorf("Worktree %s not found in git list", path)
		}
	}

	// Cleanup all worktrees
	for _, task := range tasks {
		wm.Remove(task.ID)
	}
}

// TestWorktreeManager_Path verifies the Path helper method
func TestWorktreeManager_Path(t *testing.T) {
	baseDir, wm := setupTestRepo(t)

	taskID := "task-path-test"
	expectedPath := filepath.Join(baseDir, ".drover", "worktrees", taskID)

	actualPath := wm.Path(taskID)
	if actualPath != expectedPath {
		t.Errorf("Path() returned %s, expected %s", actualPath, expectedPath)
	}
}

// TestWorktreeManager_Cleanup verifies cleanup of all worktrees
func TestWorktreeManager_Cleanup(t *testing.T) {
	baseDir, wm := setupTestRepo(t)

	tasks := []*types.Task{
		{ID: "task-cleanup-1", Title: "Task 1"},
		{ID: "task-cleanup-2", Title: "Task 2"},
	}

	// Create worktrees
	for _, task := range tasks {
		_, err := wm.Create(task)
		if err != nil {
			t.Fatalf("Failed to create worktree: %v", err)
		}
	}

	// Cleanup all
	err := wm.Cleanup()
	if err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	// Verify worktrees are gone
	cmd := exec.Command("git", "worktree", "list")
	cmd.Dir = baseDir
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to list worktrees: %v", err)
	}

	outputStr := string(output)
	for _, task := range tasks {
		if strings.Contains(outputStr, task.ID) {
			t.Errorf("Worktree %s still exists after cleanup", task.ID)
		}
	}
}

// setupEmptyTestRepo creates a temporary empty git repository for testing
func setupEmptyTestRepo(t *testing.T) (string, *git.WorktreeManager) {
	t.Helper()

	// Create temp directory
	tmpDir := t.TempDir()

	// Initialize git repo (empty - no commits)
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Configure git
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to set git email: %v", err)
	}

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to set git name: %v", err)
	}

	// Create worktree directory
	worktreeDir := filepath.Join(tmpDir, ".drover", "worktrees")

	worktreeMgr := git.NewWorktreeManager(tmpDir, worktreeDir)
	worktreeMgr.SetVerbose(true)

	return tmpDir, worktreeMgr
}

// TestEnsureMainBranch_EmptyRepo verifies EnsureMainBranch creates main branch in empty repo
func TestEnsureMainBranch_EmptyRepo(t *testing.T) {
	baseDir, wm := setupEmptyTestRepo(t)

	// Verify main branch doesn't exist initially
	cmd := exec.Command("git", "rev-parse", "--verify", "main")
	cmd.Dir = baseDir
	err := cmd.Run()
	if err == nil {
		t.Fatal("Main branch should not exist in empty repo")
	}

	// Ensure main branch
	err = wm.EnsureMainBranch(context.Background())
	if err != nil {
		t.Fatalf("EnsureMainBranch failed: %v", err)
	}

	// Verify main branch exists and is current
	cmd = exec.Command("git", "branch", "--show-current")
	cmd.Dir = baseDir
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to get current branch: %v", err)
	}

	branch := strings.TrimSpace(string(output))
	if branch != "main" {
		t.Errorf("Expected current branch to be 'main', got '%s'", branch)
	}

	// Verify main has at least one commit (the empty initial commit)
	cmd = exec.Command("git", "rev-list", "--count", "main")
	cmd.Dir = baseDir
	output, err = cmd.Output()
	if err != nil {
		t.Fatalf("Failed to get commit count: %v", err)
	}

	count := strings.TrimSpace(string(output))
	if count == "0" {
		t.Error("Main branch should have at least 1 commit")
	}
}

// TestWorktreeManager_Create_EmptyRepo verifies worktree creation in empty repo
func TestWorktreeManager_Create_EmptyRepo(t *testing.T) {
	baseDir, wm := setupEmptyTestRepo(t)

	task := &types.Task{
		ID:    "task-empty-123",
		Title: "Test Task in Empty Repo",
	}

	// Create worktree - should handle empty repo automatically
	worktreePath, err := wm.Create(task)
	if err != nil {
		t.Fatalf("Failed to create worktree in empty repo: %v", err)
	}

	// Verify worktree directory exists
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		t.Error("Worktree directory was not created")
	}

	// Verify main branch was created
	cmd := exec.Command("git", "rev-parse", "--verify", "main")
	cmd.Dir = baseDir
	if err := cmd.Run(); err != nil {
		t.Error("Main branch should exist after worktree creation")
	}

	// Verify worktree is usable (can run git commands)
	// For empty repos, worktree will be on an orphan branch which may not have commits yet
	cmd = exec.Command("git", "status")
	cmd.Dir = worktreePath
	if err := cmd.Run(); err != nil {
		t.Errorf("Worktree should be usable: %v", err)
	}

	// Cleanup
	wm.Remove(task.ID)
}

// TestWorktreeManager_Commit_EmptyRepo verifies committing in empty repo worktree
func TestWorktreeManager_Commit_EmptyRepo(t *testing.T) {
	_, wm := setupEmptyTestRepo(t)

	task := &types.Task{
		ID:    "task-commit-empty",
		Title: "Test Commit in Empty Repo",
	}

	// Create worktree
	worktreePath, err := wm.Create(task)
	if err != nil {
		t.Fatalf("Failed to create worktree: %v", err)
	}
	defer wm.Remove(task.ID)

	// Make changes and commit
	testFile := filepath.Join(worktreePath, "new-file.txt")
	if err := os.WriteFile(testFile, []byte("new content\n"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	hasChanges, err := wm.Commit(task.ID, "Add new file in empty repo")
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}
	if !hasChanges {
		t.Error("Expected hasChanges to be true")
	}

	// Verify commit was created in worktree
	cmd := exec.Command("git", "log", "--oneline", "-1")
	cmd.Dir = worktreePath
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to get log: %v", err)
	}

	if !strings.Contains(string(output), "Add new file in empty repo") {
		t.Errorf("Expected commit message not found: %s", output)
	}
}

// TestWorktreeManager_MergeToMain_EmptyRepo verifies merging from empty repo worktree
func TestWorktreeManager_MergeToMain_EmptyRepo(t *testing.T) {
	baseDir, wm := setupEmptyTestRepo(t)

	task := &types.Task{
		ID:    "task-merge-empty",
		Title: "Test Merge from Empty Repo",
	}

	// Create worktree
	worktreePath, err := wm.Create(task)
	if err != nil {
		t.Fatalf("Failed to create worktree: %v", err)
	}
	defer wm.Remove(task.ID)

	// Make and commit changes
	testFile := filepath.Join(worktreePath, "merged-file.txt")
	if err := os.WriteFile(testFile, []byte("merged content\n"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err = wm.Commit(task.ID, "Add file to merge")
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Merge to main
	err = wm.MergeToMain(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("Failed to merge to main: %v", err)
	}

	// Verify file exists in main
	mainFile := filepath.Join(baseDir, "merged-file.txt")
	if _, err := os.Stat(mainFile); os.IsNotExist(err) {
		t.Error("File was not merged to main branch")
	}

	// Verify commit is in main's history
	cmd := exec.Command("git", "log", "--oneline")
	cmd.Dir = baseDir
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to get log: %v", err)
	}

	if !strings.Contains(string(output), "Add file to merge") {
		t.Errorf("Merge commit not found in main history: %s", output)
	}
}

// TestWorktreeManager_MergeToMain_WithoutCreate verifies MergeToMain works even if Create wasn't called
// This tests the EnsureMainBranch safety check
func TestWorktreeManager_MergeToMain_WithoutCreate(t *testing.T) {
	baseDir, wm := setupEmptyTestRepo(t)

	taskID := "task-merge-no-create"
	worktreePath := wm.Path(taskID)

	// Manually create worktree without using wm.Create()
	// This simulates a scenario where MergeToMain is called independently
	cmd := exec.Command("git", "worktree", "add", "--orphan", worktreePath)
	cmd.Dir = baseDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create worktree: %v", err)
	}
	defer wm.Remove(taskID)

	// Make and commit changes in the worktree
	testFile := filepath.Join(worktreePath, "manual-file.txt")
	if err := os.WriteFile(testFile, []byte("manual content\n"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cmd = exec.Command("git", "add", "manual-file.txt")
	cmd.Dir = worktreePath
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to stage changes: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "Manual commit")
	cmd.Dir = worktreePath
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Merge to main - should create main branch automatically
	err := wm.MergeToMain(context.Background(), taskID)
	if err != nil {
		t.Fatalf("MergeToMain failed without Create being called: %v", err)
	}

	// Verify main branch exists and has the file
	mainFile := filepath.Join(baseDir, "manual-file.txt")
	if _, err := os.Stat(mainFile); os.IsNotExist(err) {
		t.Error("File was not merged to main branch")
	}

	// Verify main branch was created
	cmd = exec.Command("git", "rev-parse", "--verify", "main")
	cmd.Dir = baseDir
	if err := cmd.Run(); err != nil {
		t.Error("Main branch was not created by MergeToMain")
	}
}
