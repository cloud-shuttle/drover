// Package main provides CLI commands for Drover
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cloud-shuttle/drover/internal/db"
	"github.com/spf13/cobra"
)

// JSONLRecord represents a single line in the JSONL file
type JSONLRecord struct {
	ID                 string   `json:"id"`
	Type               string   `json:"type"`
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	Priority           int      `json:"priority"`
	PriorityStr        string   `json:"priority_str,omitempty"` // Alternative string priority (e.g. "high", "critical")
	EpicID             string   `json:"epic_id,omitempty"`
	StoryID            string   `json:"story_id,omitempty"`
	StoryPoints        int      `json:"story_points,omitempty"`
	EstimatedHours     int      `json:"estimated_hours,omitempty"`
	Labels             []string `json:"labels,omitempty"`
	AcceptanceCriteria []string `json:"acceptance_criteria,omitempty"`
}

func importJSONLCmd() *cobra.Command {
	var skipValidation bool

	command := &cobra.Command{
		Use:   "import-jsonl <file.jsonl>",
		Short: "Import epics, stories, and tasks from a JSONL file",
		Long: `Import epics, stories, and tasks from a JSON Lines (JSONL) file.

JSONL Format:
- One JSON object per line
- Types: "epic", "story", "task"
- Stories reference epics via epic_id
- Tasks reference stories via story_id (become subtasks)

Example file:
  {"id": "EPIC-001", "type": "epic", "title": "Project Setup", "description": "Set up infrastructure"}
  {"id": "STORY-001", "type": "story", "epic_id": "EPIC-001", "title": "Init Repo", "description": "Create repo", "priority": 10}
  {"id": "TASK-001", "type": "task", "story_id": "STORY-001", "title": "Create README", "description": "Add README", "priority": 5}

Priority values:
- Integer: 1-10 (higher = more urgent)
- String: "critical" (10), "high" (7), "normal" (5), "low" (2)`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImportJSONL(args[0], skipValidation)
		},
	}

	command.Flags().BoolVar(&skipValidation, "skip-validation", false, "Skip task quality validation")
	return command
}

func runImportJSONL(filename string, skipValidation bool) error {
	projectDir, err := findProjectDir()
	if err != nil {
		return err
	}

	store, err := db.Open(filepath.Join(projectDir, ".drover", "drover.db"))
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer store.Close()

	// Initialize schema if this is a fresh database
	if err := store.InitSchema(); err != nil {
		return fmt.Errorf("initializing schema: %w", err)
	}

	// Run migrations for existing databases
	if err := store.MigrateSchema(); err != nil {
		return fmt.Errorf("migrating schema: %w", err)
	}

	// Open the JSONL file
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("opening file: %w", err)
	}
	defer file.Close()

	fmt.Printf("📦 Importing from %s\n", filename)
	fmt.Println()

	// Maps to track external IDs to Drover IDs
	epicIDMap := make(map[string]string)  // EPIC-001 -> epic-123
	storyIDMap := make(map[string]string) // STORY-001 -> task-456

	// Counters
	var epicCount, storyCount, taskCount int

	// Parse file line by line
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse JSON
		var record JSONLRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			fmt.Printf("⚠️  Line %d: Skipping invalid JSON: %v\n", lineNum, err)
			continue
		}

		// Validate required fields
		if record.Type == "" {
			fmt.Printf("⚠️  Line %d: Skipping (missing 'type' field)\n", lineNum)
			continue
		}
		if record.ID == "" {
			fmt.Printf("⚠️  Line %d: Skipping (missing 'id' field)\n", lineNum)
			continue
		}

		// Normalize priority
		priority := normalizePriority(record.Priority, record.PriorityStr)

		switch record.Type {
		case "epic":
			epic, err := store.CreateEpic(record.Title, record.Description)
			if err != nil {
				fmt.Printf("❌ [%s] Failed to create epic: %v\n", record.ID, err)
				continue
			}
			epicIDMap[record.ID] = epic.ID
			epicCount++
			fmt.Printf("✅ [EPIC] %s -> %s\n", record.ID, epic.ID)
			fmt.Printf("         %s\n", record.Title)

		case "story":
			// Resolve epic ID
			epicID, ok := epicIDMap[record.EpicID]
			if !ok {
				fmt.Printf("⚠️  [%s] Skipping (epic '%s' not found)\n", record.ID, record.EpicID)
				continue
			}

			// Build description with acceptance criteria
			description := record.Description
			if len(record.AcceptanceCriteria) > 0 {
				description += "\n\nAcceptance Criteria:\n"
				for _, ac := range record.AcceptanceCriteria {
					description += fmt.Sprintf("- %s\n", ac)
				}
			}

			// Create task with epic assigned (skip validation for imported tasks)
			task, err := store.CreateTaskWithTestConfig(
				record.Title,
				description,
				epicID,
				priority,
				nil, // no blocked-by
				"",  // no operator
				"disabled", // disable tests for imported tasks
				"skip",
				"",
			)
			if err != nil {
				fmt.Printf("❌ [%s] Failed to create story: %v\n", record.ID, err)
				continue
			}
			storyIDMap[record.ID] = task.ID
			storyCount++
			fmt.Printf("✅ [STORY] %s -> %s\n", record.ID, task.ID)
			fmt.Printf("         %s\n", record.Title)

		case "task":
			// Resolve story ID (story is the parent task)
			parentID, ok := storyIDMap[record.StoryID]
			if !ok {
				fmt.Printf("⚠️  [%s] Skipping (story '%s' not found)\n", record.ID, record.StoryID)
				continue
			}

			// Build description with acceptance criteria
			description := record.Description
			if len(record.AcceptanceCriteria) > 0 {
				description += "\n\nAcceptance Criteria:\n"
				for _, ac := range record.AcceptanceCriteria {
					description += fmt.Sprintf("- %s\n", ac)
				}
			}

			// Create subtask under the story
			task, err := store.CreateSubTask(
				record.Title,
				description,
				parentID,
				priority,
				nil, // no blocked-by
			)
			if err != nil {
				fmt.Printf("❌ [%s] Failed to create task: %v\n", record.ID, err)
				continue
			}
			taskCount++
			fmt.Printf("✅ [TASK] %s -> %s\n", record.ID, task.ID)
			fmt.Printf("         %s\n", record.Title)

		default:
			fmt.Printf("⚠️  Line %d: Skipping unknown type '%s'\n", lineNum, record.Type)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	fmt.Println()
	fmt.Println("=== Import Complete ===")
	fmt.Printf("Epics:  %d\n", epicCount)
	fmt.Printf("Stories: %d\n", storyCount)
	fmt.Printf("Tasks:  %d\n", taskCount)
	fmt.Println()
	fmt.Printf("Run 'cd %s && drover status' to see all tasks\n", projectDir)

	return nil
}

// normalizePriority converts string priority to integer, or returns the integer as-is
func normalizePriority(priorityInt int, priorityStr string) int {
	// If integer is set, use it
	if priorityInt > 0 {
		return priorityInt
	}

	// Map string priorities to integers
	switch strings.ToLower(priorityStr) {
	case "critical", "urgent":
		return 10
	case "high":
		return 7
	case "normal", "medium":
		return 5
	case "low":
		return 2
	default:
		// Try to parse as integer
		if i, err := strconv.Atoi(priorityStr); err == nil && i > 0 {
			return i
		}
		return 5 // default to normal
	}
}
