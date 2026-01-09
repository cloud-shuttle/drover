// Package workflow implements durable workflows using DBOS
//
// DEPRECATED: This file contains the legacy SQLite-based Orchestrator.
// Drover now uses DBOS by default for both SQLite and PostgreSQL modes.
// See dbos_workflow.go for the current DBOS-based implementation.
// This file is kept for backwards compatibility and testing.
package workflow

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cloud-shuttle/drover/internal/beads"
	"github.com/cloud-shuttle/drover/internal/config"
	"github.com/cloud-shuttle/drover/internal/db"
	"github.com/cloud-shuttle/drover/internal/executor"
	"github.com/cloud-shuttle/drover/internal/git"
	"github.com/cloud-shuttle/drover/pkg/types"
)

// Orchestrator manages the main execution loop
type Orchestrator struct {
	config          *config.Config
	store           *db.Store
	git             *git.WorktreeManager
	executor        *executor.Executor
	workers         int
	verbose         bool // Enable verbose logging
	projectDir      string // Project directory for beads sync
}

// NewOrchestrator creates a new workflow orchestrator
func NewOrchestrator(cfg *config.Config, store *db.Store, projectDir string) (*Orchestrator, error) {
	gitMgr := git.NewWorktreeManager(
		projectDir,
		filepath.Join(projectDir, cfg.WorktreeDir),
	)
	gitMgr.SetVerbose(cfg.Verbose)

	exec := executor.NewExecutor(cfg.ClaudePath, cfg.TaskTimeout)
	exec.SetVerbose(cfg.Verbose)

	// Check Claude is installed
	if err := executor.CheckClaudeInstalled(cfg.ClaudePath); err != nil {
		return nil, fmt.Errorf("checking claude: %w", err)
	}

	return &Orchestrator{
		config:     cfg,
		store:      store,
		git:        gitMgr,
		executor:   exec,
		workers:    cfg.Workers,
		verbose:    cfg.Verbose,
		projectDir: projectDir,
	}, nil
}

// Run executes all tasks to completion
func (o *Orchestrator) Run(ctx context.Context) error {
	log.Printf("🐂 Starting Drover with %d workers", o.workers)

	// Start workers - they will claim tasks independently
	var wg sync.WaitGroup
	for i := 0; i < o.workers; i++ {
		wg.Add(1)
		go o.worker(ctx, i, &wg)
	}

	// Main orchestration loop - just print progress and check for completion
	ticker := time.NewTicker(o.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("🛑 Context cancelled, stopping...")
			wg.Wait()
			_ = o.git.Cleanup() // Clean up any remaining worktrees
			o.syncToBeadsIfNeeded()
			return ctx.Err()

		case <-ticker.C:
			// Check if we're done
			status, err := o.store.GetProjectStatus()
			if err != nil {
				log.Printf("Error getting status: %v", err)
				continue
			}

			// Calculate if we're complete
			active := status.Ready + status.InProgress + status.Claimed
			if active == 0 {
				log.Println("✅ All tasks complete!")
				wg.Wait()
				o.printFinalStatus(status)
				o.syncToBeadsIfNeeded()
				return nil
			}

			// Print progress
			o.printProgress(status)
		}
	}
}

// worker claims and executes tasks until the context is cancelled
func (o *Orchestrator) worker(ctx context.Context, id int, wg *sync.WaitGroup) {
	defer wg.Done()

	log.Printf("👷 Worker %d started", id)

	for {
		select {
		case <-ctx.Done():
			log.Printf("👷 Worker %d stopping (context cancelled)", id)
			return
		default:
			// Try to claim a task
			workerID := fmt.Sprintf("worker-%d-%d", id, time.Now().UnixNano())
			task, err := o.store.ClaimTask(workerID)
			if err != nil {
				log.Printf("Worker %d: error claiming task: %v", id, err)
				time.Sleep(time.Second)
				continue
			}

			if task == nil {
				// No tasks available, wait a bit before trying again
				time.Sleep(time.Second)
				continue
			}

			// Execute the task
			o.executeTask(id, task)
		}
	}
}

// executeTask executes a single task
func (o *Orchestrator) executeTask(workerID int, task *types.Task) {
	start := time.Now()
	taskCompleted := false

	log.Printf("👷 Worker %d executing task %s: %s", workerID, task.ID, task.Title)

	// Update to in_progress
	if err := o.store.UpdateTaskStatus(task.ID, types.TaskStatusInProgress, ""); err != nil {
		log.Printf("Error updating task status: %v", err)
	}

	// Ensure task is always marked complete or failed, even if we panic
	defer func() {
		if !taskCompleted {
			// Only mark as failed if task is still in_progress (i.e., error handler didn't run)
			// Don't mark as failed if task was set to ready for retry
			status, err := o.store.GetTaskStatus(task.ID)
			if err == nil && status == types.TaskStatusInProgress {
				log.Printf("⚠️  Task %s still in_progress at exit, marking as failed", task.ID)
				_ = o.store.UpdateTaskStatus(task.ID, types.TaskStatusFailed, "Task did not complete")
			}
		}
	}()

	// Create worktree
	worktreePath, err := o.git.Create(task)
	if err != nil {
		log.Printf("❌ Task %s failed: creating worktree: %v", task.ID, err)
		if o.handleTaskFailure(task.ID, err.Error()) {
			taskCompleted = true // Task set to ready for retry
		}
		return
	}
	defer o.git.Remove(task.ID)

	// Execute Claude Code and capture the result
	result := o.executor.ExecuteWithTimeout(worktreePath, task)
	if !result.Success {
		log.Printf("❌ Task %s failed: claude execution: %v", task.ID, result.Error)
		if o.handleTaskFailure(task.ID, result.Error.Error()) {
			taskCompleted = true // Task set to ready for retry
		}
		return
	}

	// Store the Claude output for later use (if no changes detected)
	claudeOutput := result.Output

	// Commit changes (if any)
	commitMsg := fmt.Sprintf("drover: %s\n\nTask: %s", task.ID, task.Title)
	hasChanges, err := o.git.Commit(task.ID, commitMsg)
	if err != nil {
		log.Printf("❌ Task %s failed: committing: %v", task.ID, err)
		if o.handleTaskFailure(task.ID, err.Error()) {
			taskCompleted = true // Task set to ready for retry
		}
		return
	}

	// Log diagnostic output when no changes were detected
	if !hasChanges && o.verbose {
		log.Printf("╔════════════════════════════════════════════════════════════════════════╗")
		log.Printf("║ ⚠️  Claude completed but made NO CHANGES for task %s", task.ID)
		log.Printf("╠════════════════════════════════════════════════════════════════════════╣")
		log.Printf("║ Claude Output:")
		log.Printf("╠──────────────────────────────────────────────────────────────────────────")
		for _, line := range strings.Split(claudeOutput, "\n") {
			log.Printf("║ %s", line)
		}
		log.Printf("╚════════════════════════════════════════════════════════════════════════╝")
	}

	// Try to merge to main (if there are changes to merge)
	if err := o.git.MergeToMain(task.ID); err != nil {
		// Log merge error but continue - task completed successfully even if merge failed
		log.Printf("⚠️  Task %s completed but merge failed: %v", task.ID, err)
		// Don't return here - continue to mark task as complete
	}

	// Mark complete and unblock dependents
	if err := o.store.CompleteTask(task.ID); err != nil {
		log.Printf("Error completing task: %v", err)
	}

	taskCompleted = true
	duration := time.Since(start)
	log.Printf("✅ Worker %d completed task %s in %v", workerID, task.ID, duration)
}

// handleTaskFailure increments attempts and either retries or marks as failed
// Returns true if the task was set to ready for retry (false if permanently failed)
func (o *Orchestrator) handleTaskFailure(taskID, errorMsg string) bool {
	// Fetch current task to check attempts before incrementing
	task, err := o.store.GetTask(taskID)
	if err != nil {
		log.Printf("Error fetching task %s: %v", taskID, err)
		_ = o.store.UpdateTaskStatus(taskID, types.TaskStatusFailed, errorMsg)
		return false
	}

	// Check if we've exceeded max attempts
	if task.Attempts >= task.MaxAttempts {
		_ = o.store.UpdateTaskStatus(taskID, types.TaskStatusFailed, errorMsg)
		log.Printf("❌ Task %s failed after %d attempts", taskID, task.Attempts)
		return false
	}

	// Increment attempts in database
	if err := o.store.IncrementTaskAttempts(taskID); err != nil {
		log.Printf("Error incrementing attempts for task %s: %v", taskID, err)
		_ = o.store.UpdateTaskStatus(taskID, types.TaskStatusFailed, errorMsg)
		return false
	}

	// Allow retry - task.Attempts is now incremented
	_ = o.store.UpdateTaskStatus(taskID, types.TaskStatusReady, errorMsg)
	log.Printf("🔄 Task %s retrying (attempt %d/%d)", taskID, task.Attempts+1, task.MaxAttempts)
	return true
}

// printProgress prints current progress
func (o *Orchestrator) printProgress(status *db.ProjectStatus) {
	if status.Total == 0 {
		return
	}

	progress := float64(status.Completed) / float64(status.Total) * 100
	log.Printf("📊 Progress: %d/%d tasks (%.1f%%) | Ready: %d | In Progress: %d | Blocked: %d | Failed: %d",
		status.Completed, status.Total, progress,
		status.Ready, status.InProgress, status.Blocked, status.Failed)
}

// printFinalStatus prints final run results
func (o *Orchestrator) printFinalStatus(status *db.ProjectStatus) {
	fmt.Println("\n🐂 Drover Run Complete")
	fmt.Println("═════════════════════")
	fmt.Printf("\nTotal tasks:     %d", status.Total)
	fmt.Printf("\nCompleted:       %d", status.Completed)
	fmt.Printf("\nFailed:          %d", status.Failed)
	fmt.Printf("\nBlocked:         %d", status.Blocked)

	if status.Total > 0 {
		successRate := float64(status.Completed) / float64(status.Total) * 100
		fmt.Printf("\n\nSuccess rate:    %.1f%%", successRate)
	}

	if status.Failed > 0 || status.Blocked > 0 {
		fmt.Println("\n\n⚠️  Some tasks did not complete successfully")
		fmt.Println("   Run 'drover status' for details")
	}
}

// syncToBeadsIfNeeded exports the current state to beads format if auto-sync is enabled
func (o *Orchestrator) syncToBeadsIfNeeded() {
	if !o.config.AutoSyncBeads {
		return
	}

	log.Println("📦 Syncing to beads format...")

	// Get all data from the store
	taskList, err := o.store.ListTasks()
	if err != nil {
		log.Printf("Error listing tasks for beads sync: %v", err)
		return
	}

	epicList, err := o.store.ListEpics()
	if err != nil {
		log.Printf("Error listing epics for beads sync: %v", err)
		return
	}

	deps, err := o.store.ListAllDependencies()
	if err != nil {
		log.Printf("Error listing dependencies for beads sync: %v", err)
		return
	}

	// Convert tasks from pointers to values
	tasks := make([]types.Task, len(taskList))
	for i, t := range taskList {
		tasks[i] = *t
	}

	// Convert epics from pointers to values
	epics := make([]types.Epic, len(epicList))
	for i, e := range epicList {
		epics[i] = *e
	}

	// Create beads sync config
	syncConfig := beads.DefaultSyncConfig(o.projectDir)
	syncConfig.AutoSync = true

	// Export to beads
	if err := beads.ExportToBeads(epics, tasks, deps, syncConfig); err != nil {
		log.Printf("Error exporting to beads: %v", err)
		return
	}

	log.Println("✅ Synced to beads format (.beads/beads.jsonl)")
}
