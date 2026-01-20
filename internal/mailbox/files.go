// Package mailbox provides file-based task queue implementation
package mailbox

import (
	"encoding/json"
	"os"

	"github.com/cloud-shuttle/drover/pkg/types"
)

// writeTaskFile writes a task to a file
func writeTaskFile(path string, task *types.Task) error {
	data, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// readTaskFile reads a task from a file
func readTaskFile(path string) (*types.Task, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var task types.Task
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, err
	}

	return &task, nil
}

// writeResultFile writes a task result to a file
func writeResultFile(path string, result *TaskResult) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// writeErrorFile writes a task error to a file
func writeErrorFile(path string, taskErr *TaskError) error {
	data, err := json.MarshalIndent(taskErr, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
