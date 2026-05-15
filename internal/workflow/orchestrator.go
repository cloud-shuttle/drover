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
	"github.com/cloud-shuttle/drover/internal/dashboard"
	"github.com/cloud-shuttle/drover/internal/db"
	"github.com/cloud-shuttle/drover/internal/executor"
	"github.com/cloud-shuttle/drover/internal/git"
	"github.com/cloud-shuttle/drover/pkg/telemetry"
	"github.com/cloud-shuttle/drover/pkg/types"
	"github.com/cloud-shuttle/drover-libs/pkg/clock"
)

// Orchestrator manages the main execution loop
type Orchestrator struct {
	config   *config.Config
	store    *db.Store
	git      GitManager
	agent    executor.Agent // Agent interface for Claude/Codex/Amp
	workers  int
	verbose  bool // Enable verbose logging
	projectDir string // Project directory for beads sync
	epicID   string // Optional epic filter for task execution
	clock    clock.Clock
}

// NewOrchestrator creates a new workflow orchestrator
func NewOrchestrator(cfg *config.Config, store *db.Store, projectDir string, clk clock.Clock) (*Orchestrator, error) {
	gitMgr := git.NewWorktreeManager(
		projectDir,
		filepath.Join(projectDir, cfg.WorktreeDir),
	)
	gitMgr.SetVerbose(cfg.Verbose)

	// Create the agent based on configuration
	agent, err := executor.NewAgent(&executor.AgentConfig{
		Type:       cfg.AgentType,
		Path:       cfg.AgentPath,
		Timeout:    cfg.TaskTimeout,
		Verbose:    cfg.Verbose,
		Guidelines: cfg.Guidelines,
		DroverCode: cfg.DroverCode,
	})
	if err != nil {
		return nil, fmt.Errorf("creating agent: %w", err)
	}

	agent.SetVerbose(cfg.Verbose)

	// Check agent is installed
	if err := agent.CheckInstalled(); err != nil {
		return nil, fmt.Errorf("checking %s: %w", cfg.AgentType, err)
	}

	return &Orchestrator{
		config:     cfg,
		store:      store,
		git:        gitMgr,
		agent:      agent,
		workers:    cfg.Workers,
		verbose:    cfg.Verbose,
		projectDir: projectDir,
		clock:      clk,
	}, nil
}

// SetEpicFilter sets the epic filter for task execution
// Only tasks belonging to the specified epic will be executed
func (o *Orchestrator) SetEpicFilter(epicID string) {
	o.epicID = epicID
}

// Run executes all tasks to completion
func (o *Orchestrator) Run(ctx context.Context) error {
	log.Printf("🐂 Starting Drover with %d workers", o.workers)
	if o.epicID != "" {
		log.Printf("🎯 Filtering to epic: %s", o.epicID)
	}

	// Start workflow span for telemetry
	_, workflowSpan := telemetry.StartWorkflowSpan(ctx, telemetry.SpanWorkflowRun, "")
	defer workflowSpan.End()

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
			// Try to claim a task (filtered by epic if set)
			workerID := fmt.Sprintf("worker-%d-%d", id, o.clock.Now().UnixNano())
			task, err := o.store.ClaimTaskForEpic(workerID, o.epicID)
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

			// Broadcast task claimed to dashboard
			dashboard.BroadcastTaskClaimed(task.ID, task.Title, workerID)

			// Execute the task
			o.executeTask(id, task)
		}
	}
}

// executeTask executes a single task
func (o *Orchestrator) executeTask(workerID int, task *types.Task) {
	start := o.clock.Now()
	taskCompleted := false

	log.Printf("👷 Worker %d executing task %s: %s", workerID, task.ID, task.Title)

	// If the task was cancelled after being claimed, skip execution.
	if status, err := o.store.GetTaskStatus(task.ID); err == nil && status == types.TaskStatusCancelled {
		log.Printf("🛑 Task %s was cancelled; skipping execution", task.ID)
		return
	}

	// Check if task has sub-tasks - execute them first
	hasChildren, err := o.store.HasSubTasks(task.ID)
	if err != nil {
		log.Printf("Error checking for sub-tasks: %v", err)
	}
	if hasChildren {
		log.Printf("📋 Task %s has sub-tasks, executing them first", task.ID)
		if !o.executeSubTasks(workerID, task) {
			// Sub-tasks failed, mark parent as failed
			log.Printf("❌ Task %s failed due to sub-task failures", task.ID)
			_ = o.store.UpdateTaskStatus(task.ID, types.TaskStatusFailed, "Sub-tasks failed")
			taskCompleted = true
			return
		}
	}

	// Start telemetry span for task execution
	taskCtx, taskSpan := telemetry.StartTaskSpan(context.Background(),
		telemetry.SpanTaskExecute,
		telemetry.TaskAttrs(task.ID, task.Title, "in_progress", task.Priority, task.Attempts)...)

	workerIDStr := fmt.Sprintf("worker-%d", workerID)
	telemetry.RecordTaskClaimed(taskCtx, workerIDStr, o.epicID)
	defer taskSpan.End()

	// Update to in_progress
	if err := o.store.UpdateTaskStatus(task.ID, types.TaskStatusInProgress, ""); err != nil {
		log.Printf("Error updating task status: %v", err)
	}

	// Broadcast task started to dashboard
	dashboard.BroadcastTaskStarted(task.ID, task.Title, workerIDStr)

	// Ensure task is always marked complete or failed, even if we panic
	defer func() {
		if !taskCompleted {
			// Only mark as failed if task is still in_progress (i.e., error handler didn't run)
			// Don't mark as failed if task was set to ready for retry
			status, err := o.store.GetTaskStatus(task.ID)
			if err == nil {
				// Never override a cancellation
				if status == types.TaskStatusCancelled {
					return
				}
				if status == types.TaskStatusInProgress {
					log.Printf("⚠️  Task %s still in_progress at exit, marking as failed", task.ID)
					_ = o.store.UpdateTaskStatus(task.ID, types.TaskStatusFailed, "Task did not complete")
				}
			}
		}
	}()

	// Create worktree
	worktreePath, err := o.git.Create(task)
	if err != nil {
		log.Printf("❌ Task %s failed: creating worktree: %v", task.ID, err)
		telemetry.RecordError(taskSpan, err, "WorktreeCreationFailed", "git")
		telemetry.SetTaskStatus(taskSpan, "failed")
		if o.handleTaskFailure(task.ID, err.Error()) {
			taskCompleted = true // Task set to ready for retry
		}
		return
	}
	defer o.git.Remove(task.ID)

	// Epic 3: Context window management (write large payloads to file + replace description)
	preparedTask := *task
	// Epic 6: Task context carrying (prepend recent task summaries)
	InjectRecentTaskContext(o.store, o.config, &preparedTask)
	if err := PrepareTaskContextForAgent(worktreePath, o.config, &preparedTask); err != nil {
		log.Printf("⚠️  Failed to prepare task context for %s: %v", task.ID, err)
	}

	// Execute Claude Code and capture the result
	result := o.agent.ExecuteWithContext(taskCtx, worktreePath, &preparedTask, taskSpan)
	if !result.Success {
		log.Printf("❌ Task %s failed: claude execution: %v", task.ID, result.Error)
		telemetry.RecordError(taskSpan, result.Error, "AgentExecutionFailed", "agent")
		telemetry.SetTaskStatus(taskSpan, "failed")
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
		telemetry.RecordError(taskSpan, err, "CommitFailed", "git")
		telemetry.SetTaskStatus(taskSpan, "failed")
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
		telemetry.RecordError(taskSpan, err, "MergeFailed", "git")
		// Don't return here - continue to mark task as complete
	}

	// Mark complete and unblock dependents
	if err := o.store.CompleteTask(task.ID); err != nil {
		log.Printf("Error completing task: %v", err)
	}

	taskCompleted = true
	duration := time.Since(start)
	log.Printf("✅ Worker %d completed task %s in %v", workerID, task.ID, duration)

	// Broadcast task completed to dashboard
	dashboard.BroadcastTaskCompleted(task.ID, task.Title)

	// Record task completion telemetry
	telemetry.SetTaskStatus(taskSpan, "completed")
	telemetry.RecordTaskCompleted(taskCtx, workerIDStr, o.epicID, duration)
}

// executeSubTasks executes all sub-tasks of a parent task
// Returns true if all sub-tasks succeeded, false if any failed
func (o *Orchestrator) executeSubTasks(workerID int, parentTask *types.Task) bool {
	subTasks, err := o.store.GetSubTasks(parentTask.ID)
	if err != nil {
		log.Printf("Error getting sub-tasks: %v", err)
		return false
	}

	if len(subTasks) == 0 {
		return true
	}

	log.Printf("📋 Executing %d sub-tasks for %s", len(subTasks), parentTask.ID)

	// Execute sub-tasks sequentially in order
	for _, subTask := range subTasks {
		log.Printf("📋 Executing sub-task %s: %s", subTask.ID, subTask.Title)

		// Update sub-task status to in_progress
		if err := o.store.UpdateTaskStatus(subTask.ID, types.TaskStatusInProgress, ""); err != nil {
			log.Printf("Error updating sub-task status: %v", err)
			continue
		}

		// Defence-in-depth: CreateSubTask already enforces max depth of 2 levels
		// (it rejects creation when parent.ParentID != ""). This guard protects
		// against manual DB edits or future changes to that constraint.
		hasChildren, _ := o.store.HasSubTasks(subTask.ID)
		if hasChildren {
			log.Printf("❌ Sub-task %s has children (max depth exceeded)", subTask.ID)
			o.store.UpdateTaskStatus(subTask.ID, types.TaskStatusFailed, "Max depth exceeded")
			return false
		}

		// Create worktree for sub-task
		worktreePath, err := o.git.Create(subTask)
		if err != nil {
			log.Printf("❌ Sub-task %s failed: creating worktree: %v", subTask.ID, err)
			o.handleTaskFailure(subTask.ID, err.Error())
			return false
		}

		// Execute sub-task
		start := o.clock.Now()
		taskCtx, taskSpan := telemetry.StartTaskSpan(context.Background(),
			telemetry.SpanTaskExecute,
			telemetry.TaskAttrs(subTask.ID, subTask.Title, "in_progress", subTask.Priority, subTask.Attempts)...)
		telemetry.RecordTaskClaimed(taskCtx, fmt.Sprintf("worker-%d", workerID), parentTask.EpicID)
		defer taskSpan.End()

		preparedSubTask := *subTask
		InjectRecentTaskContext(o.store, o.config, &preparedSubTask)
		if err := PrepareTaskContextForAgent(worktreePath, o.config, &preparedSubTask); err != nil {
			log.Printf("⚠️  Failed to prepare task context for %s: %v", subTask.ID, err)
		}

		result := o.agent.ExecuteWithContext(taskCtx, worktreePath, &preparedSubTask, taskSpan)

		// Clean up worktree
		o.git.Remove(subTask.ID)

		if !result.Success {
			log.Printf("❌ Sub-task %s failed: %v", subTask.ID, result.Error)
			telemetry.RecordError(taskSpan, result.Error, "AgentExecutionFailed", "agent")
			telemetry.SetTaskStatus(taskSpan, "failed")
			o.handleTaskFailure(subTask.ID, result.Error.Error())
			return false
		}

		// Commit changes
		commitMsg := fmt.Sprintf("drover: %s (sub-task of %s)\n\nTask: %s", subTask.ID, parentTask.ID, subTask.Title)
		_, err = o.git.Commit(subTask.ID, commitMsg)
		if err != nil {
			log.Printf("❌ Sub-task %s failed: committing: %v", subTask.ID, err)
			telemetry.RecordError(taskSpan, err, "CommitFailed", "git")
			telemetry.SetTaskStatus(taskSpan, "failed")
			o.handleTaskFailure(subTask.ID, err.Error())
			return false
		}

		// Try to merge to main
		if err := o.git.MergeToMain(subTask.ID); err != nil {
			log.Printf("⚠️  Sub-task %s completed but merge failed: %v", subTask.ID, err)
			telemetry.RecordError(taskSpan, err, "MergeFailed", "git")
		}

		// Mark sub-task complete
		if err := o.store.CompleteTask(subTask.ID); err != nil {
			log.Printf("Error completing sub-task: %v", err)
		}

		duration := time.Since(start)
		log.Printf("✅ Completed sub-task %s in %v", subTask.ID, duration)

		// Record sub-task completion telemetry
		telemetry.SetTaskStatus(taskSpan, "completed")
		telemetry.RecordTaskCompleted(taskCtx, fmt.Sprintf("worker-%d", workerID), parentTask.EpicID, duration)
	}

	log.Printf("✅ All %d sub-tasks completed for %s", len(subTasks), parentTask.ID)
	return true
}

// handleTaskFailure increments attempts and either retries or marks as failed
// Returns true if the task was set to ready for retry (false if permanently failed)
func (o *Orchestrator) handleTaskFailure(taskID, errorMsg string) bool {
	// Fetch current task to check attempts before incrementing
	task, err := o.store.GetTask(taskID)
	if err != nil {
		log.Printf("Error fetching task %s: %v", taskID, err)
		_ = o.store.UpdateTaskStatus(taskID, types.TaskStatusFailed, errorMsg)
		dashboard.BroadcastTaskFailed(taskID, taskID, errorMsg)
		return false
	}

	// If the task has been cancelled, do not retry or override state.
	if task.Status == types.TaskStatusCancelled {
		log.Printf("🛑 Task %s is cancelled; not applying failure handling", taskID)
		return false
	}

	// Check if we've exceeded max attempts
	if task.Attempts >= task.MaxAttempts {
		_ = o.store.UpdateTaskStatus(taskID, types.TaskStatusFailed, errorMsg)
		log.Printf("❌ Task %s failed after %d attempts", taskID, task.Attempts)
		dashboard.BroadcastTaskFailed(task.ID, task.Title, errorMsg)
		return false
	}

	// Increment attempts in database
	if err := o.store.IncrementTaskAttempts(taskID); err != nil {
		log.Printf("Error incrementing attempts for task %s: %v", taskID, err)
		_ = o.store.UpdateTaskStatus(taskID, types.TaskStatusFailed, errorMsg)
		dashboard.BroadcastTaskFailed(task.ID, task.Title, errorMsg)
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
	log.Printf("📊 Progress: %d/%d tasks (%.1f%%) | Ready: %d | In Progress: %d | Blocked: %d | Failed: %d | Cancelled: %d",
		status.Completed, status.Total, progress,
		status.Ready, status.InProgress, status.Blocked, status.Failed, status.Cancelled)
}

// printFinalStatus prints final run results
func (o *Orchestrator) printFinalStatus(status *db.ProjectStatus) {
	fmt.Println("\n🐂 Drover Run Complete")
	fmt.Println("═════════════════════")
	fmt.Printf("\nTotal tasks:     %d", status.Total)
	fmt.Printf("\nCompleted:       %d", status.Completed)
	fmt.Printf("\nFailed:          %d", status.Failed)
	fmt.Printf("\nBlocked:         %d", status.Blocked)
	fmt.Printf("\nCancelled:       %d", status.Cancelled)

	if status.Total > 0 {
		successRate := float64(status.Completed) / float64(status.Total) * 100
		fmt.Printf("\n\nSuccess rate:    %.1f%%", successRate)
	}

	if status.Failed > 0 || status.Blocked > 0 || status.Cancelled > 0 {
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
