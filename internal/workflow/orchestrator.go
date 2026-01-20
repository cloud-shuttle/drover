// Package workflow implements durable workflows using DBOS
//
// DEPRECATED: This file contains the legacy SQLite-based Orchestrator.
// Drover now uses DBOS by default for both SQLite and PostgreSQL modes.
// See dbos_workflow.go for the current DBOS-based implementation.
// This file is kept for backwards compatibility and testing.
package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cloud-shuttle/drover/internal/analytics"
	"github.com/cloud-shuttle/drover/internal/backpressure"
	"github.com/cloud-shuttle/drover/internal/beads"
	"github.com/cloud-shuttle/drover/internal/config"
	ctxmngr "github.com/cloud-shuttle/drover/internal/context"
	"github.com/cloud-shuttle/drover/internal/dashboard"
	"github.com/cloud-shuttle/drover/internal/db"
	"github.com/cloud-shuttle/drover/internal/events"
	outcomepkg "github.com/cloud-shuttle/drover/internal/outcome"
	"github.com/cloud-shuttle/drover/internal/executor"
	"github.com/cloud-shuttle/drover/internal/git"
	"github.com/cloud-shuttle/drover/internal/project"
	"github.com/cloud-shuttle/drover/internal/testing"
	"github.com/cloud-shuttle/drover/internal/webhooks"
	"github.com/cloud-shuttle/drover/pkg/telemetry"
	"github.com/cloud-shuttle/drover/pkg/types"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// Orchestrator manages the main execution loop
type Orchestrator struct {
	config        *config.Config
	store         *db.Store
	git           *git.WorktreeManager
	pool          *git.WorktreePool // Worktree pool for pre-warming
	agent         executor.Agent // Agent interface for Claude/Codex/Amp
	workers       int
	verbose       bool // Enable verbose logging
	projectDir    string // Project directory for beads sync
	epicID        string // Optional epic filter for task execution
	webhooks      *webhooks.Manager // Webhook notification manager
	analytics     *analytics.Manager // Analytics manager
	backpressure  *backpressure.Controller // Backpressure controller for adaptive concurrency
	shutdownCtx   context.Context // Context for shutdown signal
	shutdownFunc  context.CancelFunc // Function to cancel shutdown context
}

// NewOrchestrator creates a new workflow orchestrator
func NewOrchestrator(cfg *config.Config, store *db.Store, projectDir string) (*Orchestrator, error) {
	gitMgr := git.NewWorktreeManager(
		projectDir,
		filepath.Join(projectDir, cfg.WorktreeDir),
	)
	gitMgr.SetVerbose(cfg.Verbose)

	// Initialize worktree pool if enabled
	var pool *git.WorktreePool
	if cfg.PoolEnabled {
		poolConfig := &git.PoolConfig{
			MinSize:         cfg.PoolMinSize,
			MaxSize:         cfg.PoolMaxSize,
			WarmupTimeout:   cfg.PoolWarmup,
			CleanupOnExit:   cfg.PoolCleanupOnExit,
			EnableSymlinks:  true,
			GoModCache:      true,
		}
		pool = git.NewWorktreePool(gitMgr, poolConfig)
		if err := pool.Start(); err != nil {
			return nil, fmt.Errorf("starting worktree pool: %w", err)
		}
	}

	// Load project configuration
	projectCfg, err := project.Load(projectDir)
	if err != nil {
		if pool != nil {
			pool.Stop()
		}
		return nil, fmt.Errorf("loading project config: %w", err)
	}

	// Validate project config
	if err := projectCfg.Validate(); err != nil {
		log.Printf("[project] warning: %v", err)
	}

	// Merge project config with global config
	projectCfg.MergeWithGlobal(cfg.AgentType, cfg.Workers, cfg.TaskTimeout, cfg.MaxTaskAttempts)

	// Create the agent based on configuration with project guidelines
	agentType := projectCfg.Agent
	// Use worker subprocess if configured for process isolation
	if cfg.UseWorkerSubprocess {
		agentType = "worker"
		if cfg.Verbose {
			log.Printf("[worker] using drover-worker subprocess for process isolation")
		}
	}

	agent, err := executor.NewAgent(&executor.AgentConfig{
		Type:              agentType,
		Path:              cfg.AgentPath,
		Timeout:           projectCfg.TaskTimeout,
		Verbose:           cfg.Verbose,
		ProjectGuidelines: projectCfg.GetGuidelines(),
		WorkerBinary:      cfg.WorkerBinary,
		WorkerMemoryLimit: cfg.WorkerMemoryLimit,
		ContextThresholds: &ctxmngr.ContentThresholds{
			MaxDescriptionSize: projectCfg.MaxDescriptionSize,
			MaxDiffSize:       projectCfg.MaxDiffSize,
			MaxFileSize:       projectCfg.MaxFileSize,
		},
	})
	if err != nil {
		if pool != nil {
			pool.Stop()
		}
		return nil, fmt.Errorf("creating agent: %w", err)
	}

	// Log project config
	if projectCfg.GetGuidelines() != "" {
		log.Printf("[project] loaded guidelines from %s", projectCfg.ConfigPath())
	}
	if projectCfg.HasLabels() {
		log.Printf("[project] default labels: %v", projectCfg.GetLabels())
	}

	// Create webhook manager (will be started in Run())
	webhookMgr := cfg.CreateWebhookManager()

	// Create analytics manager (will be started in Run())
	analyticsMgr, _ := cfg.CreateAnalyticsManager()

	// Create backpressure controller if enabled
	var backpressureCtrl *backpressure.Controller
	if cfg.BackpressureEnabled {
		backpressureCfg := backpressure.ControllerConfig{
			InitialConcurrency: cfg.BackpressureInitialConcurrency,
			MinConcurrency:     cfg.BackpressureMinConcurrency,
			MaxConcurrency:     cfg.BackpressureMaxConcurrency,
			RateLimitBackoff:   cfg.BackpressureRateLimitBackoff,
			MaxBackoff:         cfg.BackpressureMaxBackoff,
			SlowThreshold:      cfg.BackpressureSlowThreshold,
			SlowCountThreshold: 3,

			// Memory-aware settings (drover-mem-6)
			MemoryAwareEnabled:   cfg.BackpressureMemoryAwareEnabled,
			MemoryThresholdMB:    cfg.BackpressureMemoryThresholdMB,
			MemoryCriticalMB:     cfg.BackpressureMemoryCriticalMB,
			WorkerRSSLimitMB:     cfg.BackpressureWorkerRSSLimitMB,
		}
		backpressureCtrl = backpressure.NewController(backpressureCfg)
		if cfg.Verbose {
			log.Printf("[backpressure] enabled with initial concurrency: %d, max: %d",
				backpressureCfg.InitialConcurrency, backpressureCfg.MaxConcurrency)
			if backpressureCfg.MemoryAwareEnabled {
				log.Printf("[backpressure] memory-aware: threshold=%dMB, critical=%dMB, worker_limit=%dMB",
					backpressureCfg.MemoryThresholdMB, backpressureCfg.MemoryCriticalMB, backpressureCfg.WorkerRSSLimitMB)
			}
		}
	}

	// Check agent is installed
	if err := agent.CheckInstalled(); err != nil {
		if pool != nil {
			pool.Stop()
		}
		return nil, fmt.Errorf("checking %s: %w", cfg.AgentType, err)
	}

	orch := &Orchestrator{
		config:       cfg,
		store:        store,
		git:          gitMgr,
		pool:         pool,
		agent:        agent,
		workers:      cfg.Workers,
		verbose:      cfg.Verbose,
		projectDir:   projectDir,
		webhooks:     webhookMgr,
		analytics:    analyticsMgr,
		backpressure: backpressureCtrl,
	}

	// Create shutdown context for graceful shutdown
	orch.shutdownCtx, orch.shutdownFunc = context.WithCancel(context.Background())

	// Setup signal handlers for graceful shutdown
	orch.setupSignalHandlers()

	return orch, nil
}

// setupSignalHandlers installs signal handlers for graceful shutdown
func (o *Orchestrator) setupSignalHandlers() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Printf("Received signal %v, initiating graceful shutdown...", sig)
		o.shutdownFunc()
	}()
}

// SetEpicFilter sets the epic filter for task execution
// Only tasks belonging to the specified epic will be executed
func (o *Orchestrator) SetEpicFilter(epicID string) {
	o.epicID = epicID
}

// Run executes all tasks to completion
func (o *Orchestrator) Run(ctx context.Context) error {
	log.Printf("🐂 Starting Drover with %d workers", o.workers)
	if o.pool != nil && o.pool.IsEnabled() {
		log.Printf("🚀 Worktree pool enabled (min=%d, max=%d)", o.config.PoolMinSize, o.config.PoolMaxSize)
	}
	if o.epicID != "" {
		log.Printf("🎯 Filtering to epic: %s", o.epicID)
	}

	// Merge context with shutdown context for graceful signal handling
	// When either context is cancelled, the merged context is cancelled
	mergedCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		select {
		case <-mergedCtx.Done():
			// Already cancelled, nothing to do
		case <-o.shutdownCtx.Done():
			// Shutdown context cancelled, cancel merged context
			cancel()
		}
	}()

	// Start workflow span for telemetry
	_, workflowSpan := telemetry.StartWorkflowSpan(mergedCtx, telemetry.SpanWorkflowRun, "")
	defer workflowSpan.End()

	// Start webhook manager
	if o.webhooks != nil && o.config.WebhooksEnabled {
		o.webhooks.Start(o.config.WebhookWorkers)
		defer func() {
			stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = o.webhooks.Stop(stopCtx)
		}()
	}

	// Analytics manager starts automatically on creation
	// Just ensure it stops on exit
	if o.analytics != nil {
		defer func() {
			stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = o.analytics.Stop(stopCtx)
		}()
	}

	// Recover orphaned tasks from previous crashes
	if err := o.recoverOrphanedTasks(); err != nil {
		log.Printf("[recovery] warning: failed to recover orphaned tasks: %v", err)
	}

	// Ensure pool is stopped when we exit
	if o.pool != nil {
		defer o.pool.Stop()
	}

	// Start workers - they will claim tasks independently
	var wg sync.WaitGroup
	for i := 0; i < o.workers; i++ {
		wg.Add(1)
		go o.worker(mergedCtx, i, &wg)
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

	workerID := fmt.Sprintf("worker-%d", id)
	log.Printf("👷 Worker %d started", id)
	if o.webhooks != nil {
		o.webhooks.EmitWorkerStarted(workerID, id)
	}

	for {
		select {
		case <-ctx.Done():
			log.Printf("👷 Worker %d stopping (context cancelled)", id)
			if o.webhooks != nil {
				o.webhooks.EmitWorkerStopped(workerID, id, 0)
			}
			return
		default:
			// Check backpressure controller before claiming
			if o.backpressure != nil && !o.backpressure.CanSpawn() {
				// In backoff period, wait and retry
				stats := o.backpressure.GetStats()
				if o.verbose {
					log.Printf("[backpressure] worker %d waiting: backoff until %v (in-flight: %d/%d)",
						id, stats.BackoffUntil.Format("15:04:05"), stats.CurrentInFlight, stats.MaxInFlight)
				}
				time.Sleep(time.Second)
				continue
			}

			// Try to claim a task (filtered by epic if set)
			workerID := fmt.Sprintf("worker-%d-%d", id, time.Now().UnixNano())
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

			// Track worker started in backpressure controller
			if o.backpressure != nil {
				o.backpressure.WorkerStarted()
			}

			// Broadcast task claimed to dashboard
			dashboard.BroadcastTaskClaimed(task.ID, task.Title, workerID)

			// Emit webhook event
			if o.webhooks != nil {
				o.webhooks.EmitTaskClaimed(task.ID, task.Title, workerID)
			}

			// Execute the task
			o.executeTask(id, task)

			// Track worker finished in backpressure controller
			if o.backpressure != nil {
				o.backpressure.WorkerFinished()
			}
		}
	}
}

// executeTask executes a single task
func (o *Orchestrator) executeTask(workerID int, task *types.Task) {
	start := time.Now()
	taskCompleted := false

	log.Printf("👷 Worker %d executing task %s: %s", workerID, task.ID, task.Title)

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
		telemetry.TaskAttrs(task.ID, task.Title, "in_progress", string(task.Type), task.Priority, task.Attempts)...)

	workerIDStr := fmt.Sprintf("worker-%d", workerID)
	telemetry.RecordTaskClaimed(taskCtx, workerIDStr, o.epicID)
	defer taskSpan.End()

	// Create checkpoint for crash recovery
	checkpoint := &types.TaskCheckpoint{
		TaskID:        task.ID,
		State:         types.TaskStatusInProgress,
		WorkerPID:     os.Getpid(),
		StartedAt:     time.Now().Unix(),
		LastHeartbeat: time.Now().Unix(),
		Attempt:       task.Attempts + 1,
	}
	if err := o.store.CreateCheckpoint(checkpoint); err != nil {
		log.Printf("[checkpoint] warning: failed to create checkpoint for %s: %v", task.ID, err)
	}
	defer func() {
		// Complete/cleanup checkpoint when done
		if taskCompleted {
			verdict := types.TaskVerdictPass
			if task.Status != types.TaskStatusCompleted {
				verdict = types.TaskVerdictFail
			}
			_ = o.store.CompleteCheckpoint(task.ID, verdict, task.VerdictReason)
		}
	}()

	// Update to in_progress
	if err := o.store.UpdateTaskStatus(task.ID, types.TaskStatusInProgress, ""); err != nil {
		log.Printf("Error updating task status: %v", err)
	}

	// Record claimed event
	o.recordEvent(events.EventTaskClaimed, task.ID, task.EpicID, map[string]any{
		"worker": workerIDStr,
		"title":  task.Title,
	})

	// Broadcast task started to dashboard
	dashboard.BroadcastTaskStarted(task.ID, task.Title, workerIDStr)

	// Emit webhook event
	if o.webhooks != nil {
		o.webhooks.EmitTaskStarted(task.ID, task.Title, workerIDStr)
	}

	// Record event
	o.recordEvent(events.EventTaskStarted, task.ID, task.EpicID, map[string]any{
		"worker": workerIDStr,
		"title":  task.Title,
	})

	// Start analytics tracking
	if o.analytics != nil {
		o.analytics.StartTask(task.ID, task.Title, o.config.AgentType, "")
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

	// Create worktree (use pool if enabled)
	var worktreePath string
	var worktreeCleanupNeeded = true
	if o.pool != nil && o.pool.IsEnabled() {
		worktreePath, err = o.pool.Acquire(task.ID)
		if err != nil {
			log.Printf("❌ Task %s failed: acquiring worktree from pool: %v", task.ID, err)
			telemetry.RecordError(taskSpan, err, "WorktreeAcquireFailed", "pool")
			telemetry.SetTaskStatus(taskSpan, "failed")
			if o.handleTaskFailure(task.ID, err.Error()) {
				taskCompleted = true // Task set to ready for retry
			}
			return
		}
		defer func() {
			if worktreeCleanupNeeded {
				o.pool.Release(task.ID, false) // Don't retain worktree after task completion
			}
		}()
	} else {
		// Check if worktree already exists (from a paused task)
		existingPath, err := o.git.GetWorktreePath(task.ID)
		if err == nil && existingPath != "" {
			worktreePath = existingPath
			log.Printf("♻️  Reusing existing worktree for task %s at %s", task.ID, worktreePath)
		} else {
			worktreePath, err = o.git.Create(task)
			if err != nil {
				log.Printf("❌ Task %s failed: creating worktree: %v", task.ID, err)
				telemetry.RecordError(taskSpan, err, "WorktreeCreationFailed", "git")
				telemetry.SetTaskStatus(taskSpan, "failed")
				if o.handleTaskFailure(task.ID, err.Error()) {
					taskCompleted = true // Task set to ready for retry
				}
				return
			}
		}
		defer func() {
			if worktreeCleanupNeeded {
				o.git.Remove(task.ID)
			}
		}()
	}

	// Fetch pending guidance and set on task execution context
	guidance, err := o.store.GetPendingGuidance(task.ID)
	if err != nil {
		log.Printf("Error fetching guidance: %v", err)
		// Continue without guidance rather than failing
	}
	if len(guidance) > 0 {
		log.Printf("💡 Found %d pending guidance messages for task %s", len(guidance), task.ID)
		task.ExecutionContext = &types.TaskExecutionContext{
			Guidance: guidance,
		}
		// Track guidance IDs for marking as delivered later
		var guidanceIDs []string
		for _, g := range guidance {
			guidanceIDs = append(guidanceIDs, g.ID)
		}
		defer func() {
			// Mark guidance as delivered after execution completes
			if err := o.store.MarkGuidanceDelivered(guidanceIDs); err != nil {
				log.Printf("Error marking guidance delivered: %v", err)
			}
		}()
	}

	// Fetch recent completed tasks for context carrying (if enabled)
	taskContextCount := o.getProjectTaskContextCount()
	if taskContextCount > 0 {
		maxAgeSeconds := int64(o.getProjectTaskContextMaxAge().Seconds())
		recentTasks, err := o.store.GetRecentCompletedTasks(task.EpicID, taskContextCount, maxAgeSeconds)
		if err != nil {
			log.Printf("Warning: failed to fetch recent tasks for context: %v", err)
		} else if len(recentTasks) > 0 {
			log.Printf("📚 Loaded %d recent tasks for context", len(recentTasks))
			o.agent.SetTaskContext(recentTasks, taskContextCount)
		}
	}

	// Execute Claude Code and capture the result
	result := o.agent.ExecuteWithContext(taskCtx, worktreePath, task, taskSpan)

	// Report signal to backpressure controller
	if o.backpressure != nil {
		o.backpressure.OnWorkerSignal(result.Signal)
	}

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

	// Run automated tests before task completion
	if err := o.runTests(task.ID, worktreePath, taskSpan); err != nil {
		log.Printf("❌ Task %s failed automated tests: %v", task.ID, err)
		telemetry.RecordError(taskSpan, err, "TestExecutionFailed", "tests")
		telemetry.SetTaskStatus(taskSpan, "failed")
		if o.handleTaskFailure(task.ID, err.Error()) {
			taskCompleted = true // Task set to ready for retry
		}
		return
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

	// Emit webhook event
	if o.webhooks != nil {
		o.webhooks.EmitTaskCompleted(task.ID, task.Title, duration.Milliseconds())
	}

	// Record event
	o.recordEvent(events.EventTaskCompleted, task.ID, task.EpicID, map[string]any{
		"worker":   workerIDStr,
		"title":    task.Title,
		"duration": duration.Milliseconds(),
	})

	// Parse and store structured outcome
	outcome := outcomepkg.ParseOutput(claudeOutput)
	if err := o.store.SetTaskVerdict(task.ID, types.TaskVerdict(outcome.Verdict), outcome.Summary); err != nil {
		log.Printf("Error storing verdict for task %s: %v", task.ID, err)
	}

	// End analytics tracking
	if o.analytics != nil {
		o.analytics.EndTask(task.ID, "success", "")
	}

	// Record task completion telemetry
	telemetry.SetTaskStatus(taskSpan, "completed")
	telemetry.RecordTaskCompleted(taskCtx, workerIDStr, o.epicID, string(task.Type), duration)
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

		// Check if sub-task has its own sub-tasks (shouldn't happen with max depth 2)
		hasChildren, _ := o.store.HasSubTasks(subTask.ID)
		if hasChildren {
			log.Printf("❌ Sub-task %s has children (max depth exceeded)", subTask.ID)
			o.store.UpdateTaskStatus(subTask.ID, types.TaskStatusFailed, "Max depth exceeded")
			return false
		}

		// Create worktree for sub-task (use pool if enabled)
		var worktreePath string
		if o.pool != nil && o.pool.IsEnabled() {
			worktreePath, err = o.pool.Acquire(subTask.ID)
			if err != nil {
				log.Printf("❌ Sub-task %s failed: acquiring worktree from pool: %v", subTask.ID, err)
				o.handleTaskFailure(subTask.ID, err.Error())
				return false
			}
		} else {
			worktreePath, err = o.git.Create(subTask)
			if err != nil {
				log.Printf("❌ Sub-task %s failed: creating worktree: %v", subTask.ID, err)
				o.handleTaskFailure(subTask.ID, err.Error())
				return false
			}
		}

		// Fetch pending guidance for sub-task
		guidance, gErr := o.store.GetPendingGuidance(subTask.ID)
		if gErr != nil {
			log.Printf("Error fetching guidance: %v", gErr)
			// Continue without guidance rather than failing
		}
		if len(guidance) > 0 {
			log.Printf("💡 Found %d pending guidance messages for sub-task %s", len(guidance), subTask.ID)
			subTask.ExecutionContext = &types.TaskExecutionContext{
				Guidance: guidance,
			}
			// Track guidance IDs for marking as delivered later
			var guidanceIDs []string
			for _, g := range guidance {
				guidanceIDs = append(guidanceIDs, g.ID)
			}
			defer func() {
				// Mark guidance as delivered after execution completes
				if err := o.store.MarkGuidanceDelivered(guidanceIDs); err != nil {
					log.Printf("Error marking guidance delivered: %v", err)
				}
			}()
		}

		// Execute sub-task
		start := time.Now()
		taskCtx, taskSpan := telemetry.StartTaskSpan(context.Background(),
			telemetry.SpanTaskExecute,
			telemetry.TaskAttrs(subTask.ID, subTask.Title, "in_progress", string(subTask.Type), subTask.Priority, subTask.Attempts)...)
		telemetry.RecordTaskClaimed(taskCtx, fmt.Sprintf("worker-%d", workerID), parentTask.EpicID)
		defer taskSpan.End()

		result := o.agent.ExecuteWithContext(taskCtx, worktreePath, subTask, taskSpan)

		// Report signal to backpressure controller
		if o.backpressure != nil {
			o.backpressure.OnWorkerSignal(result.Signal)
		}

		// Clean up worktree
		if o.pool != nil && o.pool.IsEnabled() {
			o.pool.Release(subTask.ID, false)
		} else {
			o.git.Remove(subTask.ID)
		}

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
		telemetry.RecordTaskCompleted(taskCtx, fmt.Sprintf("worker-%d", workerID), parentTask.EpicID, string(subTask.Type), duration)
	}

	log.Printf("✅ All %d sub-tasks completed for %s", len(subTasks), parentTask.ID)
	return true
}

// recordEvent records an event in the database
func (o *Orchestrator) recordEvent(eventType events.EventType, taskID, epicID string, data map[string]any) {
	eventID := uuid.New().String()
	timestamp := time.Now().Unix()

	// Marshal event data to JSON
	var dataJSON string
	if len(data) > 0 {
		if bytes, err := json.Marshal(data); err == nil {
			dataJSON = string(bytes)
		}
	}

	if err := o.store.RecordEvent(eventID, string(eventType), timestamp, taskID, epicID, dataJSON); err != nil {
		log.Printf("Error recording event: %v", err)
	}
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
		if o.webhooks != nil {
			o.webhooks.EmitTaskFailed(taskID, taskID, errorMsg, task.Attempts)
		}
		if o.analytics != nil {
			o.analytics.EndTask(taskID, "failed", errorMsg)
		}
		return false
	}

	// Check if we've exceeded max attempts
	if task.Attempts >= task.MaxAttempts {
		_ = o.store.UpdateTaskStatus(taskID, types.TaskStatusFailed, errorMsg)
		log.Printf("❌ Task %s failed after %d attempts", taskID, task.Attempts)
		dashboard.BroadcastTaskFailed(task.ID, task.Title, errorMsg)
		if o.webhooks != nil {
			o.webhooks.EmitTaskFailed(task.ID, task.Title, errorMsg, task.Attempts)
		}
		if o.analytics != nil {
			o.analytics.EndTask(taskID, "failed", errorMsg)
		}
		o.recordEvent(events.EventTaskFailed, task.ID, task.EpicID, map[string]any{
			"error":    errorMsg,
			"attempts": task.Attempts,
		})
		return false
	}

	// Increment attempts in database
	if err := o.store.IncrementTaskAttempts(taskID); err != nil {
		log.Printf("Error incrementing attempts for task %s: %v", taskID, err)
		_ = o.store.UpdateTaskStatus(taskID, types.TaskStatusFailed, errorMsg)
		dashboard.BroadcastTaskFailed(task.ID, task.Title, errorMsg)
		if o.analytics != nil {
			o.analytics.EndTask(taskID, "failed", errorMsg)
		}
		return false
	}

	// Allow retry - task.Attempts is now incremented
	_ = o.store.UpdateTaskStatus(taskID, types.TaskStatusReady, errorMsg)
	log.Printf("🔄 Task %s retrying (attempt %d/%d)", taskID, task.Attempts+1, task.MaxAttempts)
	return true
}

// runTests executes automated tests before task completion
// Returns an error if tests fail and the task is configured to block on test failures
func (o *Orchestrator) runTests(taskID, worktreePath string, taskSpan trace.Span) error {
	// Get the task to check test configuration
	task, err := o.store.GetTask(taskID)
	if err != nil {
		log.Printf("⚠️  Could not fetch task %s for test configuration: %v", taskID, err)
		return nil // Continue without tests if we can't get config
	}

	// Build test configuration from task
	testConfig := &testing.TestConfig{
		Mode:    testing.TestMode(task.TestMode),
		Scope:   testing.TestScope(task.TestScope),
		Timeout: 5 * time.Minute,
	}

	// Override with custom command if specified
	if task.TestCommand != "" {
		testConfig.Command = task.TestCommand
	}

	// Default to strict mode if not set
	if testConfig.Mode == "" {
		testConfig.Mode = testing.TestModeStrict
	}
	// Default to diff scope if not set
	if testConfig.Scope == "" {
		testConfig.Scope = testing.TestScopeDiff
	}

	// Skip if tests are disabled
	if testConfig.Mode == testing.TestModeDisabled {
		return nil
	}

	// Create test runner and run tests
	runner := testing.NewRunner(testConfig, worktreePath)
	runner.SetVerbose(o.verbose)

	result := runner.Run(worktreePath, taskID)

	// If tests weren't run (no changes, etc.), that's fine
	if !result.RunTests {
		return nil
	}

	// Record test results in telemetry
	if result.Success {
		telemetry.RecordTestPassed(taskSpan, result.Passed, result.Failed, result.Skipped, result.Duration)
	} else {
		telemetry.RecordTestFailed(taskSpan, result.Passed, result.Failed, result.Skipped, result.Duration, result.Error)
	}

	// In lenient mode, only log warnings
	if testConfig.Mode == testing.TestModeLenient {
		if !result.Success {
			log.Printf("⚠️  Tests failed (lenient mode - not blocking): %d passed, %d failed, %d skipped",
				result.Passed, result.Failed, result.Skipped)
			if result.Output != "" && o.verbose {
				// Print last few lines of output
				lines := strings.Split(result.Output, "\n")
				if len(lines) > 10 {
					lines = lines[len(lines)-10:]
				}
				for _, line := range lines {
					log.Printf("  %s", line)
				}
			}
		}
		return nil // Don't block in lenient mode
	}

	// In strict mode, fail if tests failed
	if !result.Success {
		return fmt.Errorf("tests failed (strict mode): %d passed, %d failed, %d skipped\n%s",
			result.Passed, result.Failed, result.Skipped, result.Output)
	}

	return nil
}

// printProgress prints current progress
func (o *Orchestrator) printProgress(status *db.ProjectStatus) {
	if status.Total == 0 {
		return
	}

	progress := float64(status.Completed) / float64(status.Total) * 100
	log.Printf("📊 Progress: %d/%d tasks (%.1f%%) | Ready: %d | In Progress: %d | Paused: %d | Blocked: %d | Failed: %d",
		status.Completed, status.Total, progress,
		status.Ready, status.InProgress, status.Paused, status.Blocked, status.Failed)
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

// getProjectTaskContextCount returns the task context count from project config or default
func (o *Orchestrator) getProjectTaskContextCount() int {
	// Try to get from project config if available
	if o.config.ProjectDir != "" {
		if projectCfg, err := project.Load(o.projectDir); err == nil {
			return projectCfg.TaskContextCount
		}
	}
	// Return default if no project config
	return 5
}

// getProjectTaskContextMaxAge returns the task context max age from project config or default
func (o *Orchestrator) getProjectTaskContextMaxAge() time.Duration {
	// Try to get from project config if available
	if o.config.ProjectDir != "" {
		if projectCfg, err := project.Load(o.projectDir); err == nil {
			return projectCfg.TaskContextMaxAge
		}
	}
	// Return default if no project config
	return 24 * time.Hour
}

// recoverOrphanedTasks finds and recovers tasks that were in progress but crashed
func (o *Orchestrator) recoverOrphanedTasks() error {
	// Default orphan timeout: 2 minutes
	orphanTimeout := int64(120)
	if o.config.StallTimeout > 0 {
		orphanTimeout = int64(o.config.StallTimeout.Seconds())
	}

	// Find orphaned checkpoints
	orphaned, err := o.store.FindOrphanedCheckpoints(orphanTimeout)
	if err != nil {
		return fmt.Errorf("finding orphaned checkpoints: %w", err)
	}

	if len(orphaned) == 0 {
		if o.verbose {
			log.Printf("[recovery] no orphaned tasks found")
		}
		return nil
	}

	log.Printf("[recovery] found %d orphaned task(s)", len(orphaned))

	for _, checkpoint := range orphaned {
		// Check if we should requeue the task
		task, err := o.store.GetTask(checkpoint.TaskID)
		if err != nil {
			log.Printf("[recovery] warning: failed to get task %s: %v", checkpoint.TaskID, err)
			continue
		}

		if task == nil {
			log.Printf("[recovery] warning: task %s no longer exists", checkpoint.TaskID)
			_ = o.store.DeleteCheckpoint(checkpoint.TaskID)
			continue
		}

		// Check retry limit
		if checkpoint.Attempt >= task.MaxAttempts {
			log.Printf("[recovery] task %s exceeded max attempts (%d), marking as failed",
				checkpoint.TaskID, task.MaxAttempts)
			_ = o.store.UpdateTaskStatus(checkpoint.TaskID, types.TaskStatusFailed,
				fmt.Sprintf("Max retries exceeded (%d attempts)", checkpoint.Attempt))
			_ = o.store.DeleteCheckpoint(checkpoint.TaskID)
			continue
		}

		// Requeue the task for retry
		log.Printf("[recovery] requeueing task %s (attempt %d/%d)",
			checkpoint.TaskID, checkpoint.Attempt+1, task.MaxAttempts)
		_ = o.store.UpdateTaskStatus(checkpoint.TaskID, types.TaskStatusReady,
			fmt.Sprintf("Recovering from crash (attempt %d)", checkpoint.Attempt))
		_ = o.store.DeleteCheckpoint(checkpoint.TaskID)
	}

	log.Printf("[recovery] recovered %d task(s)", len(orphaned))
	return nil
}
