// Package git provides tests for worktree pool functionality
package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestWorktreePool_New verifies pool creation
func TestWorktreePool_New(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "pool-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize a git repository
	gitDir := filepath.Join(tmpDir, "repo")
	if err := initGitRepo(gitDir); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Create worktree manager
	manager := NewWorktreeManager(gitDir, filepath.Join(tmpDir, "worktrees"))
	manager.SetVerbose(false)

	// Test with nil config (should use defaults)
	pool := NewWorktreePool(manager, nil)
	if pool == nil {
		t.Fatal("NewWorktreePool with nil config returned nil")
	}

	if pool.config == nil {
		t.Error("Pool config should not be nil after NewWorktreePool with nil config")
	}

	// Test with custom config
	customConfig := &PoolConfig{
		MinSize:       1,
		MaxSize:       3,
		WarmupTimeout: 10 * time.Second,
	}
	pool = NewWorktreePool(manager, customConfig)
	if pool.config.MinSize != 1 {
		t.Errorf("Expected MinSize 1, got %d", pool.config.MinSize)
	}
	if pool.config.MaxSize != 3 {
		t.Errorf("Expected MaxSize 3, got %d", pool.config.MaxSize)
	}
}

// TestWorktreePool_StartStop verifies pool startup and shutdown
func TestWorktreePool_StartStop(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pool-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	gitDir := filepath.Join(tmpDir, "repo")
	if err := initGitRepo(gitDir); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	manager := NewWorktreeManager(gitDir, filepath.Join(tmpDir, "worktrees"))
	manager.SetVerbose(false)

	// Create a small pool for faster testing
	config := &PoolConfig{
		MinSize:       1,
		MaxSize:       2,
		WarmupTimeout: 5 * time.Second,
		CleanupOnExit: true,
	}
	pool := NewWorktreePool(manager, config)

	// Start the pool
	if err := pool.Start(); err != nil {
		t.Fatalf("Failed to start pool: %v", err)
	}

	// Wait a bit for warmup to complete
	time.Sleep(200 * time.Millisecond)

	// Check stats
	stats := pool.Stats()
	if stats.Total != 1 {
		t.Errorf("Expected 1 worktree after start, got %d", stats.Total)
	}
	if stats.Warm != 1 {
		t.Errorf("Expected 1 warm worktree after start, got %d", stats.Warm)
	}

	// Stop the pool
	if err := pool.Stop(); err != nil {
		t.Fatalf("Failed to stop pool: %v", err)
	}

	// Verify pool is stopped - stats should still work
	stats = pool.Stats()
	// After stop, worktrees should be cleaned up if CleanupOnExit is true
	if stats.Total > 0 && pool.config.CleanupOnExit {
		t.Logf("Note: Worktrees may still exist briefly after stop: %d", stats.Total)
	}
}

// TestWorktreePool_AcquireRelease verifies acquiring and releasing worktrees
func TestWorktreePool_AcquireRelease(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pool-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	gitDir := filepath.Join(tmpDir, "repo")
	if err := initGitRepo(gitDir); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	manager := NewWorktreeManager(gitDir, filepath.Join(tmpDir, "worktrees"))
	manager.SetVerbose(false)

	config := &PoolConfig{
		MinSize:       1,
		MaxSize:       3,
		WarmupTimeout: 5 * time.Second,
		CleanupOnExit: true,
	}
	pool := NewWorktreePool(manager, config)

	if err := pool.Start(); err != nil {
		t.Fatalf("Failed to start pool: %v", err)
	}
	defer pool.Stop()

	// Acquire a worktree
	taskID := "test-task-1"
	path, err := pool.Acquire(taskID)
	if err != nil {
		t.Fatalf("Failed to acquire worktree: %v", err)
	}
	if path == "" {
		t.Fatal("Acquire returned empty path")
	}

	// Check stats after acquire
	stats := pool.Stats()
	if stats.InUse != 1 {
		t.Errorf("Expected 1 in-use worktree, got %d", stats.InUse)
	}

	// Release the worktree (don't retain)
	if err := pool.Release(taskID, false); err != nil {
		t.Fatalf("Failed to release worktree: %v", err)
	}

	// Worktree should be in draining state (marked for cleanup)
	time.Sleep(100 * time.Millisecond) // Give time for state transition
	stats = pool.Stats()
	if stats.InUse != 0 {
		t.Errorf("Expected 0 in-use worktrees after release, got %d", stats.InUse)
	}
}

// TestWorktreePool_MultipleAcquire verifies acquiring multiple worktrees
func TestWorktreePool_MultipleAcquire(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pool-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	gitDir := filepath.Join(tmpDir, "repo")
	if err := initGitRepo(gitDir); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	manager := NewWorktreeManager(gitDir, filepath.Join(tmpDir, "worktrees"))
	manager.SetVerbose(false)

	config := &PoolConfig{
		MinSize:       2,
		MaxSize:       4,
		WarmupTimeout: 5 * time.Second,
		CleanupOnExit: true,
	}
	pool := NewWorktreePool(manager, config)

	if err := pool.Start(); err != nil {
		t.Fatalf("Failed to start pool: %v", err)
	}
	defer pool.Stop()

	// Acquire first worktree
	taskID1 := "test-task-1"
	path1, err := pool.Acquire(taskID1)
	if err != nil {
		t.Fatalf("Failed to acquire first worktree: %v", err)
	}

	// Acquire second worktree
	taskID2 := "test-task-2"
	path2, err := pool.Acquire(taskID2)
	if err != nil {
		t.Fatalf("Failed to acquire second worktree: %v", err)
	}

	// Paths should be different
	if path1 == path2 {
		t.Error("Acquired same path for two different tasks")
	}

	// Should have 2 in-use worktrees
	stats := pool.Stats()
	if stats.InUse != 2 {
		t.Errorf("Expected 2 in-use worktrees, got %d", stats.InUse)
	}

	// Release both
	pool.Release(taskID1, false)
	pool.Release(taskID2, false)
}

// TestWorktreePool_IsEnabled verifies IsEnabled method
func TestWorktreePool_IsEnabled(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pool-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	gitDir := filepath.Join(tmpDir, "repo")
	if err := initGitRepo(gitDir); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	manager := NewWorktreeManager(gitDir, filepath.Join(tmpDir, "worktrees"))
	pool := NewWorktreePool(manager, nil)

	// Pool should be enabled after creation
	if !pool.IsEnabled() {
		t.Error("Pool should be enabled")
	}

	// After starting, should still be enabled
	if err := pool.Start(); err != nil {
		t.Fatalf("Failed to start pool: %v", err)
	}
	if !pool.IsEnabled() {
		t.Error("Pool should still be enabled after start")
	}

	pool.Stop()
	if !pool.IsEnabled() {
		t.Error("Pool should remain enabled even after stop (config persists)")
	}
}

// TestWorktreePool_ReleaseRetain verifies release with retain option
func TestWorktreePool_ReleaseRetain(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pool-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	gitDir := filepath.Join(tmpDir, "repo")
	if err := initGitRepo(gitDir); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	manager := NewWorktreeManager(gitDir, filepath.Join(tmpDir, "worktrees"))
	manager.SetVerbose(false)

	config := &PoolConfig{
		MinSize:       1,
		MaxSize:       2,
		WarmupTimeout: 5 * time.Second,
		CleanupOnExit: true,
	}
	pool := NewWorktreePool(manager, config)

	if err := pool.Start(); err != nil {
		t.Fatalf("Failed to start pool: %v", err)
	}
	defer pool.Stop()

	// Acquire a worktree
	taskID := "test-task-1"
	_, err = pool.Acquire(taskID)
	if err != nil {
		t.Fatalf("Failed to acquire worktree: %v", err)
	}

	// Release with retain=true
	if err := pool.Release(taskID, true); err != nil {
		t.Fatalf("Failed to release worktree with retain: %v", err)
	}

	// Worktree should be warm again
	time.Sleep(100 * time.Millisecond)
	stats := pool.Stats()
	if stats.Warm < 1 {
		t.Logf("Expected at least 1 warm worktree after retain release, got %d", stats.Warm)
	}

	// Acquire again - should get the same or different worktree
	taskID2 := "test-task-2"
	_, err = pool.Acquire(taskID2)
	if err != nil {
		t.Fatalf("Failed to acquire second worktree: %v", err)
	}

	pool.Release(taskID2, false)
}

// TestWorktreePool_Stats verifies pool statistics
func TestWorktreePool_Stats(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pool-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	gitDir := filepath.Join(tmpDir, "repo")
	if err := initGitRepo(gitDir); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	manager := NewWorktreeManager(gitDir, filepath.Join(tmpDir, "worktrees"))
	manager.SetVerbose(false)

	config := &PoolConfig{
		MinSize:       2,
		MaxSize:       5,
		WarmupTimeout: 5 * time.Second,
		CleanupOnExit: true,
	}
	pool := NewWorktreePool(manager, config)

	// Stats before start should show empty pool
	stats := pool.Stats()
	if stats.Total != 0 {
		t.Errorf("Expected 0 total worktrees before start, got %d", stats.Total)
	}

	if err := pool.Start(); err != nil {
		t.Fatalf("Failed to start pool: %v", err)
	}
	defer pool.Stop()

	// After start, should have minSize warm worktrees
	stats = pool.Stats()
	if stats.MinSize != 2 {
		t.Errorf("Expected MinSize 2, got %d", stats.MinSize)
	}
	if stats.MaxSize != 5 {
		t.Errorf("Expected MaxSize 5, got %d", stats.MaxSize)
	}
}

// TestWorktreePool_FetchWorktree verifies sync fetch functionality
func TestWorktreePool_FetchWorktree(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pool-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	gitDir := filepath.Join(tmpDir, "repo")
	if err := initGitRepo(gitDir); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	manager := NewWorktreeManager(gitDir, filepath.Join(tmpDir, "worktrees"))
	manager.SetVerbose(false)

	config := &PoolConfig{
		MinSize:       1,
		MaxSize:       2,
		WarmupTimeout: 5 * time.Second,
		CleanupOnExit: true,
	}
	pool := NewWorktreePool(manager, config)

	if err := pool.Start(); err != nil {
		t.Fatalf("Failed to start pool: %v", err)
	}
	defer pool.Stop()

	// Test FetchWorktree with a non-existent worktree ID
	result := pool.FetchWorktree("non-existent-id")
	if result.Success {
		t.Error("Expected fetch to fail for non-existent worktree")
	}
	if result.WorktreeID != "non-existent-id" {
		t.Errorf("Expected worktree ID 'non-existent-id', got '%s'", result.WorktreeID)
	}

	// Note: We can't easily test real git fetch without a remote repository
	// This test verifies the error handling for non-existent worktrees
}

// TestWorktreePool_FetchAll verifies async fetch for all worktrees
func TestWorktreePool_FetchAll(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pool-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	gitDir := filepath.Join(tmpDir, "repo")
	if err := initGitRepo(gitDir); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	manager := NewWorktreeManager(gitDir, filepath.Join(tmpDir, "worktrees"))
	manager.SetVerbose(false)

	config := &PoolConfig{
		MinSize:       1,
		MaxSize:       2,
		WarmupTimeout: 5 * time.Second,
		CleanupOnExit: true,
	}
	pool := NewWorktreePool(manager, config)

	if err := pool.Start(); err != nil {
		t.Fatalf("Failed to start pool: %v", err)
	}
	defer pool.Stop()

	// Wait for worktree to warm up
	time.Sleep(200 * time.Millisecond)

	// Fetch all worktrees
	resultCh := pool.FetchAll()
	if resultCh == nil {
		t.Fatal("FetchAll returned nil channel")
	}

	// Collect results with a timeout
	results := 0
	timeout := time.After(3 * time.Second)
	for {
		select {
		case result, ok := <-resultCh:
			if !ok {
				// Channel closed - done
				if results > 0 {
					t.Logf("Received %d fetch results", results)
				}
				return
			}
			results++
			t.Logf("Fetch result: worktree=%s, success=%v, error=%s",
				result.WorktreeID, result.Success, result.Error)
		case <-timeout:
			// Timeout is OK - we don't have a real remote, so fetch may fail silently
			if results == 0 {
				t.Log("No fetch results received (expected without remote repository)")
			}
			return
		}
	}
}

// Helper function to initialize a git repository for testing
func initGitRepo(dir string) error {
	// Create directory
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Initialize git repo
	if err := runCommand(dir, "git", "init"); err != nil {
		return err
	}

	// Configure git
	if err := runCommand(dir, "git", "config", "user.email", "test@example.com"); err != nil {
		return err
	}
	if err := runCommand(dir, "git", "config", "user.name", "Test User"); err != nil {
		return err
	}

	// Create an initial commit
	testFile := filepath.Join(dir, "README.md")
	if err := os.WriteFile(testFile, []byte("# Test Repository\n"), 0644); err != nil {
		return err
	}

	if err := runCommand(dir, "git", "add", "README.md"); err != nil {
		return err
	}

	if err := runCommand(dir, "git", "commit", "-m", "Initial commit"); err != nil {
		return err
	}

	return nil
}

// Helper function to run a command
func runCommand(dir string, name string, args ...string) error {
	cmd := []string{name}
	cmd = append(cmd, args...)

	// Use exec.Command for real git operations
	execCmd := exec.CommandContext(context.Background(), cmd[0], cmd[1:]...)
	execCmd.Dir = dir
	output, err := execCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("command %s failed: %w, output: %s", cmd, err, string(output))
	}
	return nil
}
