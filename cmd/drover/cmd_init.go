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

			// Create default project configuration
			configPath := filepath.Join(dir, ".drover.toml")
			configContent := `# Drover Project Configuration
# Customize these settings for your project

# Agent configuration
agent = "claude"          # Options: claude, codex, amp, opencode
max_workers = 4          # Number of parallel workers
task_timeout = "60m"     # Maximum time per task
max_attempts = 3         # Retry attempts for failed tasks

# Context settings
task_context_count = 5   # Number of recent tasks to include for context

# Size thresholds (for Epic 3: Context Window Management)
max_description_size = "250MB"  # Max task description size
max_diff_size = "250MB"         # Max diff size to inline
max_file_size = "1MB"           # Max file size to inline

# Project-specific guidelines
# These will be included in every task prompt
guidelines = """
Add your project-specific guidelines here:

Example for a Go project:
- Follow Go idioms and conventions
- Use structured logging with slog
- Write table-driven tests
- Handle errors properly, don't ignore them

Example for a web project:
- Follow existing code style and patterns
- Write tests for new features
- Update documentation for API changes
- Use TypeScript strict mode
"""

# Default labels to apply to all tasks
# default_labels = ["drover", "go", "backend"]
`
			if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
				return fmt.Errorf("creating project config: %w", err)
			}

			fmt.Printf("🐂 Initialized Drover in %s\n", droverDir)
			fmt.Println("\nWorkflow Engine:")
			fmt.Println("  • DBOS with SQLite (default): Durable execution, automatic recovery")
			fmt.Println("  • DBOS with PostgreSQL: Set DBOS_SYSTEM_DATABASE_URL for production")
			fmt.Println("\nNext steps:")
			fmt.Println("  drover epic add \"My Epic\"")
			fmt.Println("  drover add \"My first task\" --epic <epic-id>")
			fmt.Println("  drover run")
			fmt.Println("\n📋 Files created:")
			fmt.Println("  • .drover/task_template.yaml - Task quality template")
			fmt.Println("  • .drover.toml - Project configuration")
			fmt.Println("\n💡 Customize .drover.toml with your project guidelines!")

			return nil
		},
	}
}
