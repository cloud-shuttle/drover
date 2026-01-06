package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloud-shuttle/drover/internal/db"
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

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Execute all tasks to completion",
		Long: `Run all tasks to completion using parallel Claude Code agents.

Tasks are executed respecting dependencies and priorities. Use --workers
to control parallelism. Use --epic to filter execution to a specific epic.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			// TODO: Implement DBOS workflow orchestration
			fmt.Printf("🐂 Starting run with %d workers\n", workers)
			if epicID != "" {
				fmt.Printf("Epic filter: %s\n", epicID)
			}

			// For now, just show status
			status, err := store.GetProjectStatus()
			if err != nil {
				return err
			}

			printStatus(status)
			return nil
		},
	}

	cmd.Flags().IntVarP(&workers, "workers", "w", 4, "Number of parallel workers")
	cmd.Flags().StringVar(&epicID, "epic", "", "Filter to specific epic")

	return cmd
}

func addCmd() *cobra.Command {
	var (
		desc      string
		epicID    string
		priority  int
		blockedBy []string
	)

	return &cobra.Command{
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

	cmd.Flags().StringVarP(&desc, "description", "d", "", "Task description")
	cmd.Flags().StringVarP(&epicID, "epic", "e", "", "Assign to epic")
	cmd.Flags().IntVarP(&priority, "priority", "p", 0, "Task priority (higher = more urgent)")
	cmd.Flags().StringSliceVar(&blockedBy, "blocked-by", nil, "Task IDs this depends on")
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

	return &cobra.Command{
		Use:   "epic",
		Short: "Manage epics",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
}

func statusCmd() *cobra.Command {
	var watchMode bool

	return &cobra.Command{
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

	cmd.Flags().BoolVarP(&watchMode, "watch", "w", false, "Watch mode - live updates")
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
