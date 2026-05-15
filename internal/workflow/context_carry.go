package workflow

import (
	"fmt"
	"strings"
	"time"

	"github.com/cloud-shuttle/drover/internal/config"
	"github.com/cloud-shuttle/drover/internal/db"
	"github.com/cloud-shuttle/drover/pkg/types"
)

// InjectRecentTaskContext prepends a “Recent Task Context” block onto task.Description.
//
// This is Epic 6 (Task Context Carrying). It is intentionally lightweight:
// we include IDs + titles + completion timestamp proxy (updated_at) and any last_error note.
func InjectRecentTaskContext(store *db.Store, cfg *config.Config, task *types.Task) {
	if store == nil || cfg == nil || task == nil {
		return
	}
	if cfg.TaskContextCount <= 0 {
		return
	}

	// Prefer same-epic context when available; otherwise use project-wide recent tasks.
	epicFilter := ""
	if task.EpicID != "" {
		epicFilter = task.EpicID
	}

	recent, err := store.ListRecentCompletedTasks(cfg.TaskContextCount, epicFilter, task.ID)
	if err != nil || len(recent) == 0 {
		return
	}

	var b strings.Builder
	b.WriteString("## Recent Task Context\n\n")
	b.WriteString("The following tasks were recently completed. Use this context to stay consistent with recent decisions.\n\n")

	for _, r := range recent {
		b.WriteString(fmt.Sprintf("### %s: %s\n", r.ID, r.Title))
		if r.UpdatedAt != 0 {
			b.WriteString(fmt.Sprintf("*Completed (approx): %s*\n", time.Unix(r.UpdatedAt, 0).Format(time.RFC3339)))
		}
		if strings.TrimSpace(r.LastError) != "" {
			b.WriteString(fmt.Sprintf("> Note: %s\n", strings.TrimSpace(r.LastError)))
		}
		b.WriteString("\n")
	}

	b.WriteString("---\n\n")

	// Prepend
	task.Description = b.String() + task.Description
}

