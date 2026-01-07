package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/cloud-shuttle/drover/internal/db"
	"github.com/cloud-shuttle/drover/pkg/types"
	"github.com/cloud-shuttle/drover/internal/workflow"
	"github.com/spf13/cobra"
)

func initCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize Drover in the current project",
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

			fmt.Printf("🐂 Initialized Drover in %s\n", droverDir)
			fmt.Println("\nNext steps:")
			fmt.Println("  drover epic add \"My Epic\"")
			fmt.Println("  drover add \"My first task\" --epic <epic-id>")
			fmt.Println("  drover run")

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
to control parallelism. Use --epic to filter execution to a specific epic.`,
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

			// Create orchestrator
			orch, err := workflow.NewOrchestrator(&runCfg, store, projectDir)
			if err != nil {
				return fmt.Errorf("creating orchestrator: %w", err)
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
		},
	}

	cmd.Flags().IntVarP(&workers, "workers", "w", 0, "Number of parallel workers")
	cmd.Flags().StringVar(&epicID, "epic", "", "Filter to specific epic (not yet implemented)")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging for debugging")

	return cmd
}

func addCmd() *cobra.Command {
	var (
		desc      string
		epicID    string
		priority  int
		blockedBy []string
	)

	command := &cobra.Command{
		Use:   "add <title>",
		Short: "Add a new task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			task, err := store.CreateTask(args[0], desc, epicID, priority, blockedBy)
			if err != nil {
				return err
			}

			fmt.Printf("✅ Created task %s\n", task.ID)
			return nil
		},
	}

	command.Flags().StringVarP(&desc, "description", "d", "", "Task description")
	command.Flags().StringVarP(&epicID, "epic", "e", "", "Assign to epic")
	command.Flags().IntVarP(&priority, "priority", "p", 0, "Task priority (higher = more urgent)")
	command.Flags().StringSliceVar(&blockedBy, "blocked-by", nil, "Task IDs this depends on")
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
				// TODO: Implement watch mode
				fmt.Println("Watch mode not yet implemented")
				return nil
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
	return command
}

func resumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resume",
		Short: "Resume interrupted workflows",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			// TODO: Implement DBOS workflow recovery
			fmt.Println("🐂 Resuming workflows...")

			// Check for incomplete workflows
			status, err := store.GetProjectStatus()
			if err != nil {
				return err
			}

			if status.InProgress == 0 {
				fmt.Println("No incomplete workflows found")
				return nil
			}

			fmt.Printf("Found %d incomplete tasks\n", status.InProgress)
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
