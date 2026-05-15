// Command to demonstrate DBOS workflow execution
// This is a proof of concept showing how Drover could use DBOS

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/cloud-shuttle/drover/internal/workflow"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
	"github.com/spf13/cobra"
)

// dbosDemoCmd returns a command that demonstrates DBOS-based workflow execution
func dbosDemoCmd() *cobra.Command {
	var useQueue bool

	cmd := &cobra.Command{
		Use:   "dbos-demo",
		Short: "Run a demonstration of DBOS-based workflow execution",
		Long: `This command demonstrates how Drover would work using DBOS for durable workflow execution.

DBOS provides:
- Automatic state checkpointing and recovery
- Built-in retry logic for steps
- Exactly-once execution guarantees
- PostgreSQL-backed durability
- Queue-based parallel execution

This is a proof of concept for migrating Drover to DBOS.

Note: This requires PostgreSQL to be running and DBOS_SYSTEM_DATABASE_URL to be set.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get project directory
			projectDir, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("failed to get working directory: %w", err)
			}

			// Initialize DBOS context
			dbURL := os.Getenv("DBOS_SYSTEM_DATABASE_URL")
			if dbURL == "" {
				// Default to local Postgres
				dbURL = "postgres://postgres:postgres@localhost:5432/drover?sslmode=disable"
			}

			dbosCtx, err := dbos.NewDBOSContext(context.Background(), dbos.Config{
				AppName:     "drover-poc",
				DatabaseURL: dbURL,
			})
			if err != nil {
				return fmt.Errorf("failed to initialize DBOS: %w", err)
			}

			// Create DBOS orchestrator BEFORE launching DBOS (so queues can be registered)
			// Note: Passing nil for store since this is a demo without a full project
			orchestrator, err := workflow.NewDBOSOrchestrator(cfg, dbosCtx, projectDir, nil)
			if err != nil {
				return fmt.Errorf("failed to create orchestrator: %w", err)
			}

			// Register workflows BEFORE launching DBOS
			if err := orchestrator.RegisterWorkflows(); err != nil {
				return fmt.Errorf("failed to register workflows: %w", err)
			}

			// Launch DBOS runtime AFTER registering workflows and queues
			if err := dbos.Launch(dbosCtx); err != nil {
				return fmt.Errorf("failed to launch DBOS: %w", err)
			}
			defer dbos.Shutdown(dbosCtx, 5*time.Second)

			// Create sample tasks for demo - including dependencies
			tasks := []workflow.TaskInput{
				{
					TaskID:      "task-demo-1",
					Title:       "Add error handling",
					Description: "Add proper error handling to the main function",
					Priority:    1,
					MaxAttempts: 3,
				},
				{
					TaskID:      "task-demo-2",
					Title:       "Write tests",
					Description: "Add unit tests for the configuration module",
					Priority:    1,
					MaxAttempts: 3,
				},
				{
					TaskID:      "task-demo-3",
					Title:       "Add logging",
					Description: "Add structured logging throughout the application",
					Priority:    1,
					MaxAttempts: 3,
				},
				{
					TaskID:      "task-demo-4",
					Title:       "Update documentation",
					Description: "Update README with new features",
					Priority:    0,
					MaxAttempts: 3,
					BlockedBy:   []string{"task-demo-1"}, // Depends on task-1
				},
			}

			fmt.Println("\n🚀 Starting DBOS Demo")
			fmt.Println("===================")
			fmt.Printf("Database: %s\n", dbURL)
			fmt.Printf("Tasks: %d\n", len(tasks))
			if useQueue {
				fmt.Printf("Mode:     Queue-based (parallel)\n")
			} else {
				fmt.Printf("Mode:     Sequential\n")
			}
			fmt.Println()

			if useQueue {
				// Execute with queue (parallel execution)
				// Use ExecuteTasksWithQueueDirectly to avoid child workflow issues
				stats, err := orchestrator.ExecuteTasksWithQueueDirectly(tasks)
				if err != nil {
					return fmt.Errorf("queue execution failed: %w", err)
				}

				// Print results
				orchestrator.PrintQueueStats(stats)
			} else {
				// Execute sequentially (original approach)
				handle, err := dbos.RunWorkflow(dbosCtx, orchestrator.ExecuteAllTasks, tasks)
				if err != nil {
					return fmt.Errorf("failed to start workflow: %w", err)
				}

				// Wait for results
				results, err := handle.GetResult()
				if err != nil {
					return fmt.Errorf("workflow execution failed: %w", err)
				}

				// Print results
				orchestrator.PrintResults(results)
			}

			fmt.Println("\n✅ Demo complete!")
			fmt.Println("\n💡 Key differences from SQLite-based implementation:")
			fmt.Println("  • State is automatically checkpointed to PostgreSQL")
			fmt.Println("  • Each step can be independently recovered on failure")
			fmt.Println("  • No manual task status management needed")
			fmt.Println("  • Retries are built into the step execution")
			if useQueue {
				fmt.Println("  • Queue-based parallel execution with automatic worker pooling")
			}
			fmt.Println("\n📝 To use in production:")
			fmt.Println("  1. Set DBOS_SYSTEM_DATABASE_URL to your PostgreSQL instance")
			fmt.Println("  2. Replace 'drover run' with DBOS-based workflow execution")
			fmt.Println("  3. Remove SQLite database and manual state management")
			fmt.Println("  4. Configure queue workers based on your concurrency needs")

			return nil
		},
	}

	cmd.Flags().BoolVar(&useQueue, "queue", false, "Use queue-based parallel execution")
	return cmd
}
