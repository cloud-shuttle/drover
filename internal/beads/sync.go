// Package beads provides bidirectional sync between Drover and Beads
package beads

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloud-shuttle/drover/pkg/types"
)

// ============================================================================
// BEADS DATA STRUCTURES
// ============================================================================

// BeadRecord represents a single line in beads.jsonl
type BeadRecord struct {
	Type      string          `json:"type"`       // "bead", "epic", "link"
	ID        string          `json:"id"`
	Timestamp time.Time       `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
}

// BeadTask is the task data within a bead record
type BeadTask struct {
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Status      string   `json:"status"` // "open", "active", "closed"
	Priority    int      `json:"priority,omitempty"`
	Labels      []string `json:"labels,omitempty"`
	EpicID      string   `json:"epic_id,omitempty"`
	ClosedAt    string   `json:"closed_at,omitempty"`
	Reason      string   `json:"reason,omitempty"` // "completed", "wontfix", etc.
}

// BeadEpic is the epic data
type BeadEpic struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status"`
}

// BeadLink represents a dependency
type BeadLink struct {
	From     string `json:"from"`
	To       string `json:"to"`
	LinkType string `json:"link_type"` // "blocked_by", "relates_to"
}

// ============================================================================
// SYNC CONFIGURATION
// ============================================================================

type SyncConfig struct {
	BeadsDir     string
	SyncInterval time.Duration
	AutoSync     bool
}

func DefaultSyncConfig(projectDir string) SyncConfig {
	return SyncConfig{
		BeadsDir:     filepath.Join(projectDir, ".beads"),
		SyncInterval: 5 * time.Second,
		AutoSync:     false,
	}
}

// ============================================================================
// IMPORT FROM BEADS
// ============================================================================

// ImportFromBeads reads .beads/beads.jsonl and returns Drover types
func ImportFromBeads(config SyncConfig) ([]types.Epic, []types.Task, []types.TaskDependency, error) {
	jsonlPath := filepath.Join(config.BeadsDir, "beads.jsonl")

	file, err := os.Open(jsonlPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil, fmt.Errorf("beads not initialized - run 'bd init' first")
		}
		return nil, nil, nil, err
	}
	defer file.Close()

	importedTasks := make(map[string]bool)
	importedEpics := make(map[string]bool)
	var links []BeadLink

	var epics []types.Epic
	var tasks []types.Task

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var record BeadRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			continue // Skip malformed lines
		}

		switch record.Type {
		case "epic":
			var epicData BeadEpic
			json.Unmarshal(record.Data, &epicData)

			epic := types.Epic{
				ID:          record.ID,
				Title:       epicData.Title,
				Description: epicData.Description,
				Status:      types.EpicStatus(epicData.Status),
				CreatedAt:   record.Timestamp.Unix(),
			}
			epics = append(epics, epic)
			importedEpics[record.ID] = true

		case "bead":
			var taskData BeadTask
			json.Unmarshal(record.Data, &taskData)

			task := types.Task{
				ID:          record.ID,
				Title:       taskData.Title,
				Description: taskData.Description,
				EpicID:      taskData.EpicID,
				Priority:    taskData.Priority,
				Status:      beadsStatusToDrover(taskData.Status),
				CreatedAt:   record.Timestamp.Unix(),
				UpdatedAt:   time.Now().Unix(),
			}
			tasks = append(tasks, task)
			importedTasks[record.ID] = true

		case "link":
			var linkData BeadLink
			json.Unmarshal(record.Data, &linkData)
			if linkData.LinkType == "blocked_by" {
				links = append(links, linkData)
			}
		}
	}

	// Process dependencies
	var deps []types.TaskDependency
	for _, link := range links {
		if importedTasks[link.From] && importedTasks[link.To] {
			deps = append(deps, types.TaskDependency{
				TaskID:    link.From,
				BlockedBy: link.To,
			})
		}
	}

	return epics, tasks, deps, nil
}

// ============================================================================
// EXPORT TO BEADS
// ============================================================================

// ExportToBeads writes Drover state to .beads/beads.jsonl
func ExportToBeads(epics []types.Epic, tasks []types.Task, deps []types.TaskDependency, config SyncConfig) error {
	if err := os.MkdirAll(config.BeadsDir, 0755); err != nil {
		return err
	}

	jsonlPath := filepath.Join(config.BeadsDir, "beads.jsonl")
	file, err := os.Create(jsonlPath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)

	// Export epics
	for _, epic := range epics {
		record := BeadRecord{
			Type:      "epic",
			ID:        epic.ID,
			Timestamp: time.Unix(epic.CreatedAt, 0),
		}
		epicData := BeadEpic{
			Title:       epic.Title,
			Description: epic.Description,
			Status:      string(epic.Status),
		}
		record.Data, _ = json.Marshal(epicData)
		encoder.Encode(record)
	}

	// Export tasks
	for _, task := range tasks {
		record := BeadRecord{
			Type:      "bead",
			ID:        task.ID,
			Timestamp: time.Unix(task.CreatedAt, 0),
		}
		taskData := BeadTask{
			Title:       task.Title,
			Description: task.Description,
			Status:      droverStatusToBeads(task.Status),
			Priority:    task.Priority,
			EpicID:      task.EpicID,
		}
		record.Data, _ = json.Marshal(taskData)
		encoder.Encode(record)
	}

	// Export dependencies as links
	for _, dep := range deps {
		record := BeadRecord{
			Type:      "link",
			ID:        fmt.Sprintf("link-%s-%s", dep.TaskID, dep.BlockedBy),
			Timestamp: time.Now(),
		}
		linkData := BeadLink{
			From:     dep.TaskID,
			To:       dep.BlockedBy,
			LinkType: "blocked_by",
		}
		record.Data, _ = json.Marshal(linkData)
		encoder.Encode(record)
	}

	return nil
}

// ============================================================================
// BD CLI INTEGRATION
// ============================================================================

// RunBdCommand executes a bd CLI command
func RunBdCommand(args ...string) (string, error) {
	cmd := exec.Command("bd", args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// BdClose closes a task via bd CLI
func BdClose(taskID, reason string) error {
	args := []string{"close", taskID}
	if reason != "" {
		args = append(args, "--reason", reason)
	}
	_, err := RunBdCommand(args...)
	return err
}

// BdAdd creates a task via bd CLI
func BdAdd(title string, epicID string, blockedBy []string) (string, error) {
	args := []string{"add", title}
	if epicID != "" {
		args = append(args, "--epic", epicID)
	}
	for _, dep := range blockedBy {
		args = append(args, "--blocked-by", dep)
	}
	output, err := RunBdCommand(args...)
	if err != nil {
		return "", err
	}
	// Parse task ID from output
	parts := strings.Fields(output)
	if len(parts) >= 3 {
		return parts[len(parts)-1], nil
	}
	return "", fmt.Errorf("could not parse task ID from: %s", output)
}

// ============================================================================
// STATUS CONVERSION
// ============================================================================

func beadsStatusToDrover(beadsStatus string) types.TaskStatus {
	switch beadsStatus {
	case "open":
		return types.TaskStatusReady
	case "active":
		return types.TaskStatusInProgress
	case "closed":
		return types.TaskStatusCompleted
	default:
		return types.TaskStatusReady
	}
}

func droverStatusToBeads(droverStatus types.TaskStatus) string {
	switch droverStatus {
	case types.TaskStatusReady, types.TaskStatusClaimed, types.TaskStatusBlocked:
		return "open"
	case types.TaskStatusInProgress:
		return "active"
	case types.TaskStatusCompleted:
		return "closed"
	case types.TaskStatusFailed:
		return "closed"
	default:
		return "open"
	}
}
