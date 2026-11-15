package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/cloud-shuttle/drover/internal/db"
	"github.com/cloud-shuttle/drover/pkg/types"
	"github.com/spf13/cobra"
)

// epicSummary is the row printed by `drover epic list`.
type epicSummary struct {
	ID          string         `json:"id"`
	Title       string         `json:"title"`
	Description string         `json:"description,omitempty"`
	Status      string         `json:"status"`
	CreatedAt   int64          `json:"created_at"`
	TaskCount   int            `json:"task_count"`
	ByStatus    map[string]int `json:"tasks_by_status"`
}

var validTaskStatuses = []types.TaskStatus{
	types.TaskStatusReady,
	types.TaskStatusClaimed,
	types.TaskStatusInProgress,
	types.TaskStatusPaused,
	types.TaskStatusBlocked,
	types.TaskStatusCompleted,
	types.TaskStatusFailed,
	types.TaskStatusCancelled,
}

func parseTaskStatus(s string) (types.TaskStatus, error) {
	s = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(s, "-", "_")))
	for _, v := range validTaskStatuses {
		if string(v) == s {
			return v, nil
		}
	}
	names := make([]string, len(validTaskStatuses))
	for i, v := range validTaskStatuses {
		names[i] = string(v)
	}
	return "", fmt.Errorf("unknown status %q (valid: %s)", s, strings.Join(names, ", "))
}

// collectEpicSummaries joins epics with per-epic task counts.
func collectEpicSummaries(store *db.Store) ([]epicSummary, error) {
	epics, err := store.ListEpics()
	if err != nil {
		return nil, err
	}
	tasks, err := store.ListTasks()
	if err != nil {
		return nil, err
	}
	counts := map[string]map[string]int{}
	for _, t := range tasks {
		if t.EpicID == "" {
			continue
		}
		if counts[t.EpicID] == nil {
			counts[t.EpicID] = map[string]int{}
		}
		counts[t.EpicID][string(t.Status)]++
	}
	out := make([]epicSummary, 0, len(epics))
	for _, e := range epics {
		by := counts[e.ID]
		if by == nil {
			by = map[string]int{}
		}
		total := 0
		for _, n := range by {
			total += n
		}
		out = append(out, epicSummary{
			ID: e.ID, Title: e.Title, Description: e.Description,
			Status: string(e.Status), CreatedAt: e.CreatedAt,
			TaskCount: total, ByStatus: by,
		})
	}
	return out, nil
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func epicListCmd() *cobra.Command {
	var asJSON bool
	command := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List epics with task counts",
		Long: `List all epics with their IDs, titles, status and task counts.

Examples:
  drover epic list
  drover epic list --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			summaries, err := collectEpicSummaries(store)
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			if asJSON {
				return writeJSON(w, summaries)
			}
			if len(summaries) == 0 {
				fmt.Fprintln(w, "No epics found. Create one with: drover epic add <title>")
				return nil
			}
			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "ID\tSTATUS\tTASKS\tDONE\tTITLE")
			for _, s := range summaries {
				fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%s\n", s.ID, s.Status, s.TaskCount,
					s.ByStatus[string(types.TaskStatusCompleted)], s.Title)
			}
			return tw.Flush()
		},
	}
	command.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	return command
}

func listCmd() *cobra.Command {
	var (
		epicID string
		status string
		asJSON bool
	)
	command := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List tasks, optionally filtered by epic or status",
		Long: `List tasks with their IDs, status, epic and title.

Examples:
  drover list
  drover list --epic epic-123
  drover list --status ready
  drover list --epic epic-123 --status failed --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var want types.TaskStatus
			if status != "" {
				s, err := parseTaskStatus(status)
				if err != nil {
					return err
				}
				want = s
			}

			_, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			if epicID != "" {
				epics, err := store.ListEpics()
				if err != nil {
					return err
				}
				found := false
				for _, e := range epics {
					if e.ID == epicID {
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("epic %q not found (see: drover epic list)", epicID)
				}
			}

			tasks, err := store.ListTasksByEpic(epicID)
			if err != nil {
				return err
			}
			filtered := make([]*types.Task, 0, len(tasks))
			for _, t := range tasks {
				if want != "" && t.Status != want {
					continue
				}
				filtered = append(filtered, t)
			}
			sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].CreatedAt < filtered[j].CreatedAt })

			w := cmd.OutOrStdout()
			if asJSON {
				return writeJSON(w, filtered)
			}
			if len(filtered) == 0 {
				fmt.Fprintln(w, "No tasks found.")
				return nil
			}
			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "ID\tSTATUS\tPRIORITY\tEPIC\tTITLE")
			for _, t := range filtered {
				epic := t.EpicID
				if epic == "" {
					epic = "-"
				}
				fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\n", t.ID, t.Status, t.Priority, epic, t.Title)
			}
			return tw.Flush()
		},
	}
	command.Flags().StringVarP(&epicID, "epic", "e", "", "Only tasks in this epic")
	command.Flags().StringVarP(&status, "status", "s", "", "Only tasks with this status (ready, claimed, in_progress, paused, blocked, completed, failed, cancelled)")
	command.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	return command
}
