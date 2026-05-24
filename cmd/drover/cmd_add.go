package main

import (
	"fmt"
	"strings"

	"github.com/cloud-shuttle/drover/internal/db"
	"github.com/cloud-shuttle/drover/internal/template"
	"github.com/cloud-shuttle/drover/pkg/types"
	"github.com/spf13/cobra"
)

func addCmd() *cobra.Command {
	var (
		desc         string
		epicID       string
		parentID     string
		priority     int
		blockedBy    []string
		skipValidation bool
		testMode     string
		testScope    string
		testCommand  string
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

Maximum depth is 2 levels (Epic → Parent → Child).

Automated Testing:
  Use --test-mode to configure when tests run:
    strict     (default) Tests must pass for task to complete
    lenient    Tests run but failures don't block completion
    disabled   Tests are not run
  Use --test-scope to configure which tests run:
    diff       (default) Only run tests if files changed
    all        Always run all tests
    skip       Skip running tests
  Use --test-command for custom test command (e.g., "make test-unit")`,
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
						// Set test configuration on sub-task if specified
						if testMode != "" || testScope != "" || testCommand != "" {
							if err := store.SetTaskTestConfig(subTask.ID, testMode, testScope, testCommand); err != nil {
								return fmt.Errorf("setting test configuration: %w", err)
							}
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
				// Create regular task with test configuration
				task, err = store.CreateTaskWithTestConfig(title, desc, epicID, priority, blockedBy, "", testMode, testScope, testCommand)
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
	command.Flags().StringVar(&testMode, "test-mode", "", "Test execution mode: strict (block on failure), lenient (warn only), disabled")
	command.Flags().StringVar(&testScope, "test-scope", "", "Test scope: diff (only if changed), all (always), skip")
	command.Flags().StringVar(&testCommand, "test-command", "", "Custom test command (e.g., 'make test-unit')")
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
