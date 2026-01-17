// Package git handles git worktree operations for parallel task execution
package git

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/cloud-shuttle/drover/pkg/telemetry"
)

// WorktreeState represents the current state of a worktree in the pool
type WorktreeState int

const (
	// StateCold - Worktree is allocated but not yet prepared
	StateCold WorktreeState = iota
	// StateWarming - Worktree is being prepared (syncing, installing dependencies)
	StateWarming
	// StateWarm - Worktree is ready for immediate task assignment
	StateWarm
	// StateInUse - Worktree is currently assigned to a task
	StateInUse
	// StateDraining - Worktree is being cleaned up for removal
	StateDraining
)

func (s WorktreeState) String() string {
	switch s {
	case StateCold:
		return "cold"
	case StateWarming:
		return "warming"
	case StateWarm:
		return "warm"
	case StateInUse:
		return "in-use"
	case StateDraining:
		return "draining"
	default:
		return "unknown"
	}
}

// PooledWorktree represents a worktree in the pool
type PooledWorktree struct {
	ID         string        // Unique identifier (pool index or timestamp)
	TaskID     string        // Currently assigned task (empty if not in use)
	Path       string        // File system path
	Branch     string        // Git branch name
	State      WorktreeState // Current state
	CreatedAt  time.Time     // When the worktree was created
	WarmedAt   time.Time     // When the worktree became warm
	AssignedAt time.Time     // When the worktree was assigned to a task
	mu         sync.Mutex    // Protects state transitions
	// Sync state for async git fetch
	LastFetchAt     time.Time // When the last fetch completed
	LastFetchStatus string    // Status of last fetch ("", "ok", "error")
	LastFetchError  string    // Error message if fetch failed
	IsReadOnly      bool      // True when sync is in progress
}

// PoolConfig holds configuration for the worktree pool
type PoolConfig struct {
	MinSize            int           // Minimum number of warm worktrees to maintain
	MaxSize            int           // Maximum number of worktrees in the pool
	ReplenishThreshold int           // Create new worktree when warm count falls below this
	WarmupTimeout      time.Duration // Max time to wait for worktree warmup
	CleanupOnExit      bool          // Whether to clean up pooled worktrees on exit
	EnableSymlinks     bool          // Enable shared node_modules via symlinks
	GoModCache         bool          // Enable Go module cache sharing
	CargoTargetDir     bool          // Enable shared Cargo target directory for Rust projects
}

// DefaultPoolConfig returns sensible defaults for the pool
func DefaultPoolConfig() *PoolConfig {
	return &PoolConfig{
		MinSize:            2,
		MaxSize:            10,
		ReplenishThreshold: 1,
		WarmupTimeout:      5 * time.Minute,
		CleanupOnExit:      true,
		EnableSymlinks:     true,
		GoModCache:         true,
		CargoTargetDir:     true,
	}
}

// WorktreePool manages a pool of pre-initialized worktrees
type WorktreePool struct {
	manager    *WorktreeManager
	config     *PoolConfig
	worktrees  map[string]*PooledWorktree // worktree ID -> PooledWorktree
	mu         sync.RWMutex               // Protects worktrees map
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	startOnce  sync.Once
	stopOnce   sync.Once
	shutdownCh chan struct{}

	// Dependency cache paths
	sharedNodeModules string // Path to shared node_modules
	sharedGoModCache  string // Path to Go module cache (GOMODCACHE)
	sharedCargoTarget string // Path to shared Cargo target directory
}

// NewWorktreePool creates a new worktree pool
func NewWorktreePool(manager *WorktreeManager, config *PoolConfig) *WorktreePool {
	if config == nil {
		config = DefaultPoolConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &WorktreePool{
		manager:    manager,
		config:     config,
		worktrees:  make(map[string]*PooledWorktree),
		ctx:        ctx,
		cancel:     cancel,
		shutdownCh: make(chan struct{}),
	}
}

// Start initializes the pool and begins maintaining the minimum number of warm worktrees
func (p *WorktreePool) Start() error {
	var startErr error
	p.startOnce.Do(func() {
		// Initialize dependency caches
		if err := p.initDependencyCaches(); err != nil {
			startErr = fmt.Errorf("initializing dependency caches: %w", err)
			return
		}

		// Check for cache invalidation and rebuild if needed
		needsRebuild, err := p.checkCacheInvalidation()
		if err != nil {
			log.Printf("Warning: failed to check cache invalidation: %v", err)
		} else if needsRebuild {
			if err := p.rebuildDependencyCaches(); err != nil {
				log.Printf("Warning: failed to rebuild caches: %v", err)
			}
		}

		// Clean up any stale pooled worktrees from previous runs
		if err := p.cleanupStalePooled(); err != nil {
			log.Printf("Warning: failed to cleanup stale pooled worktrees: %v", err)
		}

		// Start the replenishment goroutine
		p.wg.Add(1)
		go p.replenishLoop()

		// Initial warmup
		if err := p.ensureMinWarmWorktrees(p.ctx); err != nil {
			startErr = fmt.Errorf("initial warmup: %w", err)
			return
		}

		log.Printf("🚀 Worktree pool started (min=%d, max=%d)", p.config.MinSize, p.config.MaxSize)
	})

	return startErr
}

// IsWorktreeReadOnly returns true if the worktree is currently in read-only mode (syncing)
func (p *WorktreePool) IsWorktreeReadOnly(worktreeID string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	wt, exists := p.worktrees[worktreeID]
	if !exists {
		return false
	}

	wt.mu.Lock()
	defer wt.mu.Unlock()
	return wt.IsReadOnly
}

// Stop gracefully shuts down the pool
func (p *WorktreePool) Stop() error {
	p.stopOnce.Do(func() {
		log.Printf("🛑 Stopping worktree pool...")

		// Signal shutdown
		p.cancel()

		// Wait for replenishment loop to exit
		p.wg.Wait()

		// Clean up pooled worktrees if configured
		if p.config.CleanupOnExit {
			p.cleanupPooledWorktrees()
		}

		log.Printf("✅ Worktree pool stopped")
	})

	return nil
}

// Acquire acquires a warm worktree from the pool for a task
// Returns the worktree path, or an error if no worktree is available
func (p *WorktreePool) Acquire(taskID string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Find a warm worktree that's not in use and not in read-only mode
	for _, wt := range p.worktrees {
		wt.mu.Lock()
		// Skip worktrees that are syncing (read-only mode)
		if wt.IsReadOnly {
			wt.mu.Unlock()
			continue
		}
		if wt.State == StateWarm && wt.TaskID == "" {
			// Found a warm, available worktree
			wt.State = StateInUse
			wt.TaskID = taskID
			wt.AssignedAt = time.Now()
			wt.mu.Unlock()

			log.Printf("🎯 Acquired worktree %s for task %s", wt.ID, taskID)
			return wt.Path, nil
		}
		wt.mu.Unlock()
	}

	// No warm worktrees available, check if we can create a new one
	if len(p.worktrees) < p.config.MaxSize {
		p.mu.Unlock()
		// Create and warm a new worktree
		if err := p.createAndWarmWorktree(taskID); err != nil {
			p.mu.Lock()
			return "", fmt.Errorf("creating warm worktree: %w", err)
		}
		p.mu.Lock()

		// Find the newly created worktree
		for _, wt := range p.worktrees {
			if wt.TaskID == taskID {
				log.Printf("🎯 Created and acquired worktree %s for task %s", wt.ID, taskID)
				return wt.Path, nil
			}
		}
	}

	return "", fmt.Errorf("no warm worktrees available (pool size: %d/%d)", p.countByState(StateWarm), p.config.MaxSize)
}

// Release releases a worktree back to the pool after task completion
func (p *WorktreePool) Release(taskID string, retain bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Find the worktree assigned to this task
	for _, wt := range p.worktrees {
		wt.mu.Lock()
		if wt.TaskID == taskID && wt.State == StateInUse {
			wt.TaskID = ""
			wt.AssignedAt = time.Time{}

			if retain {
				// Return to pool as warm
				wt.State = StateWarm
				wt.WarmedAt = time.Now()
				wt.mu.Unlock()
				log.Printf("↩️  Released worktree %s back to pool (warm)", wt.ID)
			} else {
				// Mark for draining - will be removed by replenish loop
				wt.State = StateDraining
				wt.mu.Unlock()
				log.Printf("🗑️  Released worktree %s for cleanup", wt.ID)
			}
			return nil
		}
		wt.mu.Unlock()
	}

	return fmt.Errorf("worktree for task %s not found", taskID)
}

// Stats returns pool statistics
func (p *WorktreePool) Stats() PoolStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return PoolStats{
		Total:    len(p.worktrees),
		Cold:     p.countByState(StateCold),
		Warming:  p.countByState(StateWarming),
		Warm:     p.countByState(StateWarm),
		InUse:    p.countByState(StateInUse),
		Draining: p.countByState(StateDraining),
		MinSize:  p.config.MinSize,
		MaxSize:  p.config.MaxSize,
	}
}

// PoolStats holds pool statistics
type PoolStats struct {
	Total    int
	Cold     int
	Warming  int
	Warm     int
	InUse    int
	Draining int
	MinSize  int
	MaxSize  int
}

// FetchSyncResult represents the result of an async git fetch operation
type FetchSyncResult struct {
	WorktreeID     string    // ID of the worktree that was fetched
	CompletedAt    time.Time // When the fetch completed
	Success        bool      // True if fetch succeeded
	Error          string    // Error message if fetch failed
	CommitsFetched int       // Number of commits fetched (if available)
}

// FetchAll initiates async git fetch for all worktrees in the pool.
// Returns a channel that will receive results for each worktree as fetches complete.
// The channel is closed after all results are sent.
func (p *WorktreePool) FetchAll() <-chan FetchSyncResult {
	p.mu.RLock()
	worktreeCount := len(p.worktrees)
	worktreesCopy := make([]*PooledWorktree, 0, worktreeCount)
	for _, wt := range p.worktrees {
		if wt.Path != "" && wt.State != StateDraining {
			worktreesCopy = append(worktreesCopy, wt)
		}
	}
	p.mu.RUnlock()

	resultCh := make(chan FetchSyncResult, worktreeCount)

	// Launch fetch for each worktree in parallel
	for _, wt := range worktreesCopy {
		p.wg.Add(1)
		go func(wt *PooledWorktree) {
			defer p.wg.Done()
			result := p.fetchWorktree(wt)
			resultCh <- result
		}(wt)
	}

	// Close channel when all fetches complete
	go func() {
		p.wg.Wait()
		close(resultCh)
	}()

	return resultCh
}

// FetchWorktree fetches updates for a specific worktree and returns the result.
// This is a synchronous, blocking operation.
func (p *WorktreePool) FetchWorktree(worktreeID string) FetchSyncResult {
	p.mu.RLock()
	wt, exists := p.worktrees[worktreeID]
	p.mu.RUnlock()

	if !exists {
		return FetchSyncResult{
			WorktreeID:  worktreeID,
			CompletedAt: time.Now(),
			Success:     false,
			Error:       "worktree not found",
		}
	}

	return p.fetchWorktree(wt)
}

// fetchWorktree performs the actual git fetch for a worktree.
// It sets the worktree to read-only mode during fetch and reports completion via result.
func (p *WorktreePool) fetchWorktree(wt *PooledWorktree) FetchSyncResult {
	startTime := time.Now()

	// Mark worktree as read-only during fetch
	wt.mu.Lock()
	wt.IsReadOnly = true
	wt.mu.Unlock()

	defer func() {
		// Clear read-only mode when done
		wt.mu.Lock()
		wt.IsReadOnly = false
		wt.LastFetchAt = time.Now()
		wt.mu.Unlock()
	}()

	// Perform git fetch in the worktree
	cmd := exec.Command("git", "fetch", "origin")
	cmd.Dir = wt.Path
	output, err := cmd.CombinedOutput()

	wt.mu.Lock()
	defer wt.mu.Unlock()

	if err != nil {
		wt.LastFetchStatus = "error"
		wt.LastFetchError = fmt.Sprintf("%v: %s", err, string(output))
		duration := time.Since(startTime)
		log.Printf("⚠️  Git fetch failed for worktree %s: %v", wt.ID, err)

		// Record failed sync telemetry
		telemetry.RecordSyncFailed(context.Background(), wt.ID, p.manager.baseDir, "git_error", duration)

		return FetchSyncResult{
			WorktreeID:     wt.ID,
			CompletedAt:    time.Now(),
			Success:        false,
			Error:          wt.LastFetchError,
			CommitsFetched: 0,
		}
	}

	// Update success status
	wt.LastFetchStatus = "ok"
	wt.LastFetchError = ""
	duration := time.Since(startTime)

	log.Printf("✅ Git fetch completed for worktree %s in %v", wt.ID, duration)

	// Record successful sync telemetry
	telemetry.RecordSyncCompleted(context.Background(), wt.ID, p.manager.baseDir, duration, 0)

	return FetchSyncResult{
		WorktreeID:     wt.ID,
		CompletedAt:    time.Now(),
		Success:        true,
		Error:          "",
		CommitsFetched: 0, // Could parse output to count commits
	}
}

// GetSyncStatus returns the sync status for all worktrees
func (p *WorktreePool) GetSyncStatus() map[string]SyncStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()

	status := make(map[string]SyncStatus)
	for id, wt := range p.worktrees {
		wt.mu.Lock()
		status[id] = SyncStatus{
			WorktreeID:      id,
			LastFetchAt:     wt.LastFetchAt,
			LastFetchStatus: wt.LastFetchStatus,
			LastFetchError:  wt.LastFetchError,
			IsReadOnly:      wt.IsReadOnly,
		}
		wt.mu.Unlock()
	}
	return status
}

// SyncStatus represents the sync status of a worktree
type SyncStatus struct {
	WorktreeID      string    `json:"worktree_id"`
	LastFetchAt     time.Time `json:"last_fetch_at"`
	LastFetchStatus string    `json:"last_fetch_status"`
	LastFetchError  string    `json:"last_fetch_error,omitempty"`
	IsReadOnly      bool      `json:"is_read_only"`
}

// replenishLoop maintains the pool by ensuring minimum warm worktrees
func (p *WorktreePool) replenishLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			// Check for cache invalidation periodically
			needsRebuild, err := p.checkCacheInvalidation()
			if err != nil {
				log.Printf("⚠️  Failed to check cache invalidation: %v", err)
			} else if needsRebuild {
				log.Printf("🔄 Dependency caches invalidated, rebuilding...")
				if err := p.rebuildDependencyCaches(); err != nil {
					log.Printf("⚠️  Failed to rebuild caches: %v", err)
				}
			}

			// Ensure minimum warm worktrees
			if err := p.ensureMinWarmWorktrees(p.ctx); err != nil {
				log.Printf("⚠️  Failed to maintain pool: %v", err)
			}

			// Clean up draining worktrees
			p.cleanupDrainingWorktrees()
		}
	}
}

// ensureMinWarmWorktrees ensures the pool has at least MinSize warm worktrees
func (p *WorktreePool) ensureMinWarmWorktrees(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	warmCount := p.countByState(StateWarm)
	if warmCount >= p.config.MinSize {
		return nil
	}

	// Need to create more warm worktrees
	needed := p.config.MinSize - warmCount
	for i := 0; i < needed; i++ {
		if len(p.worktrees) >= p.config.MaxSize {
			break
		}

		wt := &PooledWorktree{
			ID:        fmt.Sprintf("pool-%d", time.Now().UnixNano()),
			TaskID:    "",
			Path:      "",
			Branch:    "",
			State:     StateCold,
			CreatedAt: time.Now(),
		}

		p.worktrees[wt.ID] = wt

		// Start warmup in background
		p.wg.Add(1)
		go func(wt *PooledWorktree) {
			defer p.wg.Done()
			p.warmupWorktree(ctx, wt)
		}(wt)
	}

	return nil
}

// createAndWarmWorktree creates a worktree specifically for a task and warms it up
func (p *WorktreePool) createAndWarmWorktree(taskID string) error {
	// Create worktree path using task ID
	worktreePath := filepath.Join(p.manager.worktreeDir, taskID)
	branchName := fmt.Sprintf("drover-%s", taskID)

	// Ensure directory exists
	if err := os.MkdirAll(p.manager.worktreeDir, 0755); err != nil {
		return fmt.Errorf("creating worktree directory: %w", err)
	}

	// Clean up any existing worktree
	p.manager.cleanUpWorktree(taskID)

	// Delete the branch if it exists
	cmd := exec.Command("git", "branch", "-D", branchName)
	cmd.Dir = p.manager.baseDir
	_, _ = cmd.CombinedOutput() // Ignore errors - branch may not exist

	// Create the worktree
	cmd = exec.Command("git", "worktree", "add", "-b", branchName, worktreePath)
	cmd.Dir = p.manager.baseDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("creating worktree: %w\n%s", err, output)
	}

	// Create pooled worktree entry
	p.mu.Lock()
	wt := &PooledWorktree{
		ID:         taskID,
		TaskID:     taskID,
		Path:       worktreePath,
		Branch:     branchName,
		State:      StateInUse, // Already assigned to the task
		CreatedAt:  time.Now(),
		WarmedAt:   time.Now(),
		AssignedAt: time.Now(),
	}
	p.worktrees[wt.ID] = wt
	p.mu.Unlock()

	// Still do the warmup (dependency setup) even though we're assigning immediately
	if err := p.setupDependencies(worktreePath); err != nil {
		log.Printf("⚠️  Failed to setup dependencies for worktree %s: %v", wt.ID, err)
	}

	return nil
}

// warmupWorktree warms up a worktree by syncing and installing dependencies
func (p *WorktreePool) warmupWorktree(ctx context.Context, wt *PooledWorktree) {
	wt.mu.Lock()
	wt.State = StateWarming
	wt.mu.Unlock()

	// Create a temporary worktree path
	worktreePath := filepath.Join(p.manager.worktreeDir, wt.ID)
	branchName := fmt.Sprintf("drover-%s", wt.ID)

	// Ensure directory exists
	if err := os.MkdirAll(p.manager.worktreeDir, 0755); err != nil {
		log.Printf("❌ Failed to create worktree directory for %s: %v", wt.ID, err)
		wt.mu.Lock()
		wt.State = StateDraining
		wt.mu.Unlock()
		return
	}

	// Clean up any existing worktree
	p.manager.cleanUpWorktree(wt.ID)

	// Create the worktree
	cmd := exec.Command("git", "worktree", "add", "-b", branchName, worktreePath)
	cmd.Dir = p.manager.baseDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("❌ Failed to create worktree for %s: %v\n%s", wt.ID, err, output)
		wt.mu.Lock()
		wt.State = StateDraining
		wt.mu.Unlock()
		return
	}

	wt.mu.Lock()
	wt.Path = worktreePath
	wt.Branch = branchName
	wt.mu.Unlock()

	// Setup dependencies (symlinks, Go mod cache, etc.)
	if err := p.setupDependencies(worktreePath); err != nil {
		log.Printf("⚠️  Failed to setup dependencies for worktree %s: %v", wt.ID, err)
	}

	wt.mu.Lock()
	wt.State = StateWarm
	wt.WarmedAt = time.Now()
	wt.mu.Unlock()

	log.Printf("✅ Worktree %s is warm and ready", wt.ID)
}

// setupDependencies sets up shared dependencies for a worktree
func (p *WorktreePool) setupDependencies(worktreePath string) error {
	// Setup shared node_modules symlink if enabled
	if p.config.EnableSymlinks && p.sharedNodeModules != "" {
		nodeModulesPath := filepath.Join(worktreePath, "node_modules")
		if err := os.Symlink(p.sharedNodeModules, nodeModulesPath); err != nil && !os.IsExist(err) {
			log.Printf("⚠️  Failed to create node_modules symlink: %v", err)
		}
	}

	// Go module cache is shared automatically via GOMODCACHE env var
	// This is set when the pool is initialized

	return nil
}

// initDependencyCaches initializes shared dependency caches
func (p *WorktreePool) initDependencyCaches() error {
	baseDir := p.manager.baseDir
	cacheDir := filepath.Join(baseDir, ".drover", "cache")

	// Create cache directory
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("creating cache directory: %w", err)
	}

	// Initialize shared node_modules if enabled
	if p.config.EnableSymlinks {
		p.sharedNodeModules = filepath.Join(cacheDir, "node_modules_shared")
		if _, err := os.Stat(p.sharedNodeModules); os.IsNotExist(err) {
			// Create placeholder - will be populated by first install
			if err := os.MkdirAll(p.sharedNodeModules, 0755); err != nil {
				return fmt.Errorf("creating shared node_modules: %w", err)
			}
		}
	}

	// Set up Go module cache
	if p.config.GoModCache {
		p.sharedGoModCache = filepath.Join(cacheDir, "gomodcache")
		if err := os.Setenv("GOMODCACHE", p.sharedGoModCache); err != nil {
			log.Printf("⚠️  Failed to set GOMODCACHE: %v", err)
		}
		if err := os.MkdirAll(p.sharedGoModCache, 0755); err != nil {
			return fmt.Errorf("creating shared GOMODCACHE: %w", err)
		}
	}

	// Set up shared Cargo target directory
	if p.config.CargoTargetDir {
		p.sharedCargoTarget = filepath.Join(cacheDir, "cargo_target_shared")
		if err := os.MkdirAll(p.sharedCargoTarget, 0755); err != nil {
			return fmt.Errorf("creating shared Cargo target directory: %w", err)
		}
		// Set CARGO_TARGET_DIR environment variable for child processes
		if err := os.Setenv("CARGO_TARGET_DIR", p.sharedCargoTarget); err != nil {
			log.Printf("⚠️  Failed to set CARGO_TARGET_DIR: %v", err)
		}
	}

	log.Printf("📦 Dependency caches initialized (node_modules: %s, gomodcache: %s, cargo_target: %s)",
		p.sharedNodeModules, p.sharedGoModCache, p.sharedCargoTarget)

	return nil
}

// cleanupDrainingWorktrees removes worktrees marked for cleanup
func (p *WorktreePool) cleanupDrainingWorktrees() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for id, wt := range p.worktrees {
		wt.mu.Lock()
		if wt.State == StateDraining {
			// Remove the worktree
			if wt.Path != "" {
				p.manager.RemoveAggressive(wt.ID)
			}
			delete(p.worktrees, id)
			log.Printf("🗑️  Cleaned up worktree %s", id)
		}
		wt.mu.Unlock()
	}
}

// cleanupPooledWorktrees removes all pooled worktrees
func (p *WorktreePool) cleanupPooledWorktrees() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for id, wt := range p.worktrees {
		wt.mu.Lock()
		if wt.Path != "" {
			p.manager.RemoveAggressive(wt.ID)
		}
		wt.mu.Unlock()
		delete(p.worktrees, id)
	}
}

// cleanupStalePooled removes worktrees from previous runs
func (p *WorktreePool) cleanupStalePooled() error {
	worktrees, err := p.manager.ListWorktreesOnDisk()
	if err != nil {
		return err
	}

	for _, wtID := range worktrees {
		// Check if it's a pooled worktree (starts with "pool-")
		if len(wtID) > 5 && wtID[:5] == "pool-" {
			log.Printf("🧹 Cleaning up stale pooled worktree: %s", wtID)
			p.manager.RemoveAggressive(wtID)
		}
	}

	return nil
}

// countByState returns the number of worktrees in a given state
func (p *WorktreePool) countByState(state WorktreeState) int {
	count := 0
	for _, wt := range p.worktrees {
		wt.mu.Lock()
		if wt.State == state {
			count++
		}
		wt.mu.Unlock()
	}
	return count
}

// IsEnabled returns true if worktree pooling is enabled
func (p *WorktreePool) IsEnabled() bool {
	return p.config.MaxSize > 0
}

// GetSharedNodeModulesPath returns the path to the shared node_modules directory
func (p *WorktreePool) GetSharedNodeModulesPath() string {
	return p.sharedNodeModules
}

// GetSharedGoModCachePath returns the path to the shared Go module cache
func (p *WorktreePool) GetSharedGoModCachePath() string {
	return p.sharedGoModCache
}

// GetSharedCargoTargetPath returns the path to the shared Cargo target directory
func (p *WorktreePool) GetSharedCargoTargetPath() string {
	return p.sharedCargoTarget
}

// CacheState tracks the state of dependency caches
type CacheState struct {
	NodeModulesHash string `json:"node_modules_hash,omitempty"`
	GoSumHash       string `json:"go_sum_hash,omitempty"`
	YarnLockHash    string `json:"yarn_lock_hash,omitempty"`
	PackageLockHash string `json:"package_lock_hash,omitempty"`
	CargoLockHash   string `json:"cargo_lock_hash,omitempty"`
	LastUpdated     int64  `json:"last_updated"`
}

// cacheStatePath returns the path to the cache state file
func (p *WorktreePool) cacheStatePath() string {
	cacheDir := filepath.Join(p.manager.baseDir, ".drover", "cache")
	return filepath.Join(cacheDir, "cache_state.json")
}

// loadCacheState loads the cache state from disk
func (p *WorktreePool) loadCacheState() (*CacheState, error) {
	path := p.cacheStatePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &CacheState{}, nil
		}
		return nil, err
	}

	var state CacheState
	if err := json.Unmarshal(data, &state); err != nil {
		return &CacheState{}, err
	}
	return &state, nil
}

// saveCacheState saves the cache state to disk
func (p *WorktreePool) saveCacheState(state *CacheState) error {
	path := p.cacheStatePath()
	state.LastUpdated = time.Now().Unix()

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// computeFileHash computes the SHA256 hash of a file
func (p *WorktreePool) computeFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil // File doesn't exist, return empty hash
		}
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// checkCacheInvalidation checks if dependency caches need to be rebuilt
// Returns true if cache is invalid and needs rebuild
func (p *WorktreePool) checkCacheInvalidation() (bool, error) {
	oldState, err := p.loadCacheState()
	if err != nil {
		log.Printf("⚠️  Failed to load cache state: %v", err)
		return true, nil // On error, assume cache is invalid
	}

	// Check for lock file changes
	lockFiles := []struct {
		path     string
		oldHash  string
		newHash  string
		cacheKey string
	}{
		{"package-lock.json", oldState.PackageLockHash, "", "package_lock_hash"},
		{"yarn.lock", oldState.YarnLockHash, "", "yarn_lock_hash"},
		{"go.sum", oldState.GoSumHash, "", "go_sum_hash"},
		{"Cargo.lock", oldState.CargoLockHash, "", "cargo_lock_hash"},
	}

	needsRebuild := false

	for _, lf := range lockFiles {
		lfPath := filepath.Join(p.manager.baseDir, lf.path)
		hash, err := p.computeFileHash(lfPath)
		if err != nil {
			log.Printf("⚠️  Failed to compute hash for %s: %v", lf.path, err)
			continue
		}

		if hash != "" && hash != lf.oldHash {
			log.Printf("🔄 Lock file %s has changed (old: %s, new: %s), cache needs rebuild",
				lf.path, lf.oldHash[:8], hash[:8])
			needsRebuild = true
		}
	}

	return needsRebuild, nil
}

// updateCacheState updates the cache state with current lock file hashes
func (p *WorktreePool) updateCacheState() error {
	lockFiles := []struct {
		path     string
		hash     string
		cacheKey string
	}{
		{"package-lock.json", "", "package_lock_hash"},
		{"yarn.lock", "", "yarn_lock_hash"},
		{"go.sum", "", "go_sum_hash"},
		{"Cargo.lock", "", "cargo_lock_hash"},
	}

	state, err := p.loadCacheState()
	if err != nil {
		return err
	}

	for _, lf := range lockFiles {
		lfPath := filepath.Join(p.manager.baseDir, lf.path)
		hash, err := p.computeFileHash(lfPath)
		if err != nil {
			log.Printf("⚠️  Failed to compute hash for %s: %v", lf.path, err)
			continue
		}

		switch lf.cacheKey {
		case "package_lock_hash":
			state.PackageLockHash = hash
		case "yarn_lock_hash":
			state.YarnLockHash = hash
		case "go_sum_hash":
			state.GoSumHash = hash
		case "cargo_lock_hash":
			state.CargoLockHash = hash
		}
	}

	return p.saveCacheState(state)
}

// rebuildDependencyCaches rebuilds the shared dependency caches
func (p *WorktreePool) rebuildDependencyCaches() error {
	log.Printf("🔄 Rebuilding dependency caches...")

	// Remove old shared node_modules
	if p.sharedNodeModules != "" {
		if err := os.RemoveAll(p.sharedNodeModules); err != nil {
			log.Printf("⚠️  Failed to remove old node_modules: %v", err)
		}
		// Recreate directory
		if err := os.MkdirAll(p.sharedNodeModules, 0755); err != nil {
			return fmt.Errorf("recreating shared node_modules: %w", err)
		}
	}

	// Remove old Go module cache
	if p.sharedGoModCache != "" {
		if err := os.RemoveAll(p.sharedGoModCache); err != nil {
			log.Printf("⚠️  Failed to remove old GOMODCACHE: %v", err)
		}
		// Recreate directory
		if err := os.MkdirAll(p.sharedGoModCache, 0755); err != nil {
			return fmt.Errorf("recreating GOMODCACHE: %w", err)
		}
	}

	// Remove old Cargo target directory
	if p.sharedCargoTarget != "" {
		if err := os.RemoveAll(p.sharedCargoTarget); err != nil {
			log.Printf("⚠️  Failed to remove old Cargo target directory: %v", err)
		}
		// Recreate directory
		if err := os.MkdirAll(p.sharedCargoTarget, 0755); err != nil {
			return fmt.Errorf("recreating shared Cargo target directory: %w", err)
		}
	}

	// Update cache state with new hashes
	if err := p.updateCacheState(); err != nil {
		log.Printf("⚠️  Failed to update cache state: %v", err)
	}

	log.Printf("✅ Dependency caches rebuilt")
	return nil
}
