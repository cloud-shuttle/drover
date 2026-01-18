// Package spec provides types and functions for generating epics and tasks from design specifications
package spec

import (
	"fmt"

	"github.com/cloud-shuttle/drover/internal/db"
	"github.com/cloud-shuttle/drover/pkg/types"
)

// Writer creates epics and tasks in the database
type Writer struct {
	store *db.Store
}

// NewWriter creates a new spec writer
func NewWriter(store *db.Store) *Writer {
	return &Writer{store: store}
}

// WriteResult tracks what was created
type WriteResult struct {
	Epics    []*types.Epic
	Tasks    []*types.Task
	SubTasks []*types.Task
}

// WriteAnalysis creates epics and tasks from the analysis
func (w *Writer) WriteAnalysis(analysis *SpecAnalysis) (*WriteResult, error) {
	result := &WriteResult{
		Epics:    make([]*types.Epic, 0),
		Tasks:    make([]*types.Task, 0),
		SubTasks: make([]*types.Task, 0),
	}

	// Track created task IDs for dependency resolution
	taskIDMap := make(map[string]string) // Maps epic index.task index to actual task ID

	for epicIdx, epicSpec := range analysis.Epics {
		// Create epic
		epic, err := w.store.CreateEpic(epicSpec.Title, epicSpec.Description)
		if err != nil {
			return nil, fmt.Errorf("creating epic %d (%s): %w", epicIdx, epicSpec.Title, err)
		}
		result.Epics = append(result.Epics, epic)

		// Create tasks for this epic
		for taskIdx, taskSpec := range epicSpec.Tasks {
			taskKey := fmt.Sprintf("%d.%d", epicIdx, taskIdx)

			// Resolve blocked_by references
			blockedBy, err := w.resolveDependencies(taskSpec.BlockedBy, taskIDMap)
			if err != nil {
				return nil, fmt.Errorf("resolving dependencies for task %s: %w", taskKey, err)
			}

			// Create task with test configuration
			task, err := w.store.CreateTaskWithTestConfig(
				taskSpec.Title,
				w.buildTaskDescription(&taskSpec),
				epic.ID,
				taskSpec.Priority,
				blockedBy,
				"", // operator
				taskSpec.TestMode,
				taskSpec.TestScope,
				"", // test command (use default)
			)
			if err != nil {
				return nil, fmt.Errorf("creating task %s: %w", taskKey, err)
			}
			result.Tasks = append(result.Tasks, task)
			taskIDMap[taskKey] = task.ID

			// Create subtasks
			for subTaskIdx, subTaskSpec := range taskSpec.SubTasks {
				subTask, err := w.store.CreateSubTask(
					subTaskSpec.Title,
					subTaskSpec.Description,
					task.ID,
					subTaskSpec.Priority,
					nil, // subtasks don't support blocking in current implementation
				)
				if err != nil {
					return nil, fmt.Errorf("creating subtask %s.%d: %w", taskKey, subTaskIdx, err)
				}
				result.SubTasks = append(result.SubTasks, subTask)
			}
		}
	}

	return result, nil
}

// buildTaskDescription creates a task description with acceptance criteria
func (w *Writer) buildTaskDescription(spec *TaskSpec) string {
	desc := spec.Description
	if len(spec.AcceptanceCriteria) > 0 {
		desc += "\n\nAcceptance Criteria:\n"
		for _, ac := range spec.AcceptanceCriteria {
			desc += fmt.Sprintf("- %s\n", ac)
		}
	}
	return desc
}

// resolveDependencies converts task reference strings to actual task IDs
func (w *Writer) resolveDependencies(blockedBy []string, taskIDMap map[string]string) ([]string, error) {
	if len(blockedBy) == 0 {
		return nil, nil
	}

	resolved := make([]string, 0, len(blockedBy))
	for _, ref := range blockedBy {
		// Handle references like "0.1" -> epic 0, task 1
		taskID, ok := taskIDMap[ref]
		if !ok {
			return nil, fmt.Errorf("unknown task reference: %s (may refer to a task that hasn't been created yet)", ref)
		}
		resolved = append(resolved, taskID)
	}
	return resolved, nil
}
