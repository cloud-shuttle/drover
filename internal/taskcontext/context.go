// Package taskcontext provides task context carrying for AI agent memory
package taskcontext

import (
	"fmt"
	"strings"
	"time"

	"github.com/cloud-shuttle/drover/pkg/types"
)

// TaskFormatter formats completed tasks for context injection
type TaskFormatter struct {
	maxSummaryLength int
}

// NewTaskFormatter creates a new task formatter
func NewTaskFormatter() *TaskFormatter {
	return &TaskFormatter{
		maxSummaryLength: 200, // Max characters for task summary
	}
}

// FormatTasksForContext formats recent completed tasks as markdown context
// Returns empty string if no tasks provided
func (f *TaskFormatter) FormatTasksForContext(tasks []*types.Task, currentTaskID string) string {
	if len(tasks) == 0 {
		return ""
	}

	var builder strings.Builder

	builder.WriteString("## Recent Task Context\n\n")
	builder.WriteString("*The following recently completed tasks may provide useful context and patterns:*\n\n")

	for _, task := range tasks {
		// Skip the current task if it's in the list
		if task.ID == currentTaskID {
			continue
		}

		f.formatTask(&builder, task)
	}

	builder.WriteString("---\n\n")

	return builder.String()
}

// formatTask formats a single task for context display
func (f *TaskFormatter) formatTask(builder *strings.Builder, task *types.Task) {
	// Task header with verdict icon
	builder.WriteString(fmt.Sprintf("### %s: %s (%s)\n", task.ID, task.Title, f.formatVerdict(task.Verdict)))

	// Time since completion
	completedAt := time.Unix(task.UpdatedAt, 0)
	timeAgo := time.Since(completedAt)
	builder.WriteString(fmt.Sprintf("*Completed %s*\n", f.formatDuration(timeAgo)))

	// Summary/verdict reason
	summary := task.VerdictReason
	if summary == "" {
		summary = f.truncateSummary(task.Description)
	}
	if summary != "" {
		builder.WriteString(fmt.Sprintf("> %s\n", summary))
	}

	builder.WriteString("\n")
}

// formatVerdict formats the verdict with an icon
func (f *TaskFormatter) formatVerdict(verdict types.TaskVerdict) string {
	switch verdict {
	case types.TaskVerdictPass:
		return "✅ Pass"
	case types.TaskVerdictFail:
		return "❌ Fail"
	case types.TaskVerdictBlocked:
		return "🚧 Blocked"
	case types.TaskVerdictUnknown:
		return "❓ Unknown"
	default:
		return string(verdict)
	}
}

// formatDuration formats a duration in human-readable form
func (f *TaskFormatter) formatDuration(d time.Duration) string {
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		minutes := int(d.Minutes())
		if minutes == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", minutes)
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	}
	days := int(d.Hours() / 24)
	if days == 1 {
		return "1 day ago"
	}
	return fmt.Sprintf("%d days ago", days)
}

// truncateSummary truncates a summary to max length
func (f *TaskFormatter) truncateSummary(summary string) string {
	summary = strings.TrimSpace(summary)
	if len(summary) <= f.maxSummaryLength {
		return summary
	}

	// Try to truncate at a word boundary
	truncated := summary[:f.maxSummaryLength]
	lastSpace := strings.LastIndex(truncated, " ")
	if lastSpace > 0 {
		truncated = truncated[:lastSpace]
	}

	return truncated + "..."
}

// BuildContext constructs the full context with recent tasks and current task
// This is the main entry point for context injection
func BuildContext(recentTasks []*types.Task, currentTask *types.Task, taskContextCount int) string {
	formatter := NewTaskFormatter()

	// Limit to configured count
	if taskContextCount <= 0 {
		return ""
	}
	if len(recentTasks) > taskContextCount {
		recentTasks = recentTasks[:taskContextCount]
	}

	// Format recent tasks context
	context := formatter.FormatTasksForContext(recentTasks, currentTask.ID)

	return context
}

// BuildContextWithCurrentTask formats the full context including current task header
func BuildContextWithCurrentTask(recentTasks []*types.Task, currentTask *types.Task, taskContextCount int) string {
	var builder strings.Builder

	// Add recent tasks context if available
	if taskContextCount > 0 && len(recentTasks) > 0 {
		context := BuildContext(recentTasks, currentTask, taskContextCount)
		if context != "" {
			builder.WriteString(context)
		}
	}

	// Add current task header
	builder.WriteString("## Current Task\n\n")
	builder.WriteString(fmt.Sprintf("**Task ID:** %s\n", currentTask.ID))
	builder.WriteString(fmt.Sprintf("**Title:** %s\n", currentTask.Title))
	if currentTask.Type != "" {
		builder.WriteString(fmt.Sprintf("**Type:** %s\n", currentTask.Type))
	}
	builder.WriteString(fmt.Sprintf("\n%s\n", currentTask.Description))

	return builder.String()
}
