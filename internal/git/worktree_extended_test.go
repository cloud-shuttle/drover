package git_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cloud-shuttle/drover/internal/git"
	"github.com/cloud-shuttle/drover/pkg/types"
)

// ============================================================================
// PruneStale – cleans up leftover worktree registrations
// ============================================================================

func TestWorktreeManager_PruneStale(t *testing.T) {
	_, wm := setupTestRepo(t)

	task := &types.Task{ID: "task-prune-stale", Title: "Prune Stale Test"}

	// Create and then manually delete the directory (simulating a crash)
	worktreePath, err := wm.Create(task)
	if err != nil {
		t.Fatalf("Failed to create worktree: %v", err)
	}

	// Manually delete the directory to leave a stale registration
	os.RemoveAll(worktreePath)

	// PruneStale should clean up without error
	wm.PruneStale(task.ID)

	// Should be able to recreate the worktree now (no stale conflict)
	newPath, err := wm.Create(task)
	if err != nil {
		t.Fatalf("Failed to recreate worktree after prune: %v", err)
	}
	defer wm.Remove(task.ID)

	if _, err := os.Stat(newPath); os.IsNotExist(err) {
		t.Error("Recreated worktree does not exist")
	}
}

func TestWorktreeManager_PruneStale_NonExistent(t *testing.T) {
	_, wm := setupTestRepo(t)

	// Pruning a non-existent worktree should not panic
	wm.PruneStale("nonexistent-task")
}

// ============================================================================
// ListWorktreesOnDisk
// ============================================================================

func TestWorktreeManager_ListWorktreesOnDisk(t *testing.T) {
	_, wm := setupTestRepo(t)

	tasks := []*types.Task{
		{ID: "task-disk-1", Title: "T1"},
		{ID: "task-disk-2", Title: "T2"},
	}

	for _, task := range tasks {
		_, err := wm.Create(task)
		if err != nil {
			t.Fatalf("Failed to create worktree %s: %v", task.ID, err)
		}
	}
	defer func() {
		for _, task := range tasks {
			wm.Remove(task.ID)
		}
	}()

	onDisk, err := wm.ListWorktreesOnDisk()
	if err != nil {
		t.Fatalf("ListWorktreesOnDisk: %v", err)
	}

	if len(onDisk) != 2 {
		t.Errorf("expected 2 worktrees on disk, got %d", len(onDisk))
	}
}

func TestWorktreeManager_ListWorktreesOnDisk_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	// Point to a non-existent worktree directory
	wm := git.NewWorktreeManager(tmpDir, filepath.Join(tmpDir, ".drover", "nonexistent"))

	onDisk, err := wm.ListWorktreesOnDisk()
	if err != nil {
		t.Fatalf("expected no error for non-existent dir, got: %v", err)
	}
	if len(onDisk) != 0 {
		t.Errorf("expected empty list, got %d", len(onDisk))
	}
}

// ============================================================================
// GetDiskUsage
// ============================================================================

func TestWorktreeManager_GetDiskUsage(t *testing.T) {
	_, wm := setupTestRepo(t)

	task := &types.Task{ID: "task-usage", Title: "Usage Test"}
	worktreePath, err := wm.Create(task)
	if err != nil {
		t.Fatalf("Failed to create worktree: %v", err)
	}
	defer wm.Remove(task.ID)

	// Write a file with known content
	testFile := filepath.Join(worktreePath, "large.txt")
	data := make([]byte, 1024) // 1KB
	for i := range data {
		data[i] = 'x'
	}
	os.WriteFile(testFile, data, 0644)

	size, err := wm.GetDiskUsage(task.ID)
	if err != nil {
		t.Fatalf("GetDiskUsage: %v", err)
	}

	// Size should be at least 1KB (our file) plus the README.md from the base repo
	if size < 1024 {
		t.Errorf("expected at least 1024 bytes, got %d", size)
	}
}

func TestWorktreeManager_GetDiskUsage_NonExistent(t *testing.T) {
	_, wm := setupTestRepo(t)

	// Non-existent worktree should return 0
	size, _ := wm.GetDiskUsage("nonexistent-task")
	if size != 0 {
		t.Errorf("expected 0 bytes for non-existent worktree, got %d", size)
	}
}

// ============================================================================
// GetBuildArtifactSizes
// ============================================================================

func TestWorktreeManager_GetBuildArtifactSizes(t *testing.T) {
	_, wm := setupTestRepo(t)

	task := &types.Task{ID: "task-artifacts", Title: "Artifacts Test"}
	worktreePath, err := wm.Create(task)
	if err != nil {
		t.Fatalf("Failed to create worktree: %v", err)
	}
	defer wm.Remove(task.ID)

	// Create a fake node_modules directory with a file
	nmDir := filepath.Join(worktreePath, "node_modules")
	os.MkdirAll(nmDir, 0755)
	os.WriteFile(filepath.Join(nmDir, "package.json"), []byte(`{"name":"test"}`), 0644)

	// Create a fake dist directory
	distDir := filepath.Join(worktreePath, "dist")
	os.MkdirAll(distDir, 0755)
	os.WriteFile(filepath.Join(distDir, "bundle.js"), []byte("console.log('hello')"), 0644)

	sizes, err := wm.GetBuildArtifactSizes(task.ID)
	if err != nil {
		t.Fatalf("GetBuildArtifactSizes: %v", err)
	}

	if sizes["node_modules"] <= 0 {
		t.Errorf("expected node_modules size > 0, got %d", sizes["node_modules"])
	}
	if sizes["dist"] <= 0 {
		t.Errorf("expected dist size > 0, got %d", sizes["dist"])
	}
}

// ============================================================================
// RemoveAggressive
// ============================================================================

func TestWorktreeManager_RemoveAggressive(t *testing.T) {
	_, wm := setupTestRepo(t)

	task := &types.Task{ID: "task-aggressive", Title: "Aggressive Test"}
	worktreePath, err := wm.Create(task)
	if err != nil {
		t.Fatalf("Failed to create worktree: %v", err)
	}

	// Create build artifacts
	nmDir := filepath.Join(worktreePath, "node_modules")
	os.MkdirAll(nmDir, 0755)
	os.WriteFile(filepath.Join(nmDir, "big-dep.js"), make([]byte, 512), 0644)

	targetDir := filepath.Join(worktreePath, "target")
	os.MkdirAll(targetDir, 0755)
	os.WriteFile(filepath.Join(targetDir, "binary"), make([]byte, 256), 0644)

	sizeFreed, err := wm.RemoveAggressive(task.ID)
	if err != nil {
		t.Fatalf("RemoveAggressive: %v", err)
	}

	// Should have freed the artifact sizes
	if sizeFreed < 768 { // 512 + 256
		t.Errorf("expected at least 768 bytes freed, got %d", sizeFreed)
	}

	// Worktree directory should be gone
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Error("Worktree directory should be removed after RemoveAggressive")
	}
}

// ============================================================================
// RemoveByPath
// ============================================================================

func TestWorktreeManager_RemoveByPath(t *testing.T) {
	_, wm := setupTestRepo(t)

	task := &types.Task{ID: "task-remove-path", Title: "Remove By Path Test"}
	worktreePath, err := wm.Create(task)
	if err != nil {
		t.Fatalf("Failed to create worktree: %v", err)
	}

	// Verify it exists
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		t.Fatal("Worktree was not created")
	}

	// Remove by path
	err = wm.RemoveByPath(worktreePath)
	if err != nil {
		t.Fatalf("RemoveByPath: %v", err)
	}

	// Verify it's gone
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Error("Worktree directory still exists after RemoveByPath")
	}
}

// ============================================================================
// RemoveAggressiveByPath
// ============================================================================

func TestWorktreeManager_RemoveAggressiveByPath(t *testing.T) {
	_, wm := setupTestRepo(t)

	task := &types.Task{ID: "task-agg-path", Title: "Aggressive By Path Test"}
	worktreePath, err := wm.Create(task)
	if err != nil {
		t.Fatalf("Failed to create worktree: %v", err)
	}

	// Create build artifacts
	nmDir := filepath.Join(worktreePath, "node_modules")
	os.MkdirAll(nmDir, 0755)
	os.WriteFile(filepath.Join(nmDir, "dep.js"), make([]byte, 100), 0644)

	// Create a nested node_modules too
	nestedNm := filepath.Join(worktreePath, "packages", "core", "node_modules")
	os.MkdirAll(nestedNm, 0755)
	os.WriteFile(filepath.Join(nestedNm, "nested-dep.js"), make([]byte, 50), 0644)

	sizeFreed, err := wm.RemoveAggressiveByPath(worktreePath)
	if err != nil {
		t.Fatalf("RemoveAggressiveByPath: %v", err)
	}

	if sizeFreed < 100 {
		t.Errorf("expected at least 100 bytes freed, got %d", sizeFreed)
	}

	// Worktree directory should be gone
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Error("Worktree directory should be removed after RemoveAggressiveByPath")
	}
}

// ============================================================================
// ListOrphaned / PruneOrphaned
// ============================================================================

func TestWorktreeManager_ListOrphaned(t *testing.T) {
	baseDir, wm := setupTestRepo(t)

	// Create an orphaned directory (not registered with git)
	worktreeDir := filepath.Join(baseDir, ".drover", "worktrees")
	os.MkdirAll(worktreeDir, 0755)
	orphanPath := filepath.Join(worktreeDir, "orphaned-task")
	os.MkdirAll(orphanPath, 0755)
	os.WriteFile(filepath.Join(orphanPath, "file.txt"), []byte("data"), 0644)

	orphaned, err := wm.ListOrphaned()
	if err != nil {
		t.Fatalf("ListOrphaned: %v", err)
	}

	// The orphaned list should contain "orphaned-task"
	foundOrphan := false
	for _, id := range orphaned {
		if id == "orphaned-task" {
			foundOrphan = true
		}
	}
	if !foundOrphan {
		t.Errorf("expected 'orphaned-task' in orphaned list, got %v", orphaned)
	}
}

func TestWorktreeManager_PruneOrphaned(t *testing.T) {
	baseDir, wm := setupTestRepo(t)

	// Create an orphaned directory
	worktreeDir := filepath.Join(baseDir, ".drover", "worktrees")
	os.MkdirAll(worktreeDir, 0755)
	orphanPath := filepath.Join(worktreeDir, "orphan-1")
	os.MkdirAll(orphanPath, 0755)
	os.WriteFile(filepath.Join(orphanPath, "file.txt"), []byte("data"), 0644)

	pruned, totalFreed, err := wm.PruneOrphaned()
	if err != nil {
		t.Fatalf("PruneOrphaned: %v", err)
	}

	if len(pruned) != 1 {
		t.Errorf("expected 1 pruned, got %d", len(pruned))
	}
	if totalFreed <= 0 {
		t.Errorf("expected some bytes freed, got %d", totalFreed)
	}

	// Orphan should be gone
	if _, err := os.Stat(orphanPath); !os.IsNotExist(err) {
		t.Error("Orphaned directory still exists after prune")
	}
}

// ============================================================================
// CleanupAll
// ============================================================================

func TestWorktreeManager_CleanupAll(t *testing.T) {
	_, wm := setupTestRepo(t)

	tasks := []*types.Task{
		{ID: "task-all-1", Title: "T1"},
		{ID: "task-all-2", Title: "T2"},
		{ID: "task-all-3", Title: "T3"},
	}

	for _, task := range tasks {
		worktreePath, err := wm.Create(task)
		if err != nil {
			t.Fatalf("Failed to create worktree %s: %v", task.ID, err)
		}
		// Add some build artifacts to verify size tracking
		nmDir := filepath.Join(worktreePath, "node_modules")
		os.MkdirAll(nmDir, 0755)
		os.WriteFile(filepath.Join(nmDir, "dep.js"), make([]byte, 100), 0644)
	}

	count, totalFreed, err := wm.CleanupAll()
	if err != nil {
		t.Fatalf("CleanupAll: %v", err)
	}

	if count != 3 {
		t.Errorf("expected 3 removed, got %d", count)
	}
	if totalFreed < 300 { // 100 bytes per worktree * 3
		t.Errorf("expected at least 300 bytes freed, got %d", totalFreed)
	}

	// All worktrees should be gone
	onDisk, _ := wm.ListWorktreesOnDisk()
	if len(onDisk) != 0 {
		t.Errorf("expected 0 worktrees after CleanupAll, got %d", len(onDisk))
	}
}

// ============================================================================
// Commit – edge cases
// ============================================================================

func TestWorktreeManager_Commit_MultipleFiles(t *testing.T) {
	_, wm := setupTestRepo(t)

	task := &types.Task{ID: "task-multi-commit", Title: "Multi Commit"}
	worktreePath, err := wm.Create(task)
	if err != nil {
		t.Fatalf("Failed to create worktree: %v", err)
	}
	defer wm.Remove(task.ID)

	// Create multiple files in subdirectories
	subDir := filepath.Join(worktreePath, "src", "components")
	os.MkdirAll(subDir, 0755)
	os.WriteFile(filepath.Join(subDir, "button.go"), []byte("package components\n"), 0644)
	os.WriteFile(filepath.Join(subDir, "input.go"), []byte("package components\n"), 0644)
	os.WriteFile(filepath.Join(worktreePath, "main.go"), []byte("package main\n"), 0644)

	hasChanges, err := wm.Commit(task.ID, "add multiple files")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if !hasChanges {
		t.Error("expected hasChanges=true for multiple new files")
	}
}

func TestWorktreeManager_Commit_DeletedFiles(t *testing.T) {
	_, wm := setupTestRepo(t)

	task := &types.Task{ID: "task-delete-commit", Title: "Delete Commit"}
	worktreePath, err := wm.Create(task)
	if err != nil {
		t.Fatalf("Failed to create worktree: %v", err)
	}
	defer wm.Remove(task.ID)

	// Delete a tracked file (README.md was from initial commit)
	os.Remove(filepath.Join(worktreePath, "README.md"))

	hasChanges, err := wm.Commit(task.ID, "delete README")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if !hasChanges {
		t.Error("expected hasChanges=true for deleted file")
	}
}

// ============================================================================
// MergeToMain – edge cases
// ============================================================================

func TestWorktreeManager_MergeToMain_NonExistentBranch(t *testing.T) {
	_, wm := setupTestRepo(t)

	// Merge a task that was never created – should succeed silently
	err := wm.MergeToMain("nonexistent-task")
	if err != nil {
		t.Errorf("MergeToMain for non-existent branch should succeed, got: %v", err)
	}
}

// ============================================================================
// Create – branch collision recovery
// ============================================================================

func TestWorktreeManager_Create_RecreateAfterStaleBranch(t *testing.T) {
	_, wm := setupTestRepo(t)

	task := &types.Task{ID: "task-stale-branch", Title: "Stale Branch"}

	// Create, then remove, then recreate (simulates crash recovery)
	path1, err := wm.Create(task)
	if err != nil {
		t.Fatalf("First create: %v", err)
	}
	wm.Remove(task.ID)

	// Recreate – Create should handle stale branch/registration
	path2, err := wm.Create(task)
	if err != nil {
		t.Fatalf("Second create after remove: %v", err)
	}
	defer wm.Remove(task.ID)

	if _, err := os.Stat(path2); os.IsNotExist(err) {
		t.Error("Recreated worktree does not exist")
	}

	// Paths should be identical
	if path1 != path2 {
		t.Errorf("expected same path, got %s and %s", path1, path2)
	}
}
