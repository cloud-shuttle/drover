package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cloud-shuttle/drover/internal/config"
	"github.com/cloud-shuttle/drover/internal/db"
	"github.com/cloud-shuttle/drover/internal/modes"
	"github.com/cloud-shuttle/drover/internal/workflow"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
	"github.com/spf13/cobra"
)

func runCmd() *cobra.Command {
	var workers int
	var epicID string
	var verbose bool
	var poolEnabled bool
	var poolMinSize int
	var poolMaxSize int
	var workerMode string
	var requireApproval bool
	var planningRequireApproval bool
	var planningAutoApproveLow bool
	var planningMaxSteps int
	var buildingApprovedOnly bool
	var buildingVerifySteps bool
	var refinementEnabled bool
	var refinementMaxRefinements int

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Execute all tasks to completion",
		Long: `Run all tasks to completion using parallel Claude Code agents.

Tasks are executed respecting dependencies and priorities. Use --workers
to control parallelism. Use --epic to filter execution to a specific epic.

DBOS Workflow Engine:
- Default: SQLite-based orchestration (zero setup)
- With DBOS_SYSTEM_DATABASE_URL: DBOS with PostgreSQL (production mode)

Worktree Pooling:
Use --pool to enable worktree pooling for faster cold-start times.
Pre-warmed worktrees reduce setup time for tasks.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectDir, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			// Override config if flags specified
			runCfg := *cfg
			if workers > 0 {
				runCfg.Workers = workers
			}
			runCfg.Verbose = verbose
			runCfg.PoolEnabled = poolEnabled
			if poolMinSize > 0 {
				runCfg.PoolMinSize = poolMinSize
			}
			if poolMaxSize > 0 {
				runCfg.PoolMaxSize = poolMaxSize
			}
			// Override worker mode settings if flags specified
			if workerMode != "" {
				runCfg.WorkerMode = modes.WorkerMode(workerMode)
			}
			if cmd.Flags().Changed("require-approval") {
				runCfg.RequireApproval = requireApproval
			}
			if cmd.Flags().Changed("planning-require-approval") {
				runCfg.Modes.Planning.RequireApproval = planningRequireApproval
			}
			if cmd.Flags().Changed("planning-auto-approve-low") {
				runCfg.Modes.Planning.AutoApproveLowComplexity = planningAutoApproveLow
			}
			if planningMaxSteps > 0 {
				runCfg.Modes.Planning.MaxStepsPerPlan = planningMaxSteps
			}
			if cmd.Flags().Changed("building-approved-only") {
				runCfg.Modes.Building.ExecuteApprovedOnly = buildingApprovedOnly
			}
			if cmd.Flags().Changed("building-verify-steps") {
				runCfg.Modes.Building.VerifySteps = buildingVerifySteps
			}
			if cmd.Flags().Changed("refinement-enabled") {
				runCfg.Modes.Refinement.Enabled = refinementEnabled
			}
			if refinementMaxRefinements > 0 {
				runCfg.Modes.Refinement.MaxRefinements = refinementMaxRefinements
			}

			// Check if DBOS mode is enabled via environment variable
			dbosURL := os.Getenv("DBOS_SYSTEM_DATABASE_URL")

			if dbosURL != "" {
				// Use DBOS orchestrator for production
				return runWithDBOS(cmd, &runCfg, store, projectDir, dbosURL, epicID)
			}

			// Default: Use SQLite-based orchestrator for local development
			return runWithSQLite(cmd, &runCfg, store, projectDir, epicID)
		},
	}

	cmd.Flags().IntVarP(&workers, "workers", "w", 0, "Number of parallel workers")
	cmd.Flags().StringVar(&epicID, "epic", "", "Filter to specific epic")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging for debugging")
	cmd.Flags().BoolVar(&poolEnabled, "pool", false, "Enable worktree pooling for faster cold-start")
	cmd.Flags().IntVar(&poolMinSize, "pool-min", 0, "Minimum warm worktrees (default: 2)")
	cmd.Flags().IntVar(&poolMaxSize, "pool-max", 0, "Maximum pooled worktrees (default: 10)")

	// Worker mode flags
	cmd.Flags().StringVar(&workerMode, "mode", "", "Worker mode: combined, planning, or building")
	cmd.Flags().BoolVar(&requireApproval, "require-approval", false, "Require manual approval for plans")
	cmd.Flags().BoolVar(&planningRequireApproval, "planning-require-approval", false, "Require approval for plans (planning mode)")
	cmd.Flags().BoolVar(&planningAutoApproveLow, "planning-auto-approve-low", false, "Auto-approve low complexity plans")
	cmd.Flags().IntVar(&planningMaxSteps, "planning-max-steps", 0, "Maximum steps per plan (default: 20)")
	cmd.Flags().BoolVar(&buildingApprovedOnly, "building-approved-only", false, "Only execute approved plans")
	cmd.Flags().BoolVar(&buildingVerifySteps, "building-verify-steps", false, "Verify each step after execution")
	cmd.Flags().BoolVar(&refinementEnabled, "refinement-enabled", false, "Enable automatic plan refinement")
	cmd.Flags().IntVar(&refinementMaxRefinements, "refinement-max-refinements", 0, "Maximum number of refinements (default: 3)")

	return cmd
}

// runWithDBOS executes tasks using DBOS workflow engine
func runWithDBOS(cmd *cobra.Command, runCfg *config.Config, store *db.Store, projectDir, dbosURL, epicID string) error {
	fmt.Println("🐂 Using DBOS workflow engine (PostgreSQL)")

	// Show epic filter if specified
	if epicID != "" {
		fmt.Printf("🎯 Filtering to epic: %s\n", epicID)
	}

	// Initialize DBOS context
	dbosCtx, err := dbos.NewDBOSContext(context.Background(), dbos.Config{
		AppName:     "drover",
		DatabaseURL: dbosURL,
	})
	if err != nil {
		return fmt.Errorf("initializing DBOS: %w", err)
	}

	// Create DBOS orchestrator (this creates the queue before Launch)
	orch, err := workflow.NewDBOSOrchestrator(runCfg, dbosCtx, projectDir, store)
	if err != nil {
		return fmt.Errorf("creating DBOS orchestrator: %w", err)
	}

	// Register workflows
	if err := orch.RegisterWorkflows(); err != nil {
		return fmt.Errorf("registering workflows: %w", err)
	}

	// Launch DBOS runtime (must be after queue creation and workflow registration)
	if err := dbos.Launch(dbosCtx); err != nil {
		return fmt.Errorf("launching DBOS: %w", err)
	}
	defer dbos.Shutdown(dbosCtx, 5*time.Second)

	// Get tasks from database (filtered by epic if specified)
	tasks, err := store.ListTasksByEpic(epicID)
	if err != nil {
		return fmt.Errorf("listing tasks: %w", err)
	}

	// Convert to DBOS TaskInput format
	taskInputs := make([]workflow.TaskInput, 0, len(tasks))
	for _, task := range tasks {
		if task.Status == "ready" || task.Status == "claimed" || task.Status == "in_progress" {
			blockedBy, _ := store.GetBlockedBy(task.ID)
			taskInputs = append(taskInputs, workflow.TaskInput{
				TaskID:      task.ID,
				Title:       task.Title,
				Description: task.Description,
				EpicID:      task.EpicID,
				Priority:    task.Priority,
				MaxAttempts: task.MaxAttempts,
				BlockedBy:   blockedBy,
			})
		}
	}

	// Execute with queue for parallel processing
	input := workflow.QueuedTasksInput{Tasks: taskInputs}
	handle, err := dbos.RunWorkflow(dbosCtx, orch.ExecuteTasksWithQueue, input)
	if err != nil {
		return fmt.Errorf("starting DBOS workflow: %w", err)
	}

	// Wait for results
	stats, err := handle.GetResult()
	if err != nil {
		return fmt.Errorf("DBOS workflow execution failed: %w", err)
	}

	// Print results
	orch.PrintQueueStats(stats)
	return nil
}

func runWithSQLite(cmd *cobra.Command, runCfg *config.Config, store *db.Store, projectDir, epicID string) error {
	fmt.Println("🐂 Using SQLite-based orchestrator (local mode)")

	// Create orchestrator
	orch, err := workflow.NewOrchestrator(runCfg, store, projectDir)
	if err != nil {
		return fmt.Errorf("creating orchestrator: %w", err)
	}

	// Set epic filter if specified
	if epicID != "" {
		orch.SetEpicFilter(epicID)
	}

	// Setup context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signals - only process the first one
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\n🛑 Interrupt received, stopping gracefully...")
		cancel()
		// Stop listening for signals after first interrupt
		signal.Stop(sigCh)
	}()

	// Run the orchestrator
	return orch.Run(ctx)
}
