// Package git handles git worktree operations for parallel task execution
package git

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/cloud-shuttle/drover/pkg/telemetry"
	"github.com/cloud-shuttle/drover/pkg/types"
	"go.opentelemetry.io/otel/attribute"
)

// Global mutex to serialize MergeToMain operations across all workers
// Multiple workers checking out and merging to main simultaneously causes git index lock conflicts
var mergeMutex sync.Mutex

// WorktreeManager creates and manages git worktrees
type WorktreeManager struct {
	baseDir           string // Base repository directory
	worktreeDir       string // Where worktrees are created (.drover/worktrees)
	verbose           bool   // Enable verbose logging
	mergeTargetBranch string // Branch to merge changes to (default: "main")
}

// NewWorktreeManager creates a new worktree manager
func NewWorktreeManager(baseDir, worktreeDir string) *WorktreeManager {
	return &WorktreeManager{
		baseDir:           baseDir,
		worktreeDir:       worktreeDir,
		verbose:           false,
		mergeTargetBranch: "main",
	}
}

// SetMergeTargetBranch sets the branch to merge changes to
func (wm *WorktreeManager) SetMergeTargetBranch(branch string) {
	wm.mergeTargetBranch = branch
}

// GetMergeTargetBranch returns the current merge target branch
func (wm *WorktreeManager) GetMergeTargetBranch() string {
	if wm.mergeTargetBranch == "" {
		return "main"
	}
	return wm.mergeTargetBranch
}

// EnsureMainBranch ensures the repository has a main branch
// For empty repos, this creates an orphan main branch with an initial commit
func (wm *WorktreeManager) EnsureMainBranch(ctx context.Context) error {
	ctx, span := telemetry.StartGitSpan(ctx, telemetry.SpanGitEnsureMain)
	defer span.End()

	targetBranch := wm.GetMergeTargetBranch()
	span.SetAttributes(attribute.String("git.branch", targetBranch))

	// Check if target branch exists
	cmd := exec.Command("git", "rev-parse", "--verify", targetBranch)
	cmd.Dir = wm.baseDir
	err := cmd.Run()

	if err == nil {
		// Target branch already exists
		return nil
	}

	// No target branch exists, create it from orphan
	log.Printf("🌱 Creating %s branch in empty repo", targetBranch)

	cmd = exec.Command("git", "checkout", "--orphan", targetBranch)
	cmd.Dir = wm.baseDir
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create %s branch: %w\n%s", targetBranch, err, output)
	}

	// Create empty commit to establish branch
	cmd = exec.Command("git", "commit", "--allow-empty", "-m", "Initial commit")
	cmd.Dir = wm.baseDir
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create initial commit: %w\n%s", err, output)
	}

	log.Printf("✅ Created %s branch with initial commit", targetBranch)

	return nil
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

	// Check if repository has any commits BEFORE ensuring main branch
	// This determines if we need to handle empty repo specially
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = wm.baseDir
	_, err := cmd.Output()
	hadCommits := err == nil

	// Ensure main branch exists (handles empty repos)
	if err := wm.EnsureMainBranch(context.Background()); err != nil {
		return "", fmt.Errorf("ensuring main branch: %w", err)
	}

	var output []byte
	if hadCommits {
		// Repository had commits before, create worktree from HEAD
		cmd = exec.Command("git", "worktree", "add", worktreePath)
		cmd.Dir = wm.baseDir
		output, err = cmd.CombinedOutput()
	} else {
		// Repository had no commits, EnsureMainBranch created main with initial commit
		// Since main is already checked out in base directory, create orphan worktree
		// The worktree will be on an orphan branch (not 'main') which is expected for empty repos
		cmd = exec.Command("git", "worktree", "add", "--orphan", worktreePath)
		cmd.Dir = wm.baseDir
		output, err = cmd.CombinedOutput()
	}

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
func (wm *WorktreeManager) MergeToMain(ctx context.Context, taskID string) error {
	ctx, span := telemetry.StartGitSpan(ctx, telemetry.SpanGitWorktreeMerge, attribute.String("task.id", taskID))
	defer span.End()

	// Serialize merge operations to prevent git index lock conflicts
	mergeMutex.Lock()
	defer mergeMutex.Unlock()

	// Check if repository was empty before EnsureMainBranch potentially creates a commit
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = wm.baseDir
	_, err := cmd.Output()
	hadCommits := err == nil
	span.SetAttributes(attribute.Bool("repo.had_commits", hadCommits))

	// Ensure target branch exists (safety check for empty repos)
	if err := wm.EnsureMainBranch(context.TODO()); err != nil {
		return fmt.Errorf("ensuring target branch: %w", err)
	}

	worktreePath := filepath.Join(wm.worktreeDir, taskID)
	branchName := fmt.Sprintf("drover-%s", taskID)

	// If repo had no commits before EnsureMainBranch, we need special handling
	// The worktree's commit becomes the initial commit for main
	if !hadCommits {
		// Stage all changes
		cmd = exec.Command("git", "add", "-A")
		cmd.Dir = worktreePath
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("staging changes: %w\n%s", err, output)
		}

		// Create initial commit if needed
		cmd = exec.Command("git", "commit", "-m", fmt.Sprintf("drover: Initial commit from %s", taskID))
		cmd.Dir = worktreePath
		if output, err := cmd.CombinedOutput(); err != nil {
			if !strings.Contains(string(output), "nothing to commit") {
				return fmt.Errorf("creating initial commit: %w\n%s", err, output)
			}
		}

		// Get the commit hash (trim trailing newline)
		cmd = exec.Command("git", "rev-parse", "HEAD")
		cmd.Dir = worktreePath
		commitHashBytes, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("getting commit hash: %w", err)
		}
		commitHash := strings.TrimSpace(string(commitHashBytes))

		// Update main branch to point to the worktree's commit
		targetBranch := wm.GetMergeTargetBranch()
		cmd = exec.Command("git", "checkout", "-B", targetBranch)
		cmd.Dir = wm.baseDir
		output, err := cmd.CombinedOutput()
		if err != nil && !strings.Contains(string(output), "fatal: invalid reference") &&
			!strings.Contains(string(output), "fatal: you must specify a branch name") {
			return fmt.Errorf("creating %s branch: %w\n%s", targetBranch, err, output)
		}

		cmd = exec.Command("git", "reset", "--hard", commitHash)
		cmd.Dir = wm.baseDir
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("resetting main to commit: %w\n%s", err, output)
		}

		return nil
	}

	// Repository has commits, proceed with standard merge
	// Check if worktree has any commits ahead of main
	cmd = exec.Command("git", "rev-list", "main.."+branchName, "--count")
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

	// Delete the branch if it already exists from a previous failed run
	cmd = exec.Command("git", "branch", "-D", branchName)
	cmd.Dir = wm.baseDir
	_, _ = cmd.CombinedOutput() // Ignore errors - branch may not exist

	// Create a branch from the worktree
	cmd = exec.Command("git", "checkout", "-b", branchName)
	cmd.Dir = worktreePath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("creating branch: %w\n%s", err, output)
	}

	// Check if branches are unrelated (common in empty repos where worktree was created before main had commits)
	// Try the merge and check for "unrelated histories" error
	targetBranch := wm.GetMergeTargetBranch()
	cmd = exec.Command("git", "checkout", targetBranch)
	cmd.Dir = wm.baseDir
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("checking out %s: %w\n%s", targetBranch, err, output)
	}

	cmd = exec.Command("git", "merge", "--no-ff", branchName, "-m", fmt.Sprintf("drover: Merge %s", taskID))
	cmd.Dir = wm.baseDir
	output, err = cmd.CombinedOutput()
	if err != nil {
		outputStr := string(output)
		// Check for unrelated histories error
		if strings.Contains(outputStr, "refusing to merge unrelated histories") {
			// Branches are unrelated - this happens when worktree was created before main had commits
			// Use reset --hard to adopt the worktree's commit as main's commit

			// Get the worktree's commit hash
			cmd = exec.Command("git", "rev-parse", "HEAD")
			cmd.Dir = worktreePath
			commitHashBytes, err := cmd.Output()
			if err != nil {
				return fmt.Errorf("getting worktree commit hash: %w", err)
			}
			commitHash := strings.TrimSpace(string(commitHashBytes))

			// Reset main to the worktree's commit
			cmd = exec.Command("git", "reset", "--hard", commitHash)
			cmd.Dir = wm.baseDir
			if output, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("resetting main to worktree commit: %w\n%s", err, output)
			}

			return nil
		}
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
	"target",       // Rust/Cargo build artifacts
	"node_modules", // Node.js dependencies
	"vendor",       // PHP/Go vendor directories
	"__pycache__",  // Python cache
	".venv",        // Python virtual environments
	"venv",         // Python virtual environments
	"dist",         // Various build outputs
	"build",        // Various build outputs
	".next",        // Next.js cache
	".nuxt",        // Nuxt.js cache
	"coverage",     // Code coverage reports
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
	orphaned, _ := wm.ListOrphaned()

	var pruned []string
	var totalFreed int64

	for _, taskID := range orphaned {
		worktreePath := filepath.Join(wm.worktreeDir, taskID)
		size, _ := wm.GetDiskUsage(taskID)
		if err := os.RemoveAll(worktreePath); err == nil {
			pruned = append(pruned, taskID)
			totalFreed += size
			if wm.verbose {
				log.Printf("🗑️  Pruned orphaned worktree: %s (freed %s)", taskID, formatBytes(size))
			}
		}
	}

	// Also run git worktree prune to clean up registrations
	cmd := exec.Command("git", "worktree", "prune")
	cmd.Dir = wm.baseDir
	_ = cmd.Run()

	return pruned, totalFreed, nil
}

// ListOrphaned returns all worktree task IDs that exist on disk but are not registered with git
func (wm *WorktreeManager) ListOrphaned() ([]string, error) {
	// Get all git-registered worktrees
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = wm.baseDir
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("listing worktrees: %w", err)
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
		return nil, err
	}

	var orphaned []string
	for _, taskID := range onDisk {
		worktreePath := filepath.Join(wm.worktreeDir, taskID)
		// If not registered, it's orphaned
		if !registeredPaths[worktreePath] {
			orphaned = append(orphaned, taskID)
		}
	}

	return orphaned, nil
}

// RemoveByPath removes a worktree by its full path
func (wm *WorktreeManager) RemoveByPath(worktreePath string) error {
	// Remove git worktree registration if it exists
	cmd := exec.Command("git", "worktree", "remove", "--force", worktreePath)
	cmd.Dir = wm.baseDir
	_, _ = cmd.CombinedOutput() // Ignore errors

	// Remove the directory
	if err := os.RemoveAll(worktreePath); err != nil {
		return fmt.Errorf("removing worktree directory: %w", err)
	}

	// Prune stale git worktree registrations
	cmd = exec.Command("git", "worktree", "prune")
	cmd.Dir = wm.baseDir
	_ = cmd.Run()

	return nil
}

// RemoveAggressiveByPath removes a worktree by its full path and aggressively cleans up build artifacts
func (wm *WorktreeManager) RemoveAggressiveByPath(worktreePath string) (int64, error) {
	var sizeFreed int64

	// Clean up large build directories within the worktree
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

	// Clean up nested node_modules
	if err := wm.removeAllNestedDirectories(worktreePath, "node_modules"); err == nil {
		if wm.verbose {
			log.Printf("🗑️  Removed nested node_modules directories")
		}
	}

	// Clean up nested target directories
	if err := wm.removeAllNestedDirectories(worktreePath, "target"); err == nil {
		if wm.verbose {
			log.Printf("🗑️  Removed nested target directories")
		}
	}

	// Remove git worktree registration
	cmd := exec.Command("git", "worktree", "remove", "--force", worktreePath)
	cmd.Dir = wm.baseDir
	_, _ = cmd.CombinedOutput()

	// Remove the directory
	if err := os.RemoveAll(worktreePath); err != nil {
		return sizeFreed, fmt.Errorf("removing worktree directory: %w", err)
	}

	// Prune stale git worktree registrations
	cmd = exec.Command("git", "worktree", "prune")
	cmd.Dir = wm.baseDir
	_ = cmd.Run()

	return sizeFreed, nil
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
