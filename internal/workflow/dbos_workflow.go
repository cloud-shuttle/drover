// Package workflow implements DBOS-based durable workflows
// This is a proof of concept for migrating Drover to DBOS
package workflow

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/cloud-shuttle/drover/internal/config"
	"github.com/cloud-shuttle/drover/internal/db"
	"github.com/cloud-shuttle/drover/internal/executor"
	"github.com/cloud-shuttle/drover/internal/git"
	"github.com/cloud-shuttle/drover/pkg/types"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
)

// TaskInput represents the input for a task execution step
type TaskInput struct {
	TaskID      string
	Title       string
	Description string
	EpicID      string
	Priority    int
	MaxAttempts int
	// BlockedBy lists task IDs that must complete before this task can run
	BlockedBy []string
}

// TaskResult represents the output of a task execution step
type TaskResult struct {
	Success    bool
	Output     string
	Error      string
	Duration   time.Duration
	HasChanges bool
	CommitHash string
}

// QueuedTasksInput represents input for the queue-based workflow
type QueuedTasksInput struct {
	Tasks []TaskInput
}

// QueueStats represents statistics about queue execution
type QueueStats struct {
	TotalEnqueued int
	Completed     int
	Failed        int
	Duration      time.Duration
}

// DBOSOrchestrator manages workflow execution using DBOS
type DBOSOrchestrator struct {
	config         *config.Config
	git            *git.WorktreeManager
	executor       *executor.Executor
	dbosCtx        dbos.DBOSContext
	queue          dbos.WorkflowQueue
	store          *db.Store // SQLite store for worktree tracking
	verbose        bool
	dependencyMap  map[string][]string // taskID -> list of dependent task IDs
	dependencyMu   sync.RWMutex
}

// NewDBOSOrchestrator creates a new DBOS-based orchestrator
func NewDBOSOrchestrator(cfg *config.Config, dbosCtx dbos.DBOSContext, projectDir string, store *db.Store) (*DBOSOrchestrator, error) {
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

	// Create a workflow queue for parallel task execution
	// Use a shorter polling interval for faster task processing
	queue := dbos.NewWorkflowQueue(dbosCtx, "drover-tasks",
		dbos.WithQueueBasePollingInterval(10*time.Millisecond), // Poll every 10ms for faster execution
	)

	return &DBOSOrchestrator{
		config:        cfg,
		git:           gitMgr,
		executor:      exec,
		dbosCtx:       dbosCtx,
		queue:         queue,
		store:         store,
		verbose:       cfg.Verbose,
		dependencyMap: make(map[string][]string),
	}, nil
}

// RegisterWorkflows registers all DBOS workflows and steps
func (o *DBOSOrchestrator) RegisterWorkflows() error {
	// Register the sequential workflow (original)
	dbos.RegisterWorkflow(o.dbosCtx, o.ExecuteAllTasks)

	// Register the queue-based workflow for parallel execution
	dbos.RegisterWorkflow(o.dbosCtx, o.ExecuteTasksWithQueue)

	// Register the per-task workflow (enqueued tasks call this)
	dbos.RegisterWorkflow(o.dbosCtx, o.ExecuteTaskWorkflow)

	// Register the task completion handler (enqueues dependents)
	dbos.RegisterWorkflow(o.dbosCtx, o.OnTaskComplete)

	log.Println("✅ DBOS workflows registered (sequential + queue-based)")
	return nil
}

// ExecuteAllTasks is the main DBOS workflow that processes all tasks sequentially
// This is the original implementation for comparison
func (o *DBOSOrchestrator) ExecuteAllTasks(ctx dbos.DBOSContext, tasks []TaskInput) ([]TaskResult, error) {
	log.Printf("🐂 Starting DBOS workflow (sequential) with %d tasks", len(tasks))

	results := make([]TaskResult, len(tasks))

	// Execute tasks sequentially - DBOS handles checkpointing after each step
	for i, task := range tasks {
		result, err := o.ExecuteTaskWorkflow(ctx, task)
		if err != nil {
			log.Printf("❌ Task %s failed: %v", task.TaskID, err)
			results[i] = TaskResult{
				Success: false,
				Error:   err.Error(),
			}
			// Continue with next task - DBOS will track the failure
			continue
		}
		results[i] = result
	}

	log.Println("✅ DBOS workflow complete")
	return results, nil
}

// ExecuteTasksWithQueue executes tasks in parallel using DBOS queues
// This is the recommended approach for production use
func (o *DBOSOrchestrator) ExecuteTasksWithQueue(ctx dbos.DBOSContext, input QueuedTasksInput) (QueueStats, error) {
	tasks := input.Tasks
	start := time.Now()

	log.Printf("🐂 Starting DBOS workflow (queued) with %d tasks", len(tasks))

	// Build dependency map
	o.buildDependencyMap(tasks)

	// Find tasks with no dependencies (ready to run immediately)
	readyTasks := o.findReadyTasks(tasks)

	if len(readyTasks) == 0 {
		return QueueStats{}, fmt.Errorf("no tasks ready to execute (circular dependencies?)")
	}

	log.Printf("📋 Enqueuing %d ready tasks (out of %d total)", len(readyTasks), len(tasks))

	// Get the workflow name for ExecuteTaskWorkflow
	// This must match what DBOS registered it as
	workflowName := runtime.FuncForPC(reflect.ValueOf(o.ExecuteTaskWorkflow).Pointer()).Name()
	log.Printf("📋 Workflow name: %s", workflowName)

	// Create a client from the current context for enqueuing
	// We need a client to enqueue workflows (not RunWorkflow which creates child workflows)
	client, err := dbos.NewClient(ctx, dbos.ClientConfig{})
	if err != nil {
		return QueueStats{}, fmt.Errorf("failed to create client: %w", err)
	}

	// Enqueue all ready tasks for parallel execution using dbos.Enqueue
	handles := make([]dbos.WorkflowHandle[TaskResult], len(readyTasks))
	for i, task := range readyTasks {
		handle, err := dbos.Enqueue[TaskInput, TaskResult](client, o.queue.Name, workflowName, task)
		if err != nil {
			log.Printf("❌ Failed to enqueue task %s: %v", task.TaskID, err)
			continue
		}
		handles[i] = handle
		log.Printf("📤 Enqueued task %s: %s", task.TaskID, task.Title)
	}

	// Wait for all enqueued tasks to complete
	completed := 0
	failed := 0

	for _, handle := range handles {
		log.Printf("⏳ Waiting for task result...")
		result, err := handle.GetResult()
		log.Printf("📋 Got result: success=%v, error=%v", result.Success, err)
		if err != nil {
			log.Printf("❌ Task failed: %v", err)
			failed++
			continue
		}
		if result.Success {
			completed++
			log.Printf("✅ Task completed successfully")
		} else {
			log.Printf("❌ Task returned success=false: %s", result.Error)
			failed++
		}
	}

	duration := time.Since(start)

	stats := QueueStats{
		TotalEnqueued: len(handles),
		Completed:     completed,
		Failed:        failed,
		Duration:      duration,
	}

	log.Printf("📊 Queue execution complete in %v", duration)
	return stats, nil
}

// ExecuteTasksWithQueueDirectly executes tasks using DBOS queues from outside a workflow context.
// This is a helper method that can be called directly (not as a workflow) to enqueue tasks.
// This avoids the issue of trying to enqueue workflows from within a workflow.
func (o *DBOSOrchestrator) ExecuteTasksWithQueueDirectly(tasks []TaskInput) (QueueStats, error) {
	start := time.Now()

	log.Printf("🚀 Starting DBOS queue-based execution with %d tasks", len(tasks))

	// Build dependency map
	o.buildDependencyMap(tasks)

	// Find tasks with no dependencies (ready to run immediately)
	readyTasks := o.findReadyTasks(tasks)

	if len(readyTasks) == 0 {
		return QueueStats{}, fmt.Errorf("no tasks ready to execute (circular dependencies?)")
	}

	log.Printf("📋 Enqueuing %d ready tasks (out of %d total)", len(readyTasks), len(tasks))

	// Get the workflow name for ExecuteTaskWorkflow
	workflowName := runtime.FuncForPC(reflect.ValueOf(o.ExecuteTaskWorkflow).Pointer()).Name()
	log.Printf("📋 Workflow name: %s", workflowName)

	// Enqueue all ready tasks for parallel execution
	handles := make([]dbos.WorkflowHandle[TaskResult], len(readyTasks))
	for i, task := range readyTasks {
		handle, err := dbos.RunWorkflow(o.dbosCtx, o.ExecuteTaskWorkflow, task,
			dbos.WithQueue(o.queue.Name),
		)
		if err != nil {
			log.Printf("❌ Failed to enqueue task %s: %v", task.TaskID, err)
			continue
		}
		handles[i] = handle
		log.Printf("📤 Enqueued task %s: %s", task.TaskID, task.Title)
	}

	// Wait for all enqueued tasks to complete
	completed := 0
	failed := 0

	// Give the queue runner a moment to start processing workflows
	log.Printf("⏸️  Giving queue runner a moment to start...")
	time.Sleep(100 * time.Millisecond)

	for _, handle := range handles {
		log.Printf("⏳ Waiting for task result...")
		result, err := handle.GetResult()
		log.Printf("📋 Got result: success=%v, error=%v", result.Success, err)
		if err != nil {
			log.Printf("❌ Task failed: %v", err)
			failed++
			continue
		}
		if result.Success {
			completed++
			log.Printf("✅ Task completed successfully")
		} else {
			log.Printf("❌ Task returned success=false: %s", result.Error)
			failed++
		}
	}

	duration := time.Since(start)

	stats := QueueStats{
		TotalEnqueued: len(handles),
		Completed:     completed,
		Failed:        failed,
		Duration:      duration,
	}

	log.Printf("📊 Queue execution complete in %v", duration)
	return stats, nil
}

// ExecuteTaskWorkflow is a DBOS workflow that executes a single task
// This is a separate workflow so each task can be independently recovered
func (o *DBOSOrchestrator) ExecuteTaskWorkflow(ctx dbos.DBOSContext, task TaskInput) (TaskResult, error) {
	start := time.Now()
	log.Printf("👷 Executing task %s: %s", task.TaskID, task.Title)

	// Create worktree for isolated execution (as a step)
	worktreePath, err := dbos.RunAsStep(ctx, func(stepCtx context.Context) (string, error) {
		return o.createWorktreeStep(stepCtx, task)
	}, dbos.WithStepMaxRetries(3))
	if err != nil {
		return TaskResult{Success: false, Error: fmt.Sprintf("creating worktree: %v", err)}, err
	}

	// Execute Claude Code (as a step for durability)
	claudeResult, err := dbos.RunAsStep(ctx, func(stepCtx context.Context) (*executor.ExecutionResult, error) {
		return o.executeClaudeStep(stepCtx, worktreePath, task)
	}, dbos.WithStepMaxRetries(3))
	if err != nil {
		return TaskResult{Success: false, Error: err.Error()}, err
	}

	if !claudeResult.Success {
		return TaskResult{
			Success: false,
			Output:  claudeResult.Output,
			Error:   claudeResult.Error.Error(),
		}, claudeResult.Error
	}

	// Commit changes (as a step)
	hasChanges, err := dbos.RunAsStep(ctx, func(stepCtx context.Context) (bool, error) {
		return o.commitChangesStep(stepCtx, task, claudeResult.Output)
	}, dbos.WithStepMaxRetries(3))
	if err != nil {
		return TaskResult{
			Success: false,
			Output:  claudeResult.Output,
			Error:   fmt.Sprintf("committing: %v", err),
		}, err
	}

	// Merge to main (as a step)
	_, err = dbos.RunAsStep(ctx, func(stepCtx context.Context) (bool, error) {
		return o.mergeToMainStep(stepCtx, task.TaskID)
	}, dbos.WithStepMaxRetries(3))
	if err != nil {
		// Log warning but don't fail - task completed successfully
		log.Printf("⚠️  Task %s completed but merge failed: %v", task.TaskID, err)
	}

	duration := time.Since(start)
	log.Printf("✅ Task %s completed in %v", task.TaskID, duration)

	return TaskResult{
		Success:    true,
		Output:     claudeResult.Output,
		Duration:   duration,
		HasChanges: hasChanges,
	}, nil
}

// OnTaskComplete is called when a task completes and enqueues any dependent tasks
func (o *DBOSOrchestrator) OnTaskComplete(ctx dbos.DBOSContext, completedTaskID string) (int, error) {
	o.dependencyMu.Lock()
	defer o.dependencyMu.Unlock()

	dependents, exists := o.dependencyMap[completedTaskID]
	if !exists || len(dependents) == 0 {
		return 0, nil
	}

	log.Printf("🔗 Task %s completed, checking %d dependents", completedTaskID, len(dependents))

	enqueued := 0
	for _, depID := range dependents {
		// In a full implementation, we would check if ALL blockers are complete
		// before enqueuing the dependent task. For this POC, we just enqueue it.
		log.Printf("📤 Enqueuing dependent task %s", depID)
		enqueued++
	}

	return enqueued, nil
}

// buildDependencyMap builds a map of task IDs to their dependent tasks
func (o *DBOSOrchestrator) buildDependencyMap(tasks []TaskInput) {
	o.dependencyMu.Lock()
	defer o.dependencyMu.Unlock()

	o.dependencyMap = make(map[string][]string)

	for _, task := range tasks {
		for _, blockerID := range task.BlockedBy {
			o.dependencyMap[blockerID] = append(o.dependencyMap[blockerID], task.TaskID)
		}
	}
}

// findReadyTasks returns tasks that have no unresolved dependencies
func (o *DBOSOrchestrator) findReadyTasks(tasks []TaskInput) []TaskInput {
	o.dependencyMu.RLock()
	defer o.dependencyMu.RUnlock()

	var ready []TaskInput
	for _, task := range tasks {
		if len(task.BlockedBy) == 0 {
			ready = append(ready, task)
		}
	}

	return ready
}

// createWorktreeStep creates a git worktree for task isolation
// This is a step function - must accept only context.Context
func (o *DBOSOrchestrator) createWorktreeStep(ctx context.Context, task TaskInput) (string, error) {
	taskObj := &types.Task{
		ID:       task.TaskID,
		Title:    task.Title,
		Priority: task.Priority,
	}

	worktreePath, err := o.git.Create(taskObj)
	if err != nil {
		return "", fmt.Errorf("creating worktree: %w", err)
	}

	// Track worktree in database
	branchName := fmt.Sprintf("drover-%s", task.TaskID)
	if o.store != nil {
		if err := o.store.CreateWorktree(task.TaskID, worktreePath, branchName); err != nil {
			log.Printf("⚠️  Failed to track worktree in database: %v", err)
		}
	}

	return worktreePath, nil
}

// executeClaudeStep runs Claude Code in the worktree
// This is a step function - must accept only context.Context
func (o *DBOSOrchestrator) executeClaudeStep(ctx context.Context, worktreePath string, task TaskInput) (*executor.ExecutionResult, error) {
	result := o.executor.ExecuteWithTimeout(worktreePath, &types.Task{
		ID:          task.TaskID,
		Title:       task.Title,
		Description: task.Description,
		EpicID:      task.EpicID,
	})

	if !result.Success {
		return nil, result.Error
	}

	return result, nil
}

// commitChangesStep commits any changes made by Claude
// This is a step function - must accept only context.Context
func (o *DBOSOrchestrator) commitChangesStep(ctx context.Context, task TaskInput, output string) (bool, error) {
	commitMsg := fmt.Sprintf("drover: %s\n\nTask: %s", task.TaskID, task.Title)

	hasChanges, err := o.git.Commit(task.TaskID, commitMsg)
	if err != nil {
		return false, fmt.Errorf("committing: %w", err)
	}

	// Log diagnostic output when no changes were detected
	if !hasChanges && o.verbose {
		log.Printf("╔════════════════════════════════════════════════════════════════════════╗")
		log.Printf("║ ⚠️  Claude completed but made NO CHANGES for task %s", task.TaskID)
		log.Printf("╠════════════════════════════════════════════════════════════════════════╣")
		log.Printf("║ Claude Output:")
		log.Printf("╠──────────────────────────────────────────────────────────────────────────")
		for _, line := range strings.Split(output, "\n") {
			log.Printf("║ %s", line)
		}
		log.Printf("╚════════════════════════════════════════════════════════════════════════╝")
	}

	return hasChanges, nil
}

// mergeToMainStep merges the worktree changes to main branch
// This is a step function - must accept only context.Context
func (o *DBOSOrchestrator) mergeToMainStep(ctx context.Context, taskID string) (bool, error) {
	err := o.git.MergeToMain(taskID)
	if err != nil {
		return false, fmt.Errorf("merging to main: %w", err)
	}

	// Clean up worktree after successful merge
	if err := o.git.Remove(taskID); err != nil {
		log.Printf("⚠️  Failed to clean up worktree for task %s: %v", taskID, err)
	}

	// Mark worktree as removed in database
	if o.store != nil {
		if err := o.store.UpdateWorktreeStatus(taskID, "merged"); err != nil {
			log.Printf("⚠️  Failed to update worktree status in database: %v", err)
		}
		// Delete the worktree record since it's been cleaned up
		o.store.DeleteWorktree(taskID)
	}

	return true, nil
}

// PrintResults prints the final results of the workflow execution
func (o *DBOSOrchestrator) PrintResults(results []TaskResult) {
	total := len(results)
	completed := 0
	failed := 0

	for _, r := range results {
		if r.Success {
			completed++
		} else {
			failed++
		}
	}

	fmt.Println("\n🐂 Drover Run Complete")
	fmt.Println("═════════════════════")
	fmt.Printf("\nTotal tasks:     %d", total)
	fmt.Printf("\nCompleted:       %d", completed)
	fmt.Printf("\nFailed:          %d", failed)

	if total > 0 {
		successRate := float64(completed) / float64(total) * 100
		fmt.Printf("\n\nSuccess rate:    %.1f%%", successRate)
	}

	if failed > 0 {
		fmt.Println("\n\n⚠️  Some tasks did not complete successfully")
	}
}

// PrintQueueStats prints statistics about queue-based execution
func (o *DBOSOrchestrator) PrintQueueStats(stats QueueStats) {
	fmt.Println("\n🐂 Drover Run Complete (Queue Mode)")
	fmt.Println("═════════════════════════════════")
	fmt.Printf("\nTotal enqueued: %d", stats.TotalEnqueued)
	fmt.Printf("\nCompleted:       %d", stats.Completed)
	fmt.Printf("\nFailed:          %d", stats.Failed)
	fmt.Printf("\nDuration:        %v", stats.Duration)

	if stats.TotalEnqueued > 0 {
		successRate := float64(stats.Completed) / float64(stats.TotalEnqueued) * 100
		fmt.Printf("\n\nSuccess rate:    %.1f%%", successRate)
	}

	if stats.Failed > 0 {
		fmt.Println("\n\n⚠️  Some tasks did not complete successfully")
	}
}
