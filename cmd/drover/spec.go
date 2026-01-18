// Package main provides CLI commands for Drover
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cloud-shuttle/drover/internal/llmproxy/client"
	"github.com/cloud-shuttle/drover/internal/spec"
	"github.com/spf13/cobra"
)

func specCmd() *cobra.Command {
	var (
		dryRun      bool
		yes         bool
		model       string
	)

	command := &cobra.Command{
		Use:   "spec <path>",
		Short: "Generate epics and tasks from design specifications",
		Long: `Generate epics, stories, and tasks from design specification files.

Supports both single files and design folders:
  - Single spec.md file: Analyzes one specification file
  - Design folder: Merges all .md and .jsonl files in the folder

The AI will automatically:
  - Identify epics from high-level features
  - Create stories (tasks) for each epic
  - Break down stories into subtasks
  - Generate acceptance criteria
  - Configure test modes and scopes

Examples:
  drover spec spec.md
  drover spec design/
  drover spec spec.md --dry-run
  drover spec design/ --yes`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Require project
			projectDir, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			inputPath := args[0]

			// Convert to absolute path
			if !filepath.IsAbs(inputPath) {
				inputPath = filepath.Join(projectDir, inputPath)
			}

			// Parse input files
			parser := spec.NewParser()
			content, files, err := parser.ParseInput(inputPath)
			if err != nil {
				return fmt.Errorf("parsing input: %w", err)
			}

			fmt.Printf("📄 Analyzing %d file(s)\n", len(files))
			for _, f := range files {
				fmt.Printf("   - %s\n", f)
			}
			fmt.Println()

			// Setup LLM client
			baseURL := os.Getenv("DROVER_LLM_PROXY_URL")
			if baseURL == "" {
				baseURL = "http://localhost:8080"
			}

			llmClient := client.NewClient(client.Config{
				BaseURL: baseURL,
				APIKey:  os.Getenv("ANTHROPIC_API_KEY"),
				Timeout: 5 * time.Minute,
			})

			// Use specified model or default
			if model == "" {
				model = "claude-sonnet-4-20250514"
			}

			analyzer := spec.NewAnalyzer(llmClient, model)

			fmt.Println("🤖 Analyzing specification with AI...")
			fmt.Printf("   Model: %s\n", model)

			// Analyze the spec
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			analysis, err := analyzer.AnalyzeSpec(ctx, content)
			if err != nil {
				return fmt.Errorf("AI analysis failed: %w", err)
			}

			// Show preview
			fmt.Println("\n📋 Generated Plan:")
			fmt.Println("════════════════════════════════════════")
			printAnalysis(analysis)

			if dryRun {
				fmt.Println("\n🔍 Dry-run mode - no changes made")
				return nil
			}

			// Confirm unless --yes flag
			if !yes {
				fmt.Print("\n✅ Create these epics and tasks? [y/N] ")
				var response string
				fmt.Scanln(&response)
				if response != "y" && response != "Y" {
					fmt.Println("❌ Cancelled")
					return nil
				}
			}

			// Write to database
			fmt.Println("\n💾 Creating epics and tasks...")
			writer := spec.NewWriter(store)
			result, err := writer.WriteAnalysis(analysis)
			if err != nil {
				return fmt.Errorf("writing to database: %w", err)
			}

			// Show results
			fmt.Println("\n✅ Successfully created:")
			fmt.Printf("   %d epics\n", len(result.Epics))
			fmt.Printf("   %d tasks\n", len(result.Tasks))
			fmt.Printf("   %d subtasks\n", len(result.SubTasks))
			fmt.Println("\nEpic IDs:")
			for _, epic := range result.Epics {
				fmt.Printf("   - %s: %s\n", epic.ID, epic.Title)
			}

			return nil
		},
	}

	command.Flags().BoolVar(&dryRun, "dry-run", false, "Preview changes without creating")
	command.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation prompt")
	command.Flags().StringVar(&model, "model", "", "AI model to use (default: claude-sonnet-4-20250514)")

	return command
}

// printAnalysis displays the analysis in a readable format
func printAnalysis(analysis *spec.SpecAnalysis) {
	for i, epic := range analysis.Epics {
		fmt.Printf("\n📌 Epic %d: %s\n", i+1, epic.Title)
		fmt.Printf("   %s\n", epic.Description)

		for j, task := range epic.Tasks {
			fmt.Printf("\n   [%d] %s\n", j+1, task.Title)
			fmt.Printf("       Type: %s | Priority: %d\n", task.Type, task.Priority)
			fmt.Printf("       Tests: %s/%s\n", task.TestMode, task.TestScope)

			if len(task.AcceptanceCriteria) > 0 {
				fmt.Printf("       Acceptance Criteria:\n")
				for _, ac := range task.AcceptanceCriteria {
					fmt.Printf("         ✓ %s\n", ac)
				}
			}

			if len(task.SubTasks) > 0 {
				fmt.Printf("       Subtasks (%d):\n", len(task.SubTasks))
				for k, st := range task.SubTasks {
					fmt.Printf("         %d. %s\n", k+1, st.Title)
				}
			}
		}
	}
}
