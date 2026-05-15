package workflow

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloud-shuttle/drover/internal/config"
	"github.com/cloud-shuttle/drover/pkg/types"
)

const taskPayloadDirRelative = ".drover/task_payload"

// PrepareTaskContextForAgent applies Epic 3 "context window management" to the task.
//
// Today we support guarding oversized task descriptions by writing the full description
// into a file inside the worktree and replacing task.Description with a compact reference
// block + fetch instructions.
//
// This avoids exceeding LLM context limits while keeping the full details available.
func PrepareTaskContextForAgent(worktreePath string, cfg *config.Config, task *types.Task) error {
	if cfg == nil || task == nil {
		return nil
	}

	maxDesc := int64(cfg.MaxDescriptionSize)
	if maxDesc <= 0 {
		// Disabled
		return nil
	}

	descBytes := int64(len([]byte(task.Description)))
	if descBytes <= maxDesc {
		return nil
	}

	relPath := filepath.Join(taskPayloadDirRelative, fmt.Sprintf("%s.md", task.ID))
	absPath := filepath.Join(worktreePath, relPath)

	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return fmt.Errorf("creating payload dir: %w", err)
	}

	payload := fmt.Sprintf(`# Task Payload: %s

## Title
%s

## Epic
%s

## Full Description
%s
`, task.ID, task.Title, task.EpicID, task.Description)

	if err := os.WriteFile(absPath, []byte(payload), 0o644); err != nil {
		return fmt.Errorf("writing payload file: %w", err)
	}

	task.Description = fmt.Sprintf(
		"## Large Content Notice\n\n"+
			"The task description is **%d bytes** which exceeds the configured threshold (**%d bytes**).\n"+
			"The full description has been written to:\n\n"+
			"- Path: %s\n"+
			"- Fetch command: `cat %q`\n\n"+
			"Please fetch and read that file before implementing the task.\n",
		descBytes, maxDesc, relPath, relPath,
	)

	return nil
}

