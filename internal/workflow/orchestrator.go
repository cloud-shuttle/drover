// Package workflow implements durable workflows using DBOS
package workflow

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"sync"
	"time"

	"github.com/cloud-shuttle/drover/internal/config"
	"github.com/cloud-shuttle/drover/internal/db"
	"github.com/cloud-shuttle/drover/internal/executor"
	"github.com/cloud-shuttle/drover/internal/git"
	"github.com/cloud-shuttle/drover/pkg/types"
)

// Orchestrator manages the main execution loop
type Orchestrator struct {
	config     *config.Config
	store      *db.Store
	git        *git.WorktreeManager
	executor   *executor.Executor
	workerPool chan *workerJob
	workers    int
}

type workerJob struct {
	TaskID   string
	Task     *types.Task
	ResultCh chan *workerResult
}

type workerResult struct {
	TaskID    string
	Success   bool
	Error     string
	Duration  time.Duration
}

// NewOrchestrator creates a new workflow orchestrator
func NewOrchestrator(cfg *config.Config, store *db.Store, projectDir string) (*Orchestrator, error) {
	gitMgr := git.NewWorktreeManager(
		projectDir,
		filepath.Join(projectDir, cfg.WorktreeDir),
	)

	exec := executor.NewExecutor(cfg.ClaudePath, cfg.TaskTimeout)

	// Check Claude is installed
	if err := executor.CheckClaudeInstalled(cfg.ClaudePath); err != nil {
		return nil, fmt.Errorf("checking claude: %w", err)
	}

	return &Orchestrator{
		config:     cfg,
		store:      store,
		git:        gitMgr,
		executor:   exec,
		workerPool: make(chan *workerJob, cfg.Workers),
		workers:    cfg.Workers,
	}, nil
}

// Run executes all tasks to completion
func (o *Orchestrator) Run(ctx context.Context) error {
	log.Printf("🐂 Starting Drover with %d workers", o.workers)

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < o.workers; i++ {
		wg.Add(1)
		go o.worker(ctx, i, &wg)
	}

	// Main orchestration loop
	ticker := time.NewTicker(o.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("🛑 Context cancelled, stopping...")
			o.workerPool <- nil // Signal workers to stop
			wg.Wait()
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
				o.workerPool <- nil // Signal workers to stop
				wg.Wait()
				o.printFinalStatus(status)
				return nil
			}

			// Enqueue ready tasks
			if err := o.enqueueReadyTasks(); err != nil {
				log.Printf("Error enqueuing tasks: %v", err)
			}

			// Print progress
			o.printProgress(status)
		}
	}
}

// enqueueReadyTasks claims and enqueues ready tasks
func (o *Orchestrator) enqueueReadyTasks() error {
	for {
		workerID := fmt.Sprintf("worker-%d", time.Now().UnixNano())
		task, err := o.store.ClaimTask(workerID)
		if err != nil {
			return fmt.Errorf("claiming task: %w", err)
		}
		if task == nil {
			break // No more tasks to claim
		}

		log.Printf("📤 Enqueued task %s: %s", task.ID, task.Title)

		job := &workerJob{
			TaskID:   task.ID,
			Task:     task,
			ResultCh: make(chan *workerResult, 1),
		}

		select {
		case o.workerPool <- job:
			// Job sent to worker pool
		default:
			// Pool full, will retry next cycle
			_ = o.store.UpdateTaskStatus(task.ID, types.TaskStatusReady, "")
			log.Printf("⚠️  Worker pool full, task %s returned to ready", task.ID)
			return nil
		}
	}

	return nil
}

// worker processes tasks from the job queue
func (o *Orchestrator) worker(ctx context.Context, id int, wg *sync.WaitGroup) {
	defer wg.Done()

	log.Printf("👷 Worker %d started", id)

	for {
		select {
		case <-ctx.Done():
			log.Printf("👷 Worker %d stopping (context cancelled)", id)
			return

		case job := <-o.workerPool:
			if job == nil {
				log.Printf("👷 Worker %d stopping (shutdown signal)", id)
				return
			}

			o.executeTask(id, job)
		}
	}
}

// executeTask executes a single task
func (o *Orchestrator) executeTask(workerID int, job *workerJob) {
	task := job.Task
	start := time.Now()

	log.Printf("👷 Worker %d executing task %s: %s", workerID, task.ID, task.Title)

	// Update to in_progress
	if err := o.store.UpdateTaskStatus(task.ID, types.TaskStatusInProgress, ""); err != nil {
		log.Printf("Error updating task status: %v", err)
	}

	// Create worktree
	worktreePath, err := o.git.Create(task)
	if err != nil {
		o.handleTaskFailureWithMergeConflict(job, fmt.Errorf("creating worktree: %w", err), false)
		return
	}
	defer o.git.Remove(task.ID)

	// Execute Claude Code
	if err := o.executor.ExecuteWithTimeout(worktreePath, task); err != nil {
		o.handleTaskFailureWithMergeConflict(job, fmt.Errorf("claude execution: %w", err), false)
		return
	}

	// Commit changes
	commitMsg := fmt.Sprintf("drover: %s\n\nTask: %s", task.ID, task.Title)
	if err := o.git.Commit(task.ID, commitMsg); err != nil {
		o.handleTaskFailureWithMergeConflict(job, fmt.Errorf("committing: %w", err), false)
		return
	}

	// Merge to main
	if err := o.git.MergeToMain(task.ID); err != nil {
		o.handleTaskFailureWithMergeConflict(job, fmt.Errorf("merging: %w", err), true)
		return
	}

	// Mark complete and unblock dependents
	if err := o.store.CompleteTask(task.ID); err != nil {
		log.Printf("Error completing task: %v", err)
	}

	duration := time.Since(start)
	log.Printf("✅ Worker %d completed task %s in %v", workerID, task.ID, duration)

	job.ResultCh <- &workerResult{
		TaskID:   task.ID,
		Success:  true,
		Duration: duration,
	}
}

// handleTaskFailureWithMergeConflict handles task execution failures
func (o *Orchestrator) handleTaskFailureWithMergeConflict(job *workerJob, err error, mergeConflict bool) {
	task := job.Task

	log.Printf("❌ Task %s failed: %v", task.ID, err)

	// Check if we should retry
	if task.Attempts < task.MaxAttempts {
		task.Attempts++
		_ = o.store.UpdateTaskStatus(task.ID, types.TaskStatusReady, err.Error())
		log.Printf("🔄 Task %s retrying (attempt %d/%d)", task.ID, task.Attempts, task.MaxAttempts)
		return
	}

	// Max retries exceeded
	status := types.TaskStatusFailed
	if mergeConflict {
		status = types.TaskStatusBlocked
	}

	_ = o.store.UpdateTaskStatus(task.ID, status, err.Error())

	job.ResultCh <- &workerResult{
		TaskID:  task.ID,
		Success: false,
		Error:   err.Error(),
	}
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
