package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/cloud-shuttle/drover/internal/config"
	"github.com/cloud-shuttle/drover/internal/db"
	"github.com/cloud-shuttle/drover/internal/git"
	"github.com/cloud-shuttle/drover/internal/template"
	"github.com/cloud-shuttle/drover/pkg/types"
	"github.com/cloud-shuttle/drover/internal/workflow"
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

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Execute all tasks to completion",
		Long: `Run all tasks to completion using parallel Claude Code agents.

Tasks are executed respecting dependencies and priorities. Use --workers
to control parallelism. Use --epic to filter execution to a specific epic.

DBOS Workflow Engine:
- Default: SQLite-based orchestration (zero setup)
- With DBOS_SYSTEM_DATABASE_URL: DBOS with PostgreSQL (production mode)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectDir, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			// Override config if workers flag specified
			runCfg := *cfg
			if workers > 0 {
				runCfg.Workers = workers
			}
			runCfg.Verbose = verbose

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
		desc      string
		epicID    string
		parentID  string
		priority  int
		blockedBy []string
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
		Args:  cobra.ExactArgs(1),
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
	command.AddCommand(epicAdd)
	return command
}

func statusCmd() *cobra.Command {
	var watchMode bool
	var treeMode bool

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

			printStatus(status)
			return nil
		},
	}

	command.Flags().BoolVarP(&watchMode, "watch", "w", false, "Watch mode - live updates")
	command.Flags().BoolVarP(&treeMode, "tree", "t", false, "Tree mode - show hierarchical view")
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
		resetCompleted bool
		resetInProgress bool
		resetClaimed bool
		resetFailed bool
	)

	command := &cobra.Command{
		Use:   "reset",
		Short: "Reset tasks back to ready status",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

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
	return &cobra.Command{
		Use:   "export",
		Short: "Export tasks to beads format",
		RunE: func(cmd *cobra.Command, args []string) error {
			projectDir, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

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
		},
	}
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
	fmt.Printf("Completed:  %d\n", status.Completed)
	fmt.Printf("Failed:     %d\n", status.Failed)
	fmt.Printf("Blocked:    %d\n", status.Blocked)

	if status.Total > 0 {
		progress := float64(status.Completed) / float64(status.Total) * 100
		fmt.Printf("\nProgress: %.1f%%\n", progress)
		printProgressBar(progress)
	}
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
	case types.TaskStatusBlocked:
		return "⏸️  blocked"
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
			_, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			// Import dashboard package
			return runDashboard(store, port, open)
		},
	}

	command.Flags().StringVarP(&port, "port", "p", "3847", "Port to run dashboard on")
	command.Flags().BoolVar(&open, "open", false, "Open browser automatically")
	return command
}
