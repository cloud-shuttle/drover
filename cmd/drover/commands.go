package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/cloud-shuttle/drover/internal/config"
	"github.com/cloud-shuttle/drover/internal/dashboard"
	"github.com/cloud-shuttle/drover/internal/db"
	"github.com/cloud-shuttle/drover/internal/git"
	"github.com/cloud-shuttle/drover/internal/template"
	"github.com/cloud-shuttle/drover/internal/workflow"
	"github.com/cloud-shuttle/drover/pkg/types"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
	"github.com/spf13/cobra"
)

func initCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize Drover in the current project",
		Long: `Initialize Drover in the current project.

Creates a .drover directory with SQLite database for task storage and workflow state.
Drover uses DBOS for durable workflow execution with automatic recovery.

Database modes:
- Default: DBOS with SQLite (zero setup, durable execution)
- Production: Set DBOS_SYSTEM_DATABASE_URL to use PostgreSQL`,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := os.Getwd()
			if err != nil {
				return err
			}

			droverDir := filepath.Join(dir, ".drover")
			if _, err := os.Stat(droverDir); err == nil {
				return fmt.Errorf("already initialized in %s", droverDir)
			}

			if err := os.MkdirAll(droverDir, 0755); err != nil {
				return fmt.Errorf("creating .drover directory: %w", err)
			}

			dbPath := filepath.Join(droverDir, "drover.db")
			store, err := db.Open(dbPath)
			if err != nil {
				return fmt.Errorf("creating database: %w", err)
			}
			defer store.Close()

			if err := store.InitSchema(); err != nil {
				return fmt.Errorf("initializing schema: %w", err)
			}

			// Run any necessary migrations for existing databases
			if err := store.MigrateSchema(); err != nil {
				return fmt.Errorf("migrating schema: %w", err)
			}

			// Copy task template
			templatePath := filepath.Join(droverDir, "task_template.yaml")
			templateContent := `# Drover Task Template
# Use this template to create high-quality, actionable tasks

title: "Specific Component/Feature Name - Action Verb"
description: |
  Detailed description of what needs to be done.

  Include:
  - Target files/packages (e.g., packages/components/src/button/)
  - Specific action (create/update/fix/test/refactor)
  - Technical details (function names, feature flags, file paths)
  - Acceptance criteria (how to verify it works)

# Example good tasks:

# Example 1: Specific component update
title: "Add New York variant to Button component"
description: |
  Create packages/components/src/button/new_york.rs with:
  - New York theme styling (smaller border-radius, muted colors)
  - Same props API as default variant
  - Consistent with other New York variants
  Tests in packages/components/src/button/new_york_tests.rs

# Quality Checklist:
# [ ] Title starts with action verb (Create, Fix, Add, Update, Implement)
# [ ] Description mentions specific files/packages
# [ ] Description includes acceptance criteria
# [ ] Technical details provided (function names, feature flags)
# [ ] Context is clear (why this is needed, what problem it solves)
`
			if err := os.WriteFile(templatePath, []byte(templateContent), 0644); err != nil {
				return fmt.Errorf("creating task template: %w", err)
			}

			fmt.Printf("🐂 Initialized Drover in %s\n", droverDir)
			fmt.Println("\nWorkflow Engine:")
			fmt.Println("  • DBOS with SQLite (default): Durable execution, automatic recovery")
			fmt.Println("  • DBOS with PostgreSQL: Set DBOS_SYSTEM_DATABASE_URL for production")
			fmt.Println("\nNext steps:")
			fmt.Println("  drover epic add \"My Epic\"")
			fmt.Println("  drover add \"My first task\" --epic <epic-id>")
			fmt.Println("  drover run")
			fmt.Println("\n📋 Task quality template created: .drover/task_template.yaml")
			fmt.Println("   Review it before adding tasks for best results!")

			return nil
		},
	}
}

func runCmd() *cobra.Command {
	var workers int
	var epicID string
	var verbose bool
	var poolEnabled bool
	var poolMinSize int
	var poolMaxSize int

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

func addCmd() *cobra.Command {
	var (
		desc           string
		epicID         string
		parentID       string
		priority       int
		blockedBy      []string
		skipValidation bool
	)

	command := &cobra.Command{
		Use:   "add <title>",
		Short: "Add a new task",
		Long: `Add a new task to the project.

Tasks are validated against quality standards to ensure they are actionable.
Use --skip-validation to bypass validation (not recommended).

Hierarchical Tasks:
  Use --parent to create a sub-task, or use hierarchical ID syntax:
    drover add task-123.1 "Sub-task title"
    drover add "Sub-task title" --parent task-123

Maximum depth is 2 levels (Epic → Parent → Child).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			title := args[0]

			// Auto-detect hierarchical ID syntax (e.g., "task-123.1 Title here")
			if parentID == "" {
				// First, try to extract a hierarchical ID prefix from the title
				// The pattern is: prefix-number(.number(.number)?) followed by space and title
				// We need to find the longest matching prefix
				words := strings.Fields(title)
				if len(words) > 0 {
					firstWord := words[0]
					baseID, level1, level2, _ := db.ParseHierarchicalID(firstWord)
					if baseID != "" && (level1 > 0 || level2 > 0) {
						// Extract the parent ID, sequence number, and actual title
						var sequence int
						if level2 > 0 {
							// task-123.1.2 -> parent is task-123.1, sequence is 2
							parentID = fmt.Sprintf("%s.%d", baseID, level1)
							sequence = level2
						} else if level1 > 0 {
							// task-123.1 -> parent is task-123, sequence is 1
							parentID = baseID
							sequence = level1
						}
						// Extract the actual title (after the hierarchical ID prefix)
						title = strings.TrimSpace(strings.TrimPrefix(title, firstWord+" "))

						// Use CreateSubTaskWithSequence when user specifies a sequence number
						subTask, err := store.CreateSubTaskWithSequence(title, desc, parentID, sequence, priority, blockedBy)
						if err != nil {
							return err
						}
						fmt.Printf("✅ Created task %s\n", subTask.ID)
						return nil
					}
				}
			}

			// Validate task quality unless explicitly skipped
			if !skipValidation {
				errors := template.Validate(title, desc)
				if len(errors) > 0 {
					fmt.Printf("⚠️  Task quality validation failed:\n\n")
					for _, e := range errors {
						fmt.Printf("  [%s] %s\n", e.Field, e.Message)
						for _, s := range e.Suggestions {
							fmt.Printf("    → %s\n", s)
						}
						fmt.Println()
					}
					fmt.Println("💡 Tips for better tasks:")
					fmt.Println("  1. Be specific: mention files, components, or packages")
					fmt.Println("  2. Use action verbs: Create, Fix, Add, Update, Implement")
					fmt.Println("  3. Add acceptance criteria: how to verify it works")
					fmt.Println("  4. Include technical details: function names, feature flags")
					fmt.Println("\nReference template: .drover/task_template.yaml")
					fmt.Println("\nUse --skip-validation to create this task anyway (not recommended)")
					return fmt.Errorf("task validation failed")
				}
			}

			var task *types.Task
			if parentID != "" {
				// Create sub-task with hierarchical ID
				task, err = store.CreateSubTask(title, desc, parentID, priority, blockedBy)
			} else {
				// Create regular task
				task, err = store.CreateTask(title, desc, epicID, priority, blockedBy)
			}
			if err != nil {
				return err
			}

			fmt.Printf("✅ Created task %s\n", task.ID)
			return nil
		},
	}

	command.Flags().StringVarP(&desc, "description", "d", "", "Task description")
	command.Flags().StringVarP(&epicID, "epic", "e", "", "Assign to epic")
	command.Flags().StringVarP(&parentID, "parent", "P", "", "Parent task ID (creates sub-task)")
	command.Flags().IntVarP(&priority, "priority", "p", 0, "Task priority (higher = more urgent)")
	command.Flags().StringSliceVar(&blockedBy, "blocked-by", nil, "Task IDs this depends on")
	command.Flags().BoolVar(&skipValidation, "skip-validation", false, "Skip task quality validation (not recommended)")
	return command
}

// quickCmd creates a task with minimal input for rapid capture
func quickCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "quick <title>",
		Short: "Quickly create a task (no validation, minimal prompts)",
		Long: `Quickly create a task with minimal friction.

Skips all validation and prompts. Just provide a title and go.
Perfect for capturing ideas quickly during meetings or brainstorming.

Examples:
  drover quick "Fix the login bug"
  drover quick "Investigate the memory leak in worker pool"
  drover quick "Add dark mode to dashboard"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			title := args[0]

			// Create task without any validation
			task, err := store.CreateTask(title, "", "", 0, nil)
			if err != nil {
				return err
			}

			fmt.Printf("⚡ Quick capture: %s\n", task.ID)
			fmt.Printf("   %s\n", task.Title)
			return nil
		},
	}
}

// watchCmd provides a standalone watch command for live status updates
func watchCmd() *cobra.Command {
	var onelineMode bool

	command := &cobra.Command{
		Use:   "watch",
		Short: "Watch status updates in real-time",
		Long: `Continuously display task status with auto-refresh.

This provides a lightweight alternative to running the full workflow.
Press Ctrl+C to exit.

Examples:
  drover watch          # Full status display
  drover watch --oneline # Compact one-line display`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			// Clear screen on start
			fmt.Print("\033[H\033[2J")

			// Set up signal handling for graceful exit
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

			// Create a ticker for regular updates
			ticker := time.NewTicker(1 * time.Second)
			defer ticker.Stop()

			var lastStatus *db.ProjectStatus

			for {
				select {
				case <-sigChan:
					// User pressed Ctrl+C
					fmt.Println("\n\n👋 Watch mode stopped")
					return nil

				case <-ticker.C:
					// Get fresh status
					status, err := store.GetProjectStatus()
					if err != nil {
						fmt.Printf("\nError getting status: %v\n", err)
						return err
					}

					// Only update if something changed
					if lastStatus == nil || statusChanged(lastStatus, status) {
						// Clear screen and move cursor to top-left
						fmt.Print("\033[H\033[2J")

						if onelineMode {
							// Compact one-line display
							fmt.Printf("🐂 [%s] %s\n", time.Now().Format("15:04:05"),
								printStatusOnelineContent(status))
						} else {
							// Full status display with header
							fmt.Printf("🐂 Drover Watch (live - %s)\n", time.Now().Format("15:04:05"))
							fmt.Println("════════════════════════════════════════")
							fmt.Printf("\nTotal:      %d\n", status.Total)
							fmt.Printf("Ready:      %d\n", status.Ready)
							fmt.Printf("In Progress: %d\n", status.InProgress)
							fmt.Printf("Paused:     %d\n", status.Paused)
							fmt.Printf("Completed:  %d\n", status.Completed)
							fmt.Printf("Failed:     %d\n", status.Failed)
							fmt.Printf("Blocked:    %d\n", status.Blocked)

							if status.Total > 0 {
								progress := float64(status.Completed) / float64(status.Total) * 100
								fmt.Printf("\nProgress: %.1f%%\n", progress)
								printProgressBarCompact(progress)
							}
						}

						lastStatus = status
					}
				}
			}
		},
	}

	command.Flags().BoolVar(&onelineMode, "oneline", false, "Show compact one-line display")
	return command
}

func epicCmd() *cobra.Command {
	epicAdd := &cobra.Command{
		Use:   "add <title>",
		Short: "Create a new epic",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			desc, _ := cmd.Flags().GetString("description")

			epic, err := store.CreateEpic(args[0], desc)
			if err != nil {
				return err
			}

			fmt.Printf("✅ Created epic %s: %s\n", epic.ID, epic.Title)
			return nil
		},
	}

	epicAdd.Flags().StringP("description", "d", "", "Epic description")

	command := &cobra.Command{
		Use:   "epic",
		Short: "Manage epics",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	// epic list - show all epics
	epicList := &cobra.Command{
		Use:   "list",
		Short: "List all epics",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			epics, err := store.ListEpics()
			if err != nil {
				return fmt.Errorf("listing epics: %w", err)
			}

			if len(epics) == 0 {
				fmt.Println("No epics found. Create one with: drover epic add \"My Epic\"")
				return nil
			}

			for _, epic := range epics {
				emoji := "📘"
				switch string(epic.Status) {
				case "open":
					emoji = "📖"
				case "closed":
					emoji = "📕"
				}
				fmt.Printf("%s  %s — %s (%s)\n", emoji, epic.ID, epic.Title, epic.Status)
			}

			return nil
		},
	}

	// epic close - close an epic
	epicClose := &cobra.Command{
		Use:   "close <epic-id>",
		Short: "Close an epic",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			epicID := args[0]
			if err := store.UpdateEpicStatus(epicID, types.EpicStatusClosed); err != nil {
				return err
			}
			fmt.Printf("🔒 Closed epic %s\n", epicID)
			return nil
		},
	}

	command.AddCommand(epicAdd, epicList, epicClose)
	return command
}

func statusCmd() *cobra.Command {
	var watchMode bool
	var treeMode bool
	var onelineMode bool

	command := &cobra.Command{
		Use:   "status",
		Short: "Show current project status",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			if watchMode {
				return runWatchMode(store)
			}

			if treeMode {
				return printTreeStatus(store)
			}

			status, err := store.GetProjectStatus()
			if err != nil {
				return err
			}

			if onelineMode {
				printStatusOneline(status)
				return nil
			}

			printStatus(status)
			return nil
		},
	}

	command.Flags().BoolVarP(&watchMode, "watch", "w", false, "Watch mode - live updates")
	command.Flags().BoolVarP(&treeMode, "tree", "t", false, "Tree mode - show hierarchical view")
	command.Flags().BoolVar(&onelineMode, "oneline", false, "Single line summary (e.g., for shell prompts)")
	return command
}

func resumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resume",
		Short: "Resume interrupted workflows",
		Long: `Resume interrupted workflows.

DBOS automatically handles workflow recovery through its durable execution engine.
If a workflow is interrupted, simply run 'drover run' again and DBOS will
continue from where it left off.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("🐂 DBOS mode: Workflows are automatically recovered on 'drover run'")
			fmt.Println("\nTo resume execution, simply run:")
			fmt.Println("  drover run")
			fmt.Println("\n💡 DBOS handles workflow recovery automatically through durable execution.")
			return nil
		},
	}
}

func infoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info <task-id>",
		Short: "Show detailed information about a specific task",
		Long: `Show detailed information about a specific task.

Displays task title, description, status, epic, priority, dependencies,
and other metadata. Useful for inspecting individual task details.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			taskID := args[0]

			// Get task details
			task, err := store.GetTask(taskID)
			if err != nil {
				return fmt.Errorf("task not found: %s", taskID)
			}

			// Get dependencies
			blockedBy, err := store.GetBlockedBy(taskID)
			if err != nil {
				blockedBy = nil
			}

			// Find tasks that depend on this one
			rows, err := store.DB.Query(`
				SELECT task_id FROM task_dependencies WHERE blocked_by = ?
			`, taskID)
			var blocking []string
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var id string
					if rows.Scan(&id) == nil {
						blocking = append(blocking, id)
					}
				}
			}

			printTaskInfo(task, blockedBy, blocking)
			return nil
		},
	}
}

func resetCmd() *cobra.Command {
	var (
		resetCompleted  bool
		resetInProgress bool
		resetClaimed    bool
		resetFailed     bool
	)

	command := &cobra.Command{
		Use:   "reset [TASK_IDS...]",
		Short: "Reset tasks back to ready status",
		Long: `Reset tasks back to ready status.

If task IDs are provided, only those specific tasks will be reset.
Otherwise, use flags to specify which statuses to reset.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			// If specific task IDs are provided, reset only those
			if len(args) > 0 {
				count, err := store.ResetTasksByIDs(args)
				if err != nil {
					return err
				}
				fmt.Printf("🔄 Reset %d task(s) to ready status\n", count)
				return nil
			}

			// Otherwise, use status-based reset (existing behavior)
			var statusesToReset []types.TaskStatus

			if resetCompleted {
				statusesToReset = append(statusesToReset, types.TaskStatusCompleted)
			}
			if resetInProgress {
				statusesToReset = append(statusesToReset, types.TaskStatusInProgress)
			}
			if resetClaimed {
				statusesToReset = append(statusesToReset, types.TaskStatusClaimed)
			}
			if resetFailed {
				statusesToReset = append(statusesToReset, types.TaskStatusFailed)
			}

			// If no flags specified, reset claimed, in-progress and completed
			if len(statusesToReset) == 0 {
				statusesToReset = []types.TaskStatus{
					types.TaskStatusClaimed,
					types.TaskStatusInProgress,
					types.TaskStatusCompleted,
				}
			}

			count, err := store.ResetTasks(statusesToReset)
			if err != nil {
				return err
			}

			fmt.Printf("🔄 Reset %d tasks to ready status\n", count)
			return nil
		},
	}

	command.Flags().BoolVar(&resetCompleted, "completed", false, "Reset completed tasks")
	command.Flags().BoolVar(&resetInProgress, "in-progress", false, "Reset in-progress tasks")
	command.Flags().BoolVar(&resetClaimed, "claimed", false, "Reset claimed tasks")
	command.Flags().BoolVar(&resetFailed, "failed", false, "Reset failed tasks")

	return command
}

func exportCmd() *cobra.Command {
	var output string
	var format string

	command := &cobra.Command{
		Use:   "export",
		Short: "Export tasks to portable format",
		Long: `Export tasks and session state to a portable format.

Formats:
  - beads: Export to .beads/beads.jsonl (default, for beads sync)
  - json: Export to full session JSON file (for handoff/import)
  - yaml: Export to full session YAML file (for human readability)

Examples:
  drover export                    # Export to beads format
  drover export --format json       # Export to session JSON
  drover export --format json --output session.drover`,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectDir, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			if format == "json" || format == "yaml" {
				return exportSession(projectDir, store, output, format)
			}
			// Default: export to beads format
			return exportToBeads(projectDir, store)
		},
	}

	command.Flags().StringVarP(&format, "format", "f", "beads", "Export format: beads, json, or yaml")
	command.Flags().StringVarP(&output, "output", "o", "", "Output file path (default: session-YYYY-MM-DD.drover for json/yaml)")
	return command
}

// exportToBeads exports tasks to beads format
func exportToBeads(projectDir string, store *db.Store) error {
	// Get all tasks from database
	rows, err := store.DB.Query(`
		SELECT id, title, COALESCE(description, ''), COALESCE(epic_id, ''),
		       priority, status, created_at
		FROM tasks
		ORDER BY created_at ASC
	`)
	if err != nil {
		return fmt.Errorf("querying tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*types.Task
	for rows.Next() {
		var task types.Task
		var description sql.NullString
		var epicID sql.NullString
		err := rows.Scan(&task.ID, &task.Title, &description, &epicID,
			&task.Priority, &task.Status, &task.CreatedAt)
		if err != nil {
			return fmt.Errorf("scanning task: %w", err)
		}
		task.Description = description.String
		task.EpicID = epicID.String
		tasks = append(tasks, &task)
	}

	// Get all epics from database
	rows2, err := store.DB.Query(`
		SELECT id, title, COALESCE(description, ''), status, created_at
		FROM epics
		ORDER BY created_at ASC
	`)
	if err != nil {
		return fmt.Errorf("querying epics: %w", err)
	}
	defer rows2.Close()

	var epics []*types.Epic
	for rows2.Next() {
		var epic types.Epic
		var description sql.NullString
		err := rows2.Scan(&epic.ID, &epic.Title, &description, &epic.Status, &epic.CreatedAt)
		if err != nil {
			return fmt.Errorf("scanning epic: %w", err)
		}
		epic.Description = description.String
		epics = append(epics, &epic)
	}

	// Write beads.jsonl
	beadsDir := filepath.Join(projectDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		return fmt.Errorf("creating beads dir: %w", err)
	}

	jsonlPath := filepath.Join(beadsDir, "beads.jsonl")
	file, err := os.Create(jsonlPath)
	if err != nil {
		return fmt.Errorf("creating beads.jsonl: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)

	// Export epics
	for _, epic := range epics {
		record := map[string]interface{}{
			"type":      "epic",
			"id":        epic.ID,
			"timestamp": time.Unix(epic.CreatedAt, 0),
			"data": map[string]interface{}{
				"title":       epic.Title,
				"description": epic.Description,
				"status":      epic.Status,
			},
		}
		if err := encoder.Encode(record); err != nil {
			return fmt.Errorf("encoding epic: %w", err)
		}
	}

	// Export tasks
	for _, task := range tasks {
		status := droverStatusToBeads(task.Status)
		record := map[string]interface{}{
			"type":      "bead",
			"id":        task.ID,
			"timestamp": time.Unix(task.CreatedAt, 0),
			"data": map[string]interface{}{
				"title":       task.Title,
				"description": task.Description,
				"status":      status,
				"priority":    task.Priority,
				"epic_id":     task.EpicID,
			},
		}
		if err := encoder.Encode(record); err != nil {
			return fmt.Errorf("encoding task: %w", err)
		}
	}

	fmt.Printf("✅ Exported %d epics and %d tasks to %s\n", len(epics), len(tasks), jsonlPath)
	return nil
}

// exportSession exports full session state to JSON or YAML
func exportSession(projectDir string, store *db.Store, outputPath, format string) error {
	// Get all tasks with full details
	tasks, err := store.ListTasks()
	if err != nil {
		return fmt.Errorf("querying tasks: %w", err)
	}

	// Get all epics
	epics, err := store.ListEpics()
	if err != nil {
		return fmt.Errorf("querying epics: %w", err)
	}

	// Get all dependencies
	dependencies, err := store.ListAllDependencies()
	if err != nil {
		return fmt.Errorf("querying dependencies: %w", err)
	}

	// Get worktrees
	worktrees, err := store.ListWorktrees()
	if err != nil {
		return fmt.Errorf("querying worktrees: %w", err)
	}

	// Build session export
	session := map[string]interface{}{
		"version":      "1.0",
		"exportedAt":   time.Now().Format(time.RFC3339),
		"repository":   projectDir,
		"tasks":        tasks,
		"epics":        epics,
		"dependencies": dependencies,
		"worktrees":    worktrees,
	}

	// Determine output path
	if outputPath == "" {
		outputPath = filepath.Join(projectDir, fmt.Sprintf("session-%s.drover", time.Now().Format("2006-01-02")))
	}

	// Write export
	var data []byte
	if format == "json" {
		data, err = json.MarshalIndent(session, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling JSON: %w", err)
		}
	} else {
		// YAML format - use simple JSON for now, can add yaml library later
		data, err = json.MarshalIndent(session, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling YAML: %w", err)
		}
	}

	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return fmt.Errorf("writing session file: %w", err)
	}

	fmt.Printf("✅ Exported session to %s\n", outputPath)
	fmt.Printf("   Epics: %d, Tasks: %d, Dependencies: %d, Worktrees: %d\n",
		len(epics), len(tasks), len(dependencies), len(worktrees))
	fmt.Println("\nUse 'drover import <file>' on another machine to continue.")

	return nil
}

func droverStatusToBeads(status types.TaskStatus) string {
	switch status {
	case types.TaskStatusReady, types.TaskStatusClaimed, types.TaskStatusBlocked:
		return "open"
	case types.TaskStatusInProgress:
		return "active"
	case types.TaskStatusCompleted, types.TaskStatusFailed:
		return "closed"
	default:
		return "open"
	}
}

// pauseCmd pauses a running task
func pauseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pause <task-id>",
		Short: "Pause a running task",
		Long: `Pause a running task, preserving its worktree state.

The task must be in 'in_progress' or 'claimed' status to be paused.
Pausing a task will:
  - Stop the task's execution
  - Preserve the worktree state
  - Keep any changes made so far
  - Allow manual intervention in the worktree

Use 'drover resume' to continue the task from where it left off.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			taskID := args[0]

			// Get task details first
			task, err := store.GetTask(taskID)
			if err != nil {
				return fmt.Errorf("task not found: %s", taskID)
			}

			// Pause the task
			if err := store.PauseTask(taskID); err != nil {
				return fmt.Errorf("pausing task: %w", err)
			}

			fmt.Printf("⏸️  Paused task %s\n", taskID)
			fmt.Printf("   %s\n", task.Title)
			fmt.Println("\nWorktree state preserved. Use 'drover resume' to continue.")

			return nil
		},
	}
}

// resumeCmdForTask resumes a paused task
func resumeCmdForTask() *cobra.Command {
	var hint string

	command := &cobra.Command{
		Use:   "resume-task <task-id>",
		Short: "Resume a paused task",
		Long: `Resume a paused task, continuing from where it left off.

The task must be in 'paused' status to be resumed.
Optionally provide guidance/hints that will be injected when the task continues.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			taskID := args[0]

			// Get task details first
			task, err := store.GetTask(taskID)
			if err != nil {
				return fmt.Errorf("task not found: %s", taskID)
			}

			// Add hint if provided
			if hint != "" {
				_, err := store.AddGuidance(taskID, hint)
				if err != nil {
					return fmt.Errorf("adding guidance: %w", err)
				}
				fmt.Printf("💡 Added guidance to task %s\n", taskID)
			}

			// Resume the task
			if err := store.ResumeTask(taskID); err != nil {
				return fmt.Errorf("resuming task: %w", err)
			}

			fmt.Printf("▶️  Resumed task %s\n", taskID)
			fmt.Printf("   %s\n", task.Title)
			if hint != "" {
				fmt.Println("\nGuidance will be injected when the task is claimed.")
			}

			return nil
		},
	}

	command.Flags().StringVarP(&hint, "hint", "H", "", "Guidance message to inject when task resumes")
	return command
}

// hintCmd adds guidance to a task's queue
func hintCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "hint <task-id> <message>",
		Short: "Send guidance to a running task",
		Long: `Send guidance or hints to a running task.

The guidance will be queued and injected at the next safe point
during task execution. This allows you to steer the AI without
interrupting its work.

Example:
  drover hint task-123 "Try using the existing auth middleware"`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			taskID := args[0]
			message := strings.Join(args[1:], " ")

			// Get task details first
			task, err := store.GetTask(taskID)
			if err != nil {
				return fmt.Errorf("task not found: %s", taskID)
			}

			// Add guidance
			guidance, err := store.AddGuidance(taskID, message)
			if err != nil {
				return fmt.Errorf("adding guidance: %w", err)
			}

			fmt.Printf("💡 Guidance queued for %s\n", taskID)
			fmt.Printf("   Task: %s\n", task.Title)
			fmt.Printf("   Message: %s\n", message)
			fmt.Printf("   ID: %s\n", guidance.ID)

			return nil
		},
	}
}

// importCmd imports a session from an export file
func importCmd() *cobra.Command {
	var continueExecution bool

	command := &cobra.Command{
		Use:   "import <file>",
		Short: "Import a session from an export file",
		Long: `Import a session from an export file created by 'drover export --format json'.

This restores tasks, epics, and dependencies from the exported session.
Worktrees are not imported as they are machine-specific.

Examples:
  drover import session-2024-01-13.drover
  drover import session.drover --continue    # Import and continue execution`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectDir, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			importFile := args[0]

			// Read the import file
			data, err := os.ReadFile(importFile)
			if err != nil {
				return fmt.Errorf("reading import file: %w", err)
			}

			// Parse the session
			var session db.SessionExport
			if err := json.Unmarshal(data, &session); err != nil {
				return fmt.Errorf("parsing session file: %w", err)
			}

			// Validate version
			if session.Version != "1.0" {
				return fmt.Errorf("unsupported session version: %s (expected 1.0)", session.Version)
			}

			fmt.Printf("📦 Importing session from %s\n", importFile)
			fmt.Printf("   Repository: %s\n", session.Repository)
			fmt.Printf("   Exported: %s\n", session.ExportedAt)
			fmt.Printf("   Epics: %d, Tasks: %d, Dependencies: %d\n",
				len(session.Epics), len(session.Tasks), len(session.Dependencies))

			// Import the session
			if err := store.ImportSession(&session); err != nil {
				return fmt.Errorf("importing session: %w", err)
			}

			fmt.Println("\n✅ Session imported successfully")

			if continueExecution {
				fmt.Println("\n▶️  Starting execution...")
				// Create a new orchestrator and run
				runCfg, err := config.Load()
				if err != nil {
					return fmt.Errorf("loading config: %w", err)
				}
				orch, err := workflow.NewOrchestrator(runCfg, store, projectDir)
				if err != nil {
					return fmt.Errorf("creating orchestrator: %w", err)
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				return orch.Run(ctx)
			}

			return nil
		},
	}

	command.Flags().BoolVarP(&continueExecution, "continue", "c", false, "Continue execution after import")
	return command
}

// editCmd shows the worktree path for manual editing of paused tasks
func editCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit <task-id>",
		Short: "Show worktree path for manual editing of a paused task",
		Long: `Show the worktree path for a paused task, allowing manual intervention.

This command displays the path to the worktree for a paused task, enabling
you to manually edit files before resuming the task.

The worktree will be preserved when you resume, keeping your manual changes.

Example:
  drover edit task-123
  cd /path/to/worktree/task-123
  # Make your edits...
  drover resume-task task-123`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectDir, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			taskID := args[0]

			// Get task details
			task, err := store.GetTask(taskID)
			if err != nil {
				return fmt.Errorf("task not found: %s", taskID)
			}

			// Check if task is paused
			if task.Status != types.TaskStatusPaused {
				return fmt.Errorf("task must be paused to edit (current status: %s)", task.Status)
			}

			// Get the worktree path
			worktreePath := filepath.Join(projectDir, ".drover", "worktrees", taskID)

			// Check if worktree exists
			if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
				return fmt.Errorf("worktree not found at %s (it may have been cleaned up)", worktreePath)
			}

			fmt.Printf("📁 Worktree path: %s\n", worktreePath)
			fmt.Printf("\nTask: %s\n", task.Title)
			fmt.Println("\nYou can now:")
			fmt.Printf("  cd %s\n", worktreePath)
			fmt.Println("  # Make your manual edits...")
			fmt.Println("  drover resume-task", taskID)

			return nil
		},
	}
}

// shareCmd creates a shareable session link
func shareCmd() *cobra.Command {
	var expiresHours int

	command := &cobra.Command{
		Use:   "share",
		Short: "Create a shareable link for the current session",
		Long: `Create a shareable link that allows other operators to import this session.

This generates a unique token that can be shared with other operators. They can
use the token to import the session via 'drover import-share <token>'.

The link can optionally expire after a specified number of hours.

Examples:
  drover share                    # Create link that doesn't expire
  drover share --expires 24       # Create link that expires in 24 hours`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectDir, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			// Get operator name
			operator := config.GetOperator()

			// Create session export (use project directory name as repo name)
			session := db.SessionExport{
				Version:    "1.0",
				ExportedAt: time.Now().Format(time.RFC3339),
				Repository: filepath.Base(projectDir),
			}

			// Get all epics
			epics, err := store.ListEpics()
			if err != nil {
				return fmt.Errorf("getting epics: %w", err)
			}
			session.Epics = epics

			// Get all tasks
			tasks, err := store.ListTasks()
			if err != nil {
				return fmt.Errorf("getting tasks: %w", err)
			}
			session.Tasks = tasks

			// Get all dependencies
			deps, err := store.ListAllDependencies()
			if err != nil {
				return fmt.Errorf("getting dependencies: %w", err)
			}
			session.Dependencies = deps

			// Serialize to JSON
			sessionJSON, err := json.Marshal(session)
			if err != nil {
				return fmt.Errorf("serializing session: %w", err)
			}

			// Create share
			share, err := store.CreateSessionShare(string(sessionJSON), operator, expiresHours)
			if err != nil {
				return fmt.Errorf("creating share: %w", err)
			}

			fmt.Printf("🔗 Shareable session link created!\n\n")
			fmt.Printf("Token: %s\n", share.Token)
			if share.ExpiresAt != nil {
				expiresAt := time.Unix(*share.ExpiresAt, 0)
				fmt.Printf("Expires: %s\n", expiresAt.Format(time.RFC1123))
			} else {
				fmt.Printf("Expires: never\n")
			}
			fmt.Printf("\nShare this token with other operators. They can import the session with:\n")
			fmt.Printf("  drover import-share %s\n", share.Token)

			return nil
		},
	}

	command.Flags().IntVarP(&expiresHours, "expires", "e", 0, "Hours until the link expires (0 = never)")
	return command
}

// importShareCmd imports a session from a shared token
func importShareCmd() *cobra.Command {
	var continueExecution bool

	command := &cobra.Command{
		Use:   "import-share <token>",
		Short: "Import a session from a shared token",
		Long: `Import a session from a shared token created by 'drover share'.

This restores the tasks, epics, and dependencies from the shared session.
Worktrees are not imported as they are machine-specific.

Examples:
  drover import-share abc123xyz
  drover import-share abc123xyz --continue    # Import and continue execution`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectDir, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			token := args[0]

			// Get the share
			share, err := store.GetSessionShareByToken(token)
			if err != nil {
				return fmt.Errorf("getting shared session: %w", err)
			}

			// Increment access count
			if err := store.IncrementShareAccess(token); err != nil {
				return fmt.Errorf("updating access count: %w", err)
			}

			// Parse the session data
			var session db.SessionExport
			if err := json.Unmarshal([]byte(share.SessionData), &session); err != nil {
				return fmt.Errorf("parsing session data: %w", err)
			}

			// Validate version
			if session.Version != "1.0" {
				return fmt.Errorf("unsupported session version: %s (expected 1.0)", session.Version)
			}

			fmt.Printf("📦 Importing shared session\n")
			fmt.Printf("   Created by: %s\n", share.CreatedBy)
			fmt.Printf("   Repository: %s\n", session.Repository)
			fmt.Printf("   Exported: %s\n", session.ExportedAt)
			fmt.Printf("   Epics: %d, Tasks: %d, Dependencies: %d\n",
				len(session.Epics), len(session.Tasks), len(session.Dependencies))

			// Import the session
			if err := store.ImportSession(&session); err != nil {
				return fmt.Errorf("importing session: %w", err)
			}

			fmt.Println("\n✅ Session imported successfully")

			if continueExecution {
				fmt.Println("\n▶️  Starting execution...")
				runCfg, err := config.Load()
				if err != nil {
					return fmt.Errorf("loading config: %w", err)
				}
				orch, err := workflow.NewOrchestrator(runCfg, store, projectDir)
				if err != nil {
					return fmt.Errorf("creating orchestrator: %w", err)
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				return orch.Run(ctx)
			}

			return nil
		},
	}

	command.Flags().BoolVarP(&continueExecution, "continue", "c", false, "Continue execution after import")
	return command
}

// operatorCmd manages operators for multiplayer collaboration
func operatorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "operator",
		Short: "Manage operators for multiplayer collaboration",
		Long: `Manage operators (users) in the Drover system.

Operators can be created with API keys for authentication in multiplayer scenarios.`,
	}

	cmd.AddCommand(
		operatorCreateCmd(),
		operatorListCmd(),
		operatorDeleteCmd(),
		operatorLoginCmd(),
	)

	return cmd
}

// operatorCreateCmd creates a new operator
func operatorCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new operator",
		Long: `Create a new operator with a generated API key.

The API key can be used for authentication in multiplayer scenarios.

Example:
  drover operator create alice`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			name := args[0]

			// Check if operator already exists
			_, err = store.GetOperatorByName(name)
			if err == nil {
				return fmt.Errorf("operator '%s' already exists", name)
			}

			// Create operator
			op, err := store.CreateOperator(name)
			if err != nil {
				return fmt.Errorf("creating operator: %w", err)
			}

			fmt.Printf("✅ Operator created successfully!\n\n")
			fmt.Printf("Name: %s\n", op.Name)
			fmt.Printf("API Key: %s\n\n", op.APIKey)
			fmt.Printf("Save this API key securely. You'll need it to authenticate as this operator.\n")
			fmt.Printf("Use it with: export DROVER_API_KEY=%s\n", op.APIKey)

			return nil
		},
	}
}

// operatorListCmd lists all operators
func operatorListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all operators",
		Long: `List all operators in the system.

Example:
  drover operator list`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			operators, err := store.ListOperators()
			if err != nil {
				return fmt.Errorf("listing operators: %w", err)
			}

			if len(operators) == 0 {
				fmt.Println("No operators found. Create one with: drover operator create <name>")
				return nil
			}

			fmt.Printf("Operators (%d):\n\n", len(operators))
			for _, op := range operators {
				fmt.Printf("  • %s\n", op.Name)
				if op.LastActive != nil {
					lastActive := time.Unix(*op.LastActive, 0)
					fmt.Printf("    Last active: %s\n", lastActive.Format(time.RFC1123))
				} else {
					fmt.Printf("    Last active: never\n")
				}
			}

			return nil
		},
	}
}

// operatorDeleteCmd deletes an operator
func operatorDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete an operator",
		Long: `Delete an operator from the system.

Warning: This action cannot be undone.

Example:
  drover operator delete alice`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			name := args[0]

			// Check if operator exists
			_, err = store.GetOperatorByName(name)
			if err != nil {
				return fmt.Errorf("operator '%s' not found", name)
			}

			// Delete operator
			if err := store.DeleteOperator(name); err != nil {
				return fmt.Errorf("deleting operator: %w", err)
			}

			fmt.Printf("✅ Operator '%s' deleted successfully\n", name)

			return nil
		},
	}
}

// operatorLoginCmd authenticates as an operator using API key
func operatorLoginCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login <name>",
		Short: "Login as an operator (sets default operator)",
		Long: `Set the default operator for the current session.

This saves the operator name to the configuration file, so you don't need to
specify it for every command.

Example:
  drover operator login alice`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			// Update config with operator
			if err := config.SetOperator(name); err != nil {
				return fmt.Errorf("setting operator: %w", err)
			}

			fmt.Printf("✅ Logged in as '%s'\n", name)

			return nil
		},
	}
}

// runWatchMode continuously updates the status display
func runWatchMode(store *db.Store) error {
	// Clear screen on start
	fmt.Print("\033[H\033[2J")

	// Set up signal handling for graceful exit
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Create a ticker for regular updates
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var lastStatus *db.ProjectStatus

	for {
		select {
		case <-sigChan:
			// User pressed Ctrl+C
			fmt.Println("\n\n👋 Watch mode stopped")
			return nil

		case <-ticker.C:
			// Get fresh status
			status, err := store.GetProjectStatus()
			if err != nil {
				fmt.Printf("\nError getting status: %v\n", err)
				return err
			}

			// Only update if something changed
			if lastStatus == nil || statusChanged(lastStatus, status) {
				// Clear screen and move cursor to top-left
				fmt.Print("\033[H\033[2J")

				// Print header with timestamp
				fmt.Printf("🐂 Drover Status (watch mode - %s)\n", time.Now().Format("15:04:05"))
				fmt.Println("════════════════════════════════════════")
				fmt.Printf("\nTotal:      %d\n", status.Total)
				fmt.Printf("Ready:      %d\n", status.Ready)
				fmt.Printf("In Progress: %d\n", status.InProgress)
				fmt.Printf("Paused:     %d\n", status.Paused)
				fmt.Printf("Completed:  %d\n", status.Completed)
				fmt.Printf("Failed:     %d\n", status.Failed)
				fmt.Printf("Blocked:    %d\n", status.Blocked)

				if status.Total > 0 {
					progress := float64(status.Completed) / float64(status.Total) * 100
					fmt.Printf("\nProgress: %.1f%%\n", progress)
					printProgressBar(progress)
				}

				fmt.Println("\nPress Ctrl+C to exit")

				lastStatus = status
			}
		}
	}
}

// statusChanged checks if the status has changed since last update
func statusChanged(old, new *db.ProjectStatus) bool {
	return old.Total != new.Total ||
		old.Ready != new.Ready ||
		old.InProgress != new.InProgress ||
		old.Paused != new.Paused ||
		old.Completed != new.Completed ||
		old.Failed != new.Failed ||
		old.Blocked != new.Blocked
}

func printStatus(status *db.ProjectStatus) {
	fmt.Println("\n🐂 Drover Status")
	fmt.Println("════════════════")
	fmt.Printf("\nTotal:      %d\n", status.Total)
	fmt.Printf("Ready:      %d\n", status.Ready)
	fmt.Printf("In Progress: %d\n", status.InProgress)
	fmt.Printf("Paused:     %d\n", status.Paused)
	fmt.Printf("Completed:  %d\n", status.Completed)
	fmt.Printf("Failed:     %d\n", status.Failed)
	fmt.Printf("Blocked:    %d\n", status.Blocked)

	if status.Total > 0 {
		progress := float64(status.Completed) / float64(status.Total) * 100
		fmt.Printf("\nProgress: %.1f%%\n", progress)
		printProgressBar(progress)
	}
}

// printStatusOneline prints a single-line status summary
// Format: "X running, Y queued, Z completed, W blocked"
// Useful for shell prompt integration
func printStatusOneline(status *db.ProjectStatus) {
	fmt.Printf("%d running, %d queued, %d completed, %d blocked",
		status.InProgress, status.Ready, status.Completed, status.Blocked)
}

// printStatusOnelineContent returns the oneline string without printing
func printStatusOnelineContent(status *db.ProjectStatus) string {
	return fmt.Sprintf("%d running, %d queued, %d completed, %d blocked",
		status.InProgress, status.Ready, status.Completed, status.Blocked)
}

func printProgressBar(percent float64) {
	width := 40
	filled := int(percent / 100 * float64(width))

	fmt.Print("[")
	for i := 0; i < width; i++ {
		if i < filled {
			fmt.Print("█")
		} else {
			fmt.Print("░")
		}
	}
	fmt.Printf("] %.1f%%\n", percent)
}

// printProgressBarCompact prints a shorter progress bar
func printProgressBarCompact(percent float64) {
	width := 20
	filled := int(percent / 100 * float64(width))

	fmt.Print("[")
	for i := 0; i < width; i++ {
		if i < filled {
			fmt.Print("█")
		} else {
			fmt.Print("░")
		}
	}
	fmt.Printf("] %.1f%%\n", percent)
}

// printTreeStatus displays tasks in a hierarchical tree view
func printTreeStatus(store *db.Store) error {
	fmt.Println("\n🐂 Drover Task Tree")
	fmt.Println("════════════════════")

	// Get all tasks
	tasks, err := store.ListTasks()
	if err != nil {
		return fmt.Errorf("listing tasks: %w", err)
	}

	// Separate root tasks (no parent) from sub-tasks
	var rootTasks []*types.Task
	subTasks := make(map[string][]*types.Task) // parent_id -> children

	for _, task := range tasks {
		if task.ParentID == "" {
			rootTasks = append(rootTasks, task)
		} else {
			subTasks[task.ParentID] = append(subTasks[task.ParentID], task)
		}
	}

	// Print tree structure
	for _, root := range rootTasks {
		printTaskNode(root, subTasks, "", true)
	}

	return nil
}

// printTaskNode recursively prints a task and its children
func printTaskNode(task *types.Task, subTasks map[string][]*types.Task, prefix string, isLast bool) {
	// Task status icons
	statusIcon := map[types.TaskStatus]string{
		types.TaskStatusReady:      "⏳",
		types.TaskStatusClaimed:    "🔒",
		types.TaskStatusInProgress: "🔄",
		types.TaskStatusPaused:     "⏸",
		types.TaskStatusCompleted:  "✅",
		types.TaskStatusFailed:     "❌",
		types.TaskStatusBlocked:    "🚫",
	}
	icon := statusIcon[task.Status]
	if icon == "" {
		icon = "⏳"
	}

	// Print current task
	connector := "└── "
	if prefix == "" {
		connector = ""
	}
	fmt.Printf("%s%s%s %s: %s\n", prefix, connector, icon, task.ID, task.Title)

	// Print children if any
	children := subTasks[task.ID]
	if len(children) > 0 {
		newPrefix := prefix
		if prefix != "" {
			newPrefix = prefix + "    "
		} else {
			newPrefix = "    "
		}
		for i, child := range children {
			isLastChild := i == len(children)-1
			printTaskNode(child, subTasks, newPrefix, isLastChild)
		}
	}
}

func printTaskInfo(task *types.Task, blockedBy, blocking []string) {
	fmt.Println("\n📋 Task Info")
	fmt.Println("════════════")

	fmt.Printf("\nID:         %s\n", task.ID)
	fmt.Printf("Title:      %s\n", task.Title)
	fmt.Printf("Status:     %s\n", formatTaskStatus(task.Status))
	fmt.Printf("Priority:   %d\n", task.Priority)

	if task.Description != "" {
		fmt.Printf("\nDescription:\n")
		fmt.Printf("  %s\n", task.Description)
	}

	if task.EpicID != "" {
		fmt.Printf("\nEpic:       %s\n", task.EpicID)
	}

	// Timestamps
	fmt.Printf("\nCreated:    %s\n", formatTimestamp(task.CreatedAt))
	fmt.Printf("Updated:    %s\n", formatTimestamp(task.UpdatedAt))

	// Attempts
	if task.Attempts > 0 {
		fmt.Printf("Attempts:   %d / %d\n", task.Attempts, task.MaxAttempts)
	}

	// Claim info
	if task.ClaimedBy != "" {
		fmt.Printf("Claimed by: %s\n", task.ClaimedBy)
		if task.ClaimedAt != nil {
			fmt.Printf("Claimed at: %s\n", formatTimestamp(*task.ClaimedAt))
		}
	}

	// Error info
	if task.LastError != "" {
		fmt.Printf("\nLast Error:\n")
		fmt.Printf("  %s\n", task.LastError)
	}

	// Dependencies
	if len(blockedBy) > 0 {
		fmt.Printf("\nBlocked by:\n")
		for _, id := range blockedBy {
			fmt.Printf("  • %s\n", id)
		}
	}

	if len(blocking) > 0 {
		fmt.Printf("\nBlocking:\n")
		for _, id := range blocking {
			fmt.Printf("  • %s\n", id)
		}
	}

	fmt.Println()
}

func formatTaskStatus(status types.TaskStatus) string {
	switch status {
	case types.TaskStatusReady:
		return "🟢 ready"
	case types.TaskStatusClaimed:
		return "🟡 claimed"
	case types.TaskStatusInProgress:
		return "🔵 in_progress"
	case types.TaskStatusPaused:
		return "⏸️  paused"
	case types.TaskStatusBlocked:
		return "🚫 blocked"
	case types.TaskStatusCompleted:
		return "✅ completed"
	case types.TaskStatusFailed:
		return "❌ failed"
	default:
		return string(status)
	}
}

func formatTimestamp(timestamp int64) string {
	t := time.Unix(timestamp, 0)
	return t.Format("2006-01-02 15:04:05")
}

// worktreeCmd returns the worktree management command
func worktreeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "worktree",
		Short: "Manage git worktrees used for task execution",
		Long: `Manage git worktrees used for task execution.

Worktrees can consume significant disk space, especially with build artifacts
like Cargo targets (Rust) or node_modules (JavaScript). These commands help
manage and clean up worktrees to free up space.`,
	}

	cmd.AddCommand(
		worktreeListCmd(),
		worktreeCleanupCmd(),
		worktreePruneCmd(),
	)

	return cmd
}

// worktreeListCmd lists all worktrees with their status and disk usage
func worktreeListCmd() *cobra.Command {
	var verbose bool

	command := &cobra.Command{
		Use:   "list",
		Short: "List all worktrees with status and disk usage",
		RunE: func(cmd *cobra.Command, args []string) error {
			projectDir, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			worktreeDir := filepath.Join(projectDir, ".drover", "worktrees")

			// Get worktrees from database
			worktrees, err := store.ListWorktrees()
			if err != nil {
				return fmt.Errorf("querying worktrees: %w", err)
			}

			// Get worktrees on disk
			gitMgr := git.NewWorktreeManager(projectDir, worktreeDir)
			onDisk, _ := gitMgr.ListWorktreesOnDisk()

			// Build a map for quick lookup
			onDiskMap := make(map[string]bool)
			for _, id := range onDisk {
				onDiskMap[id] = true
			}

			if len(worktrees) == 0 && len(onDisk) == 0 {
				fmt.Println("No worktrees found")
				return nil
			}

			fmt.Println("\n🌳 Worktrees")
			fmt.Println("════════════")

			// Print tracked worktrees
			for _, w := range worktrees {
				onDiskIndicator := "✓"
				if !onDiskMap[w.TaskID] {
					onDiskIndicator = "✗ (missing)"
				}

				fmt.Printf("\n%s %s\n", onDiskIndicator, w.TaskID)
				fmt.Printf("  Status:    %s\n", w.Status)
				fmt.Printf("  Path:      %s\n", w.Path)
				if w.TaskTitle != "" {
					fmt.Printf("  Task:      %s\n", w.TaskTitle)
				}
				if w.TaskStatus != "" {
					fmt.Printf("  Task Status: %s\n", w.TaskStatus)
				}

				// Get disk usage
				if onDiskMap[w.TaskID] {
					size, _ := gitMgr.GetDiskUsage(w.TaskID)
					fmt.Printf("  Disk:      %s\n", formatBytes(size))

					// Show build artifacts if verbose
					if verbose {
						artifacts, _ := gitMgr.GetBuildArtifactSizes(w.TaskID)
						if len(artifacts) > 0 {
							fmt.Printf("  Artifacts:\n")
							for name, size := range artifacts {
								if size > 0 {
									fmt.Printf("    - %s: %s\n", name, formatBytes(size))
								}
							}
						}
					}
				}
			}

			// Print orphaned worktrees (on disk but not in database)
			var orphaned []string
			for _, id := range onDisk {
				found := false
				for _, w := range worktrees {
					if w.TaskID == id {
						found = true
						break
					}
				}
				if !found {
					orphaned = append(orphaned, id)
				}
			}

			if len(orphaned) > 0 {
				fmt.Println("\n👻 Orphaned (on disk but not tracked):")
				for _, id := range orphaned {
					size, _ := gitMgr.GetDiskUsage(id)
					fmt.Printf("  %s (%s)\n", id, formatBytes(size))
				}
			}

			return nil
		},
	}

	command.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show build artifact details")
	return command
}

// worktreeCleanupCmd removes all worktrees
func worktreeCleanupCmd() *cobra.Command {
	var force bool

	command := &cobra.Command{
		Use:   "cleanup",
		Short: "Remove all worktrees to free disk space",
		Long: `Remove all worktrees to free disk space.

This command aggressively removes all worktrees and their build artifacts
including target/, node_modules/, vendor/, etc.

Use --force to skip confirmation.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectDir, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			worktreeDir := filepath.Join(projectDir, ".drover", "worktrees")
			gitMgr := git.NewWorktreeManager(projectDir, worktreeDir)

			// Get list of worktrees to clean
			worktrees, err := store.ListWorktrees()
			if err != nil {
				return fmt.Errorf("querying worktrees: %w", err)
			}

			if len(worktrees) == 0 {
				// Check for orphaned worktrees
				onDisk, _ := gitMgr.ListWorktreesOnDisk()
				if len(onDisk) == 0 {
					fmt.Println("No worktrees to clean")
					return nil
				}
				fmt.Printf("Found %d orphaned worktrees (not tracked in database)\n", len(onDisk))
			} else {
				fmt.Printf("Found %d tracked worktrees\n", len(worktrees))
			}

			// Calculate total disk usage
			var totalSize int64
			for _, w := range worktrees {
				size, _ := gitMgr.GetDiskUsage(w.TaskID)
				totalSize += size
			}

			if totalSize > 0 {
				fmt.Printf("Total disk usage: %s\n", formatBytes(totalSize))
			}

			// Confirm unless --force
			if !force {
				fmt.Print("\nRemove all worktrees? [y/N] ")
				var response string
				fmt.Scanln(&response)
				if response != "y" && response != "Y" {
					fmt.Println("Aborted")
					return nil
				}
			}

			// Clean up all worktrees
			count, freed, err := gitMgr.CleanupAll()
			if err != nil {
				return fmt.Errorf("cleaning up worktrees: %w", err)
			}

			// Mark all worktrees as removed in database
			for _, w := range worktrees {
				store.UpdateWorktreeStatus(w.TaskID, "removed")
			}

			fmt.Printf("\n✅ Removed %d worktrees, freed %s\n", count, formatBytes(freed))
			return nil
		},
	}

	command.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation")
	return command
}

// worktreePruneCmd removes worktrees for completed/failed tasks
func worktreePruneCmd() *cobra.Command {
	var force bool
	var aggressive bool

	command := &cobra.Command{
		Use:   "prune",
		Short: "Remove worktrees for completed or failed tasks",
		Long: `Remove worktrees for completed or failed tasks.

This is safer than 'cleanup' as it only removes worktrees for tasks that
are no longer active (completed or failed).

Use --aggressive to also remove build artifacts (target/, node_modules/, etc.)
Use --force to skip confirmation.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectDir, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			worktreeDir := filepath.Join(projectDir, ".drover", "worktrees")
			gitMgr := git.NewWorktreeManager(projectDir, worktreeDir)

			// Get worktrees that can be pruned
			worktrees, err := store.GetWorktreesForCleanup(true) // completedOnly=true
			if err != nil {
				return fmt.Errorf("querying worktrees for cleanup: %w", err)
			}

			// Also check for orphaned worktrees upfront (on disk but not in database)
			orphanedPaths, err := store.GetOrphanedWorktrees(worktreeDir)
			if err != nil {
				return fmt.Errorf("listing orphaned worktrees: %w", err)
			}

			// Extract task IDs from orphaned paths
			orphanedTaskIDs := make([]string, 0, len(orphanedPaths))
			for _, path := range orphanedPaths {
				orphanedTaskIDs = append(orphanedTaskIDs, filepath.Base(path))
			}

			if len(worktrees) == 0 && len(orphanedTaskIDs) == 0 {
				fmt.Println("No worktrees to prune (no completed/failed tasks with worktrees)")
				return nil
			}

			// If we only have orphaned worktrees, clean them up
			if len(worktrees) == 0 && len(orphanedTaskIDs) > 0 {
				fmt.Printf("Found %d orphaned worktree(s) (not tracked in database)\n", len(orphanedTaskIDs))

				// Calculate sizes before removal
				orphanedSizes := make(map[string]int64)
				var totalOrphanedSize int64
				for _, taskID := range orphanedTaskIDs {
					if size, err := gitMgr.GetDiskUsage(taskID); err == nil {
						orphanedSizes[taskID] = size
						totalOrphanedSize += size
					}
				}
				if totalOrphanedSize > 0 {
					fmt.Printf("Total disk usage: %s\n", formatBytes(totalOrphanedSize))
				}

				fmt.Println("\nOrphaned worktrees to be removed:")
				for _, taskID := range orphanedTaskIDs {
					fmt.Printf("  - %s (%s)\n", taskID, formatBytes(orphanedSizes[taskID]))
				}

				// Confirm unless --force
				if !force {
					fmt.Print("\nRemove these orphaned worktrees? [y/N] ")
					var response string
					fmt.Scanln(&response)
					if response != "y" && response != "Y" {
						fmt.Println("Aborted")
						return nil
					}
				}

				// Remove orphaned worktrees
				var totalFreed int64
				for _, taskID := range orphanedTaskIDs {
					worktreePath := filepath.Join(worktreeDir, taskID)
					var freed int64
					var err error

					if aggressive {
						freed, err = gitMgr.RemoveAggressiveByPath(worktreePath)
					} else {
						err = gitMgr.RemoveByPath(worktreePath)
						// For non-aggressive removal, use the pre-calculated size
						freed = orphanedSizes[taskID]
					}

					if err != nil {
						fmt.Printf("⚠️  Failed to remove %s: %v\n", taskID, err)
						continue
					}
					totalFreed += freed
				}

				fmt.Printf("\n✅ Pruned %d orphaned worktrees, freed %s\n", len(orphanedTaskIDs), formatBytes(totalFreed))
				return nil
			}

			fmt.Printf("Found %d worktrees for completed/failed tasks\n", len(worktrees))

			// Calculate total disk usage
			var totalSize int64
			for _, w := range worktrees {
				size, _ := gitMgr.GetDiskUsage(w.TaskID)
				totalSize += size
			}

			if totalSize > 0 {
				fmt.Printf("Total disk usage: %s\n", formatBytes(totalSize))
			}

			fmt.Println("\nWorktrees to be removed:")
			for _, w := range worktrees {
				size, _ := gitMgr.GetDiskUsage(w.TaskID)
				fmt.Printf("  - %s: %s (%s)\n", w.TaskID, w.TaskTitle, formatBytes(size))
			}

			// Confirm unless --force
			if !force {
				fmt.Print("\nRemove these worktrees? [y/N] ")
				var response string
				fmt.Scanln(&response)
				if response != "y" && response != "Y" {
					fmt.Println("Aborted")
					return nil
				}
			}

			// Remove each worktree
			var totalFreed int64
			for _, w := range worktrees {
				var freed int64
				var err error

				if aggressive {
					freed, err = gitMgr.RemoveAggressive(w.TaskID)
				} else {
					err = gitMgr.Remove(w.TaskID)
				}

				if err != nil {
					fmt.Printf("⚠️  Failed to remove %s: %v\n", w.TaskID, err)
					continue
				}

				totalFreed += freed
				store.UpdateWorktreeStatus(w.TaskID, "removed")
				store.DeleteWorktree(w.TaskID)
			}

			fmt.Printf("\n✅ Pruned %d worktrees, freed %s\n", len(worktrees), formatBytes(totalFreed))

			// Prune orphaned worktrees too
			orphaned, orphanedFreed, err := gitMgr.PruneOrphaned()
			if err == nil && len(orphaned) > 0 {
				fmt.Printf("Also removed %d orphaned worktrees, freed %s\n", len(orphaned), formatBytes(orphanedFreed))
			}

			return nil
		},
	}

	command.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation")
	command.Flags().BoolVarP(&aggressive, "aggressive", "a", false, "Remove build artifacts (target/, node_modules/, etc.)")
	return command
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

// dashboardCmd starts the web dashboard
func dashboardCmd() *cobra.Command {
	var (
		port string
		open bool
	)

	command := &cobra.Command{
		Use:   "dashboard",
		Short: "Start the web dashboard",
		Long:  `Start a local web dashboard for visualizing project progress, tasks, and workers in real-time.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectDir, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			return runDashboard(store, projectDir, port, open)
		},
	}

	command.Flags().StringVarP(&port, "port", "p", "3847", "Port to run dashboard on")
	command.Flags().BoolVar(&open, "open", false, "Open browser automatically")
	return command
}

func runDashboard(store *db.Store, projectDir string, port string, openBrowser bool) error {
	// Import dashboard package
	dash := dashboard.Config{
		Addr:        ":" + port,
		DatabaseURL: filepath.Join(projectDir, ".drover", "drover.db"),
		Store:       store,
	}

	server, err := dashboard.New(dash)
	if err != nil {
		return fmt.Errorf("creating dashboard: %w", err)
	}

	// Set global dashboard for event broadcasting
	dashboard.SetGlobal(server)

	// Open browser if requested
	if openBrowser {
		go func() {
			time.Sleep(500 * time.Millisecond)
			url := fmt.Sprintf("http://localhost:%s", port)
			var cmd *exec.Cmd
			switch runtime.GOOS {
			case "darwin":
				cmd = exec.Command("open", url)
			case "windows":
				cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
			default: // linux, bsd, etc.
				cmd = exec.Command("xdg-open", url)
			}
			_ = cmd.Run()
		}()
	}

	return server.Start()
}
