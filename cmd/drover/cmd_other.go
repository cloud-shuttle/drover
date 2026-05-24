package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloud-shuttle/drover/internal/events"
	"github.com/cloud-shuttle/drover/pkg/types"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

// resumeCmd resumes interrupted workflows
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

// infoCmd shows detailed information about a specific task
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

// resetCmd resets tasks back to ready status
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

			// Record paused event
			eventID := uuid.New().String()
			timestamp := time.Now().Unix()
			_ = store.RecordEvent(eventID, string(events.EventTaskPaused), timestamp, taskID, task.EpicID, "")

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

			// Record resumed event
			eventID := uuid.New().String()
			timestamp := time.Now().Unix()
			var dataJSON string
			if hint != "" {
				data, _ := json.Marshal(map[string]any{"hint": hint})
				dataJSON = string(data)
			}
			_ = store.RecordEvent(eventID, string(events.EventTaskResumed), timestamp, taskID, task.EpicID, dataJSON)

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

// cancelCmd cancels a running or ready task
func cancelCmd() *cobra.Command {
	var reason string

	command := &cobra.Command{
		Use:   "cancel <task-id>",
		Short: "Cancel a task",
		Long: `Cancel a running, ready, or claimed task.

Canceling a task will:
  - Stop the task's execution immediately
  - Mark the task as 'cancelled'
  - Release any claims on the task
  - Record the cancellation reason

Use 'drover retry' to retry a cancelled task if needed.`,
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

			// Check if task can be cancelled
			if task.Status != types.TaskStatusReady && task.Status != types.TaskStatusClaimed && task.Status != types.TaskStatusInProgress {
				return fmt.Errorf("cannot cancel task with status '%s'", task.Status)
			}

			// Cancel the task
			if err := store.CancelTask(taskID, reason); err != nil {
				return fmt.Errorf("cancelling task: %w", err)
			}

			// Record cancellation event
			eventID := uuid.New().String()
			timestamp := time.Now().Unix()
			var dataJSON string
			if reason != "" {
				data, _ := json.Marshal(map[string]any{"reason": reason})
				dataJSON = string(data)
			}
			_ = store.RecordEvent(eventID, string(events.EventTaskCancelled), timestamp, taskID, task.EpicID, dataJSON)

			fmt.Printf("✅ Cancelled task %s\n", taskID)
			fmt.Printf("   %s\n", task.Title)
			if reason != "" {
				fmt.Printf("   Reason: %s\n", reason)
			}

			return nil
		},
	}

	command.Flags().StringVar(&reason, "reason", "", "Reason for cancellation (optional)")
	return command
}

// retryCmd retries a failed or cancelled task
func retryCmd() *cobra.Command {
	var force bool

	command := &cobra.Command{
		Use:   "retry <task-id>",
		Short: "Retry a failed or cancelled task",
		Long: `Retry a failed or cancelled task.

Retrying a task will:
  - Reset the task to 'ready' status
  - Increment the attempt counter
  - Clear any previous error messages

By default, the task retains its attempt count. If the task has
reached max_attempts, use --force to reset the counter and allow
additional attempts.`,
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

			// Check if task can be retried
			if task.Status != types.TaskStatusFailed && task.Status != types.TaskStatusCancelled {
				return fmt.Errorf("cannot retry task with status '%s'", task.Status)
			}

			// Check if force is needed
			if task.Attempts >= task.MaxAttempts && !force {
				fmt.Printf("⚠️  Task has reached max attempts (%d/%d)\n", task.Attempts, task.MaxAttempts)
				fmt.Println("Use --force to reset the attempt counter and retry anyway.")
				return fmt.Errorf("max attempts reached")
			}

			// Retry the task
			if err := store.RetryTask(taskID, force); err != nil {
				return fmt.Errorf("retrying task: %w", err)
			}

			fmt.Printf("✅ Retrying task %s\n", taskID)
			fmt.Printf("   %s\n", task.Title)
			if force {
				fmt.Printf("   Attempt counter reset (will be attempt 1/%d)\n", task.MaxAttempts)
			} else {
				fmt.Printf("   Will be attempt %d/%d\n", task.Attempts+1, task.MaxAttempts)
			}

			return nil
		},
	}

	command.Flags().BoolVar(&force, "force", false, "Reset attempt counter to retry even if max attempts reached")
	return command
}

// resolveCmd removes blockers from a blocked task
func resolveCmd() *cobra.Command {
	var note string

	command := &cobra.Command{
		Use:   "resolve <task-id>",
		Short: "Resolve blockers for a blocked task",
		Long: `Resolve all blockers for a blocked task, setting it back to ready.

Resolving a task will:
  - Remove all 'blocked-by' dependencies
  - Set the task status to 'ready'
  - Record the resolution note

Use this when you have manually fixed the issues that were blocking
the task and want to allow it to proceed.`,
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

			// Check if task is blocked
			if task.Status != types.TaskStatusBlocked {
				return fmt.Errorf("cannot resolve task with status '%s'", task.Status)
			}

			// Get blockers count for confirmation
			blockers, err := store.GetBlockedBy(taskID)
			if err != nil {
				return fmt.Errorf("getting blockers: %w", err)
			}

			// Resolve the task
			if err := store.ResolveTask(taskID, note); err != nil {
				return fmt.Errorf("resolving task: %w", err)
			}

			// Record unblocked event
			eventID := uuid.New().String()
			timestamp := time.Now().Unix()
			var dataJSON string
			if note != "" {
				data, _ := json.Marshal(map[string]any{"note": note})
				dataJSON = string(data)
			}
			_ = store.RecordEvent(eventID, string(events.EventTaskUnblocked), timestamp, taskID, task.EpicID, dataJSON)

			fmt.Printf("✅ Resolved task %s\n", taskID)
			fmt.Printf("   %s\n", task.Title)
			fmt.Printf("   Removed %d blocker(s)\n", len(blockers))
			if note != "" {
				fmt.Printf("   Note: %s\n", note)
			}

			return nil
		},
	}

	command.Flags().StringVar(&note, "note", "", "Note about how the issue was resolved (optional)")
	return command
}

// streamCmd streams task lifecycle events in real-time
func streamCmd() *cobra.Command {
	var (
		eventTypes []string
		epicID     string
		taskID     string
		since      string
		until      string
		follow     bool
		jsonLines  bool
		quiet      bool
		limit      int
	)

	command := &cobra.Command{
		Use:   "stream",
		Short: "Stream task lifecycle events",
		Long: `Stream task lifecycle events in real-time.

Supported event types:
  - task.started:    Task execution began
  - task.completed:  Task completed successfully
  - task.failed:     Task failed
  - task.blocked:    Task was blocked by dependencies
  - task.unblocked:  Task blockers were resolved
  - task.cancelled:  Task was cancelled
  - task.claimed:    Task was claimed by a worker
  - task.paused:     Task was paused
  - task.resumed:    Task was resumed

Examples:
  # Stream all events in JSONL format
  drover stream --jsonl

  # Filter by event type
  drover stream --type completed,failed

  # Filter by epic
  drover stream --epic epic-abc

  # Include historical events since a timestamp
  drover stream --since 2024-01-01T00:00:00Z

  # Follow for new events (real-time streaming)
  drover stream --follow`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			ctx := cmd.Context()

			// Parse time filters
			var sinceTS, untilTS int64
			if since != "" {
				t, err := time.Parse(time.RFC3339, since)
				if err != nil {
					return fmt.Errorf("parsing --since timestamp: %w", err)
				}
				sinceTS = t.Unix()
			}
			if until != "" {
				t, err := time.Parse(time.RFC3339, until)
				if err != nil {
					return fmt.Errorf("parsing --until timestamp: %w", err)
				}
				untilTS = t.Unix()
			}

			// Build event type filter
			var types []string
			for _, t := range eventTypes {
				// Add "task." prefix if not present
				if !strings.HasPrefix(t, "task.") {
					t = "task." + t
				}
				types = append(types, t)
			}
			if len(types) == 0 {
				// Default to all event types if none specified
				types = []string{
					"task.started", "task.completed", "task.failed",
					"task.blocked", "task.unblocked", "task.cancelled",
					"task.claimed", "task.paused", "task.resumed",
				}
			}

			// Query historical events first
			if !follow {
				events, err := store.QueryEvents(types, epicID, taskID, sinceTS, untilTS, limit)
				if err != nil {
					return fmt.Errorf("querying events: %w", err)
				}

				if jsonLines {
					// Output in JSONL format
					for _, e := range events {
						data, err := json.Marshal(e)
						if err != nil {
							return fmt.Errorf("marshaling event: %w", err)
						}
						fmt.Println(string(data))
					}
				} else {
					// Output in human-readable format
					if len(events) == 0 && !quiet {
						fmt.Println("No events found.")
					}
					for _, e := range events {
						printEvent(e)
					}
				}
				return nil
			}

			// Follow mode: stream events in real-time
			// For now, just show historical events and note that follow is not yet implemented
			if !quiet {
				fmt.Println("📡 Streaming events...")
				fmt.Println("Note: Real-time follow mode will be implemented in a future update.")
				fmt.Println()
			}

			events, err := store.QueryEvents(types, epicID, taskID, sinceTS, untilTS, limit)
			if err != nil {
				return fmt.Errorf("querying events: %w", err)
			}

			if jsonLines {
				for _, e := range events {
					data, err := json.Marshal(e)
					if err != nil {
						return fmt.Errorf("marshaling event: %w", err)
					}
					fmt.Println(string(data))
				}
			} else {
				for _, e := range events {
					printEvent(e)
				}
			}

			// In follow mode, we would poll for new events here
			// For now, just show a message
			if !quiet && len(events) > 0 {
				fmt.Println()
				fmt.Println("Waiting for new events... (Ctrl+C to exit)")
				<-ctx.Done()
			}

			return nil
		},
	}

	command.Flags().StringSliceVar(&eventTypes, "type", []string{}, "Filter by event type (comma-separated)")
	command.Flags().StringVar(&epicID, "epic", "", "Filter by epic ID")
	command.Flags().StringVar(&taskID, "task", "", "Filter by task ID")
	command.Flags().StringVar(&since, "since", "", "Include events since timestamp (RFC3339)")
	command.Flags().StringVar(&until, "until", "", "Include events until timestamp (RFC3339)")
	command.Flags().BoolVar(&follow, "follow", false, "Follow for new events (real-time streaming)")
	command.Flags().BoolVar(&jsonLines, "jsonl", false, "Output in JSON Lines format")
	command.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress informational messages")
	command.Flags().IntVar(&limit, "limit", 0, "Maximum number of events to return")

	return command
}

// printEvent prints an event in a human-readable format
func printEvent(event map[string]any) {
	eventType, _ := event["type"].(string)
	timestamp, _ := event["timestamp"].(int64)
	taskID, _ := event["task_id"].(string)

	// Format timestamp
	t := time.Unix(timestamp, 0)
	timeStr := t.Format("2006-01-02 15:04:05")

	// Format event type
	var emoji string
	switch eventType {
	case "task.started":
		emoji = "🚀"
	case "task.completed":
		emoji = "✅"
	case "task.failed":
		emoji = "❌"
	case "task.blocked":
		emoji = "🚧"
	case "task.unblocked":
		emoji = "🔓"
	case "task.cancelled":
		emoji = "⛔"
	case "task.claimed":
		emoji = "🤖"
	case "task.paused":
		emoji = "⏸️"
	case "task.resumed":
		emoji = "▶️"
	default:
		emoji = "📡"
	}

	fmt.Printf("%s [%s] %s task=%s", emoji, timeStr, eventType, taskID)

	if epicID, ok := event["epic_id"].(string); ok && epicID != "" {
		fmt.Printf(" epic=%s", epicID)
	}

	if data, ok := event["data"].(string); ok && data != "" {
		fmt.Printf(" data=%s", data)
	}

	fmt.Println()
}

// flagsCmd manages feature flags
func flagsCmd() *cobra.Command {
	command := &cobra.Command{
		Use:   "flags",
		Short: "Manage feature flags",
		Long:  `Manage feature flags for experimental features.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Feature flags management is coming soon.")
			fmt.Println("This will allow enabling/disabling experimental features.")
			return nil
		},
	}
	return command
}

// searchCmd performs full-text search across tasks
func searchCmd() *cobra.Command {
	var query string

	command := &cobra.Command{
		Use:   "search <query>",
		Short: "Full-text search across tasks and outputs",
		Long:  `Search across task titles, descriptions, and execution outputs.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query = args[0]
			_, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			// For now, just show a placeholder
			fmt.Printf("Searching for: %s\n", query)
			fmt.Println("Full-text search will be implemented in a future update.")
			return nil
		},
	}
	return command
}

// backpressureCmd manages backpressure validation checks
func backpressureCmd() *cobra.Command {
	command := &cobra.Command{
		Use:   "backpressure",
		Short: "Manage backpressure validation checks",
		Long: `Configure and monitor backpressure settings for adaptive concurrency control.

This helps prevent OOM by reducing worker spawning when Claude API is rate-limited.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Backpressure management is coming soon.")
			fmt.Println("This will be part of the memory management improvements.")
			return nil
		},
	}
	return command
}

// proxyCmd manages the LLM proxy server
func proxyCmd() *cobra.Command {
	command := &cobra.Command{
		Use:   "proxy",
		Short: "Manage the LLM proxy server",
		Long:  `Configure and manage the proxy server for LLM API requests.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Proxy server management is coming soon.")
			return nil
		},
	}
	return command
}
