package worker

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
)

// CLI represents the worker command-line interface
type CLI struct {
	rootCmd *cobra.Command
}

// NewCLI creates a new CLI instance
func NewCLI() *CLI {
	cli := &CLI{}

	cli.rootCmd = &cobra.Command{
		Use:   "drover-worker",
		Short: "Process-isolated task executor for Drover",
		Long: `drover-worker executes tasks in separate processes to ensure memory is reclaimed
when tasks complete. This prevents OOM issues in the main Drover orchestrator.`,
		Version: "0.1.0",
	}

	// Add execute command
	cli.rootCmd.AddCommand(cli.executeCmd())

	return cli
}

// Execute runs the CLI
func (cli *CLI) Execute() error {
	return cli.rootCmd.Execute()
}

// executeCmd handles the execute command
func (cli *CLI) executeCmd() *cobra.Command {
	var (
		taskID      string
		worktree    string
		title       string
		description string
		epicID      string
		guidance    []string
		timeout     string
		claudePath  string
		verbose     bool
		memoryLimit string
	)

	cmd := &cobra.Command{
		Use:   "execute [flags|-]",
		Short: "Execute a task",
		Long: `Execute a task using Claude Code.

The task can be specified via flags or provided as JSON via stdin.
Use "-" as the first argument to read from stdin.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var input TaskInput

			// Check if reading from stdin
			if len(args) > 0 && args[0] == "-" {
				data, err := io.ReadAll(os.Stdin)
				if err != nil {
					return fmt.Errorf("failed to read stdin: %w", err)
				}
				if err := json.Unmarshal(data, &input); err != nil {
					return fmt.Errorf("failed to parse input JSON: %w", err)
				}
			} else {
				// Build input from flags
				if taskID == "" {
					return errors.New("--task-id is required")
				}
				if worktree == "" {
					return errors.New("--worktree is required")
				}
				if title == "" {
					return errors.New("--title is required")
				}
				if description == "" {
					return errors.New("--description is required")
				}

				input = TaskInput{
					ID:          taskID,
					Title:       title,
					Description: description,
					EpicID:      epicID,
					Worktree:    worktree,
					Guidance:    guidance,
					Timeout:     timeout,
					ClaudePath:  claudePath,
					Verbose:     verbose,
					MemoryLimit: memoryLimit,
				}
			}

			// Apply defaults
			if input.Timeout == "" {
				input.Timeout = DefaultTimeout.String()
			}
			if input.ClaudePath == "" {
				input.ClaudePath = "claude"
			}

			// Parse timeout
			duration, err := time.ParseDuration(input.Timeout)
			if err != nil {
				return fmt.Errorf("invalid timeout: %w", err)
			}

			// Create executor and run task
			executor := NewExecutor(input.ClaudePath, duration, input.Verbose)
			result := executor.Execute(&input)

			// Output result as JSON
			encoder := json.NewEncoder(os.Stdout)
			encoder.SetIndent("", "  ")
			if err := encoder.Encode(result); err != nil {
				return fmt.Errorf("failed to encode result: %w", err)
			}

			// Exit with appropriate code
			if !result.Success {
				os.Exit(1)
			}
			return nil
		},
	}

	// Flags
	cmd.Flags().StringVar(&taskID, "task-id", "", "Task ID (required)")
	cmd.Flags().StringVar(&worktree, "worktree", "", "Path to git worktree (required)")
	cmd.Flags().StringVar(&title, "title", "", "Task title (required)")
	cmd.Flags().StringVar(&description, "description", "", "Task description (required)")
	cmd.Flags().StringVar(&epicID, "epic-id", "", "Parent epic ID")
	cmd.Flags().StringArrayVar(&guidance, "guidance", []string{}, "Guidance messages (can be specified multiple times)")
	cmd.Flags().StringVar(&timeout, "timeout", "", "Task timeout (default: 30m)")
	cmd.Flags().StringVar(&claudePath, "claude-path", "", "Path to Claude binary (default: claude)")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Enable verbose logging")
	cmd.Flags().StringVar(&memoryLimit, "memory-limit", "", "Worker memory limit (e.g., 512M, 2G)")

	return cmd
}
