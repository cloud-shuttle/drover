package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cloud-shuttle/drover/internal/db"
	"github.com/cloud-shuttle/drover/pkg/types"
	"github.com/spf13/cobra"
)

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
