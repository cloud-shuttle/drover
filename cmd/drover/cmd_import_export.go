package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cloud-shuttle/drover/internal/config"
	"github.com/cloud-shuttle/drover/internal/db"
	"github.com/cloud-shuttle/drover/internal/workflow"
	"github.com/cloud-shuttle/drover/pkg/types"
	"github.com/spf13/cobra"
)

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
		"version":   "1.0",
		"exportedAt": time.Now().Format(time.RFC3339),
		"repository": projectDir,
		"tasks":     tasks,
		"epics":     epics,
		"dependencies": dependencies,
		"worktrees": worktrees,
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
