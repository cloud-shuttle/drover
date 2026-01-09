// Package git handles git worktree operations for parallel task execution
package git

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/cloud-shuttle/drover/pkg/types"
)

// Global mutex to serialize MergeToMain operations across all workers
// Multiple workers checking out and merging to main simultaneously causes git index lock conflicts
var mergeMutex sync.Mutex

// WorktreeManager creates and manages git worktrees
type WorktreeManager struct {
	baseDir     string // Base repository directory
	worktreeDir string // Where worktrees are created (.drover/worktrees)
	verbose     bool   // Enable verbose logging
}

// NewWorktreeManager creates a new worktree manager
func NewWorktreeManager(baseDir, worktreeDir string) *WorktreeManager {
	return &WorktreeManager{
		baseDir:     baseDir,
		worktreeDir: worktreeDir,
		verbose:     false,
	}
}

// SetVerbose enables or disables verbose logging
func (wm *WorktreeManager) SetVerbose(v bool) {
	wm.verbose = v
}

// Create creates a new worktree for a task
func (wm *WorktreeManager) Create(task *types.Task) (string, error) {
	worktreePath := filepath.Join(wm.worktreeDir, task.ID)

	// Ensure worktree directory exists
	if err := os.MkdirAll(wm.worktreeDir, 0755); err != nil {
		return "", fmt.Errorf("creating worktree directory: %w", err)
	}

	// Clean up any existing worktree at this path first
	// This handles stale worktrees from interrupted runs
	wm.cleanUpWorktree(task.ID)

	// Create the worktree
	cmd := exec.Command("git", "worktree", "add", worktreePath)
	cmd.Dir = wm.baseDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("creating worktree: %w\n%s", err, output)
	}

	return worktreePath, nil
}

// cleanUpWorktree removes any existing worktree registration and directory for a task
func (wm *WorktreeManager) cleanUpWorktree(taskID string) {
	worktreePath := filepath.Join(wm.worktreeDir, taskID)

	// Step 1: Try to remove the worktree via git (handles registered worktrees)
	cmd := exec.Command("git", "worktree", "remove", "--force", worktreePath)
	cmd.Dir = wm.baseDir
	_ = cmd.Run() // Ignore errors

	// Step 2: If directory still exists, remove it manually (handles unregistered directories)
	if _, err := os.Stat(worktreePath); err == nil {
		_ = os.RemoveAll(worktreePath)
	}

	// Step 3: Prune all stale worktree registrations globally
	cmd = exec.Command("git", "worktree", "prune")
	cmd.Dir = wm.baseDir
	_ = cmd.Run() // Ignore errors
}

// PruneStale removes stale git worktree registrations for a specific task
func (wm *WorktreeManager) PruneStale(taskID string) {
	worktreePath := filepath.Join(wm.worktreeDir, taskID)

	// First, try force remove if the worktree is registered but directory is missing
	cmd := exec.Command("git", "worktree", "remove", "--force", worktreePath)
	cmd.Dir = wm.baseDir
	_ = cmd.Run() // Ignore errors

	// Then prune all stale worktree registrations (globally, not per-worktree)
	cmd = exec.Command("git", "worktree", "prune")
	cmd.Dir = wm.baseDir
	_ = cmd.Run() // Ignore errors, this is best-effort cleanup
}

// Remove removes a worktree
func (wm *WorktreeManager) Remove(taskID string) error {
	worktreePath := filepath.Join(wm.worktreeDir, taskID)

	// Remove the worktree
	cmd := exec.Command("git", "worktree", "remove", worktreePath)
	cmd.Dir = wm.baseDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		// If worktree doesn't exist, that's okay
		outputStr := string(output)
		if strings.Contains(outputStr, "Not a worktree") ||
			strings.Contains(outputStr, "no such file or directory") ||
			strings.Contains(outputStr, "is not a working tree") {
			return nil
		}
		return fmt.Errorf("removing worktree: %w\n%s", err, output)
	}

	return nil
}

// Commit commits all changes in the worktree
// Returns (hasChanges, error) - hasChanges is true if changes were committed
func (wm *WorktreeManager) Commit(taskID, message string) (bool, error) {
	worktreePath := filepath.Join(wm.worktreeDir, taskID)

	// Check if there are any changes to commit
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = worktreePath
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("checking status: %w", err)
	}

	// If no changes, return success without committing
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		if wm.verbose {
			log.Printf("📭 No changes detected in worktree %s", taskID)
		}
		return false, nil // Nothing to commit
	}

	// Log what files changed (verbose only)
	if wm.verbose {
		lines := strings.Split(trimmed, "\n")
		log.Printf("📝 Changes detected in %d files for task %s:", len(lines), taskID)
		for _, line := range lines {
			if line != "" {
				log.Printf("   %s", line)
			}
		}
	}

	// Stage all changes
	cmd = exec.Command("git", "add", "-A")
	cmd.Dir = worktreePath
	if output, err := cmd.CombinedOutput(); err != nil {
		return false, fmt.Errorf("staging changes: %w\n%s", err, output)
	}

	// Commit
	cmd = exec.Command("git", "commit", "-m", message)
	cmd.Dir = worktreePath
	if output, err := cmd.CombinedOutput(); err != nil {
		// If git commit says "nothing to commit", treat it as success
		// This can happen if the working tree changes between the check and the commit
		outputStr := string(output)
		if strings.Contains(outputStr, "nothing to commit") {
			if wm.verbose {
				log.Printf("📭 No changes to commit (working tree clean)")
			}
			return false, nil // No problem, just no changes to commit
		}
		return false, fmt.Errorf("committing: %w\n%s", err, output)
	}

	if wm.verbose {
		log.Printf("✅ Committed changes for task %s", taskID)
	}

	return true, nil
}

// MergeToMain merges the worktree changes to main branch
func (wm *WorktreeManager) MergeToMain(taskID string) error {
	// Serialize merge operations to prevent git index lock conflicts
	mergeMutex.Lock()
	defer mergeMutex.Unlock()

	worktreePath := filepath.Join(wm.worktreeDir, taskID)
	branchName := fmt.Sprintf("drover-%s", taskID)

	// Check if worktree has any commits ahead of main
	// Run this from the base directory to ensure we have the main branch reference
	cmd := exec.Command("git", "rev-list", "main.."+branchName, "--count")
	cmd.Dir = wm.baseDir
	output, err := cmd.Output()
	if err != nil {
		// Branch doesn't exist yet, we'll create it and check if there's anything to merge
		output = []byte("1") // Assume there's something to merge
	}

	// If the count is 0, no commits to merge
	if strings.TrimSpace(string(output)) == "0" {
		return nil
	}

	// Create a branch from the worktree
	cmd = exec.Command("git", "checkout", "-b", branchName)
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

// Directories to clean up aggressively (build artifacts and dependencies)
// These can consume massive amounts of disk space
var aggressiveCleanupDirs = []string{
	"target",           // Rust/Cargo build artifacts
	"node_modules",     // Node.js dependencies
	"vendor",           // PHP/Go vendor directories
	"__pycache__",      // Python cache
	".venv",            // Python virtual environments
	"venv",             // Python virtual environments
	"dist",             // Various build outputs
	"build",            // Various build outputs
	".next",            // Next.js cache
	".nuxt",            // Nuxt.js cache
	"coverage",         // Code coverage reports
}

// RemoveAggressive removes a worktree and aggressively cleans up build artifacts
// This is useful after a task is completed to free up disk space
func (wm *WorktreeManager) RemoveAggressive(taskID string) (int64, error) {
	worktreePath := filepath.Join(wm.worktreeDir, taskID)

	// First, clean up large build directories within the worktree
	var sizeFreed int64

	// Remove aggressive cleanup directories
	for _, dirName := range aggressiveCleanupDirs {
		dirPath := filepath.Join(worktreePath, dirName)
		if size, err := wm.getDirectorySize(dirPath); err == nil && size > 0 {
			if err := os.RemoveAll(dirPath); err == nil {
				sizeFreed += size
				if wm.verbose {
					log.Printf("🗑️  Removed %s: freed %s", dirName, formatBytes(size))
				}
			}
		}
	}

	// Clean up nested node_modules (can exist in subdirectories)
	if err := wm.removeAllNestedDirectories(worktreePath, "node_modules"); err == nil {
		if wm.verbose {
			log.Printf("🗑️  Removed nested node_modules directories")
		}
	}

	// Clean up nested target directories (Cargo workspaces)
	if err := wm.removeAllNestedDirectories(worktreePath, "target"); err == nil {
		if wm.verbose {
			log.Printf("🗑️  Removed nested target directories")
		}
	}

	// Remove git worktree registration
	cmd := exec.Command("git", "worktree", "remove", "--force", worktreePath)
	cmd.Dir = wm.baseDir
	_, _ = cmd.CombinedOutput() // Ignore errors

	// If directory still exists, remove it manually
	if _, err := os.Stat(worktreePath); err == nil {
		if err := os.RemoveAll(worktreePath); err == nil {
			if wm.verbose {
				log.Printf("🗑️  Removed worktree directory: %s", worktreePath)
			}
		}
	}

	// Prune stale git worktree registrations
	cmd = exec.Command("git", "worktree", "prune")
	cmd.Dir = wm.baseDir
	_ = cmd.Run() // Ignore errors

	return sizeFreed, nil
}

// removeAllNestedDirectories removes all directories with a given name recursively
func (wm *WorktreeManager) removeAllNestedDirectories(root, dirName string) error {
	var dirsToRemove []string

	// First pass: collect all directories to remove
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Continue on error
		}
		if d.IsDir() && d.Name() == dirName && path != filepath.Join(root, dirName) {
			// Skip the root level directory (will be handled by aggressiveCleanupDirs)
			// but collect nested ones
			dirsToRemove = append(dirsToRemove, path)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Second pass: remove collected directories
	for _, dir := range dirsToRemove {
		os.RemoveAll(dir)
	}

	return nil
}

// getDirectorySize returns the size of a directory in bytes
func (wm *WorktreeManager) getDirectorySize(path string) (int64, error) {
	var size int64

	err := filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Continue on error
		}
		if !d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return nil
			}
			size += info.Size()
		}
		return nil
	})

	return size, err
}

// GetDiskUsage returns the disk usage of a specific worktree
func (wm *WorktreeManager) GetDiskUsage(taskID string) (int64, error) {
	worktreePath := filepath.Join(wm.worktreeDir, taskID)
	return wm.getDirectorySize(worktreePath)
}

// ListWorktreesOnDisk returns all worktree directories that exist on disk
func (wm *WorktreeManager) ListWorktreesOnDisk() ([]string, error) {
	entries, err := os.ReadDir(wm.worktreeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("reading worktree directory: %w", err)
	}

	var worktrees []string
	for _, entry := range entries {
		if entry.IsDir() {
			worktrees = append(worktrees, entry.Name())
		}
	}

	return worktrees, nil
}

// PruneOrphaned removes all worktrees that exist on disk but are not registered with git
func (wm *WorktreeManager) PruneOrphaned() ([]string, int64, error) {
	// Get all git-registered worktrees
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = wm.baseDir
	output, err := cmd.Output()
	if err != nil {
		return nil, 0, fmt.Errorf("listing worktrees: %w", err)
	}

	// Parse registered worktree paths
	registeredPaths := make(map[string]bool)
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "worktree ") {
			path := strings.TrimPrefix(line, "worktree ")
			registeredPaths[path] = true
		}
	}

	// Get all directories on disk
	onDisk, err := wm.ListWorktreesOnDisk()
	if err != nil {
		return nil, 0, err
	}

	var pruned []string
	var totalFreed int64

	for _, taskID := range onDisk {
		worktreePath := filepath.Join(wm.worktreeDir, taskID)
		// If not registered, it's orphaned
		if !registeredPaths[worktreePath] {
			size, _ := wm.GetDiskUsage(taskID)
			if err := os.RemoveAll(worktreePath); err == nil {
				pruned = append(pruned, taskID)
				totalFreed += size
				if wm.verbose {
					log.Printf("🗑️  Pruned orphaned worktree: %s (freed %s)", taskID, formatBytes(size))
				}
			}
		}
	}

	// Also run git worktree prune to clean up registrations
	cmd = exec.Command("git", "worktree", "prune")
	cmd.Dir = wm.baseDir
	_ = cmd.Run()

	return pruned, totalFreed, nil
}

// CleanupAll removes all worktrees and returns total space freed
func (wm *WorktreeManager) CleanupAll() (count int, totalFreed int64, err error) {
	// First get all worktrees and their sizes
	onDisk, err := wm.ListWorktreesOnDisk()
	if err != nil {
		return 0, 0, err
	}

	for _, taskID := range onDisk {
		size, err := wm.RemoveAggressive(taskID)
		if err != nil {
			if wm.verbose {
				log.Printf("⚠️  Failed to remove worktree %s: %v", taskID, err)
			}
			continue
		}
		count++
		totalFreed += size
	}

	return count, totalFreed, nil
}

// formatBytes converts bytes to a human-readable string
func formatBytes(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	unitIndex := 0
	value := float64(bytes)

	for value >= 1024 && unitIndex < len(units)-1 {
		value /= 1024
		unitIndex++
	}

	return fmt.Sprintf("%.1f %s", value, units[unitIndex])
}

// GetBuildArtifactSizes returns the sizes of common build artifact directories
func (wm *WorktreeManager) GetBuildArtifactSizes(taskID string) (map[string]int64, error) {
	worktreePath := filepath.Join(wm.worktreeDir, taskID)
	sizes := make(map[string]int64)

	for _, dirName := range aggressiveCleanupDirs {
		dirPath := filepath.Join(worktreePath, dirName)
		if size, err := wm.getDirectorySize(dirPath); err == nil {
			sizes[dirName] = size
		}
	}

	return sizes, nil
}
