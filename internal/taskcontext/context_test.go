// Package taskcontext provides tests for task context carrying
package taskcontext

import (
	"testing"
	"time"

	"github.com/cloud-shuttle/drover/pkg/types"
)

func TestBuildContext(t *testing.T) {
	now := time.Now().Unix()

	recentTasks := []*types.Task{
		{
			ID:          "task-1",
			Title:       "Implement user auth",
			Description: "Added JWT authentication with refresh tokens",
			Verdict:     types.TaskVerdictPass,
			VerdictReason: "Successfully implemented auth system",
			UpdatedAt:   now - 3600, // 1 hour ago
		},
		{
			ID:          "task-2",
			Title:       "Add rate limiting",
			Description: "Implemented token bucket rate limiter",
			Verdict:     types.TaskVerdictPass,
			VerdictReason: "Rate limiting working as expected",
			UpdatedAt:   now - 7200, // 2 hours ago
		},
	}

	currentTask := &types.Task{
		ID:          "task-3",
		Title:       "Add password reset",
		Description: "Implement forgot password flow",
	}

	formatter := NewTaskFormatter()

	t.Run("FormatTasksForContext", func(t *testing.T) {
		context := formatter.FormatTasksForContext(recentTasks, currentTask.ID)

		if context == "" {
			t.Fatal("Expected non-empty context")
		}

		// Check for expected elements
		if !contains(context, "## Recent Task Context") {
			t.Error("Expected 'Recent Task Context' header")
		}
		if !contains(context, "task-1") {
			t.Error("Expected task-1 in context")
		}
		if !contains(context, "Implement user auth") {
			t.Error("Expected task title in context")
		}
		if !contains(context, "✅ Pass") {
			t.Error("Expected pass verdict")
		}
		if !contains(context, "Successfully implemented auth system") {
			t.Error("Expected verdict reason in context")
		}
	})

	t.Run("BuildContext with limit", func(t *testing.T) {
		context := BuildContext(recentTasks, currentTask, 1)

		if context == "" {
			t.Fatal("Expected non-empty context")
		}

		// Should only include 1 task
		lines := countLines(context)
		if lines > 20 {
			t.Errorf("Expected shorter context with limit=1, got %d lines", lines)
		}
	})

	t.Run("BuildContext with zero limit", func(t *testing.T) {
		context := BuildContext(recentTasks, currentTask, 0)

		if context != "" {
			t.Error("Expected empty context with limit=0")
		}
	})

	t.Run("BuildContext with empty tasks", func(t *testing.T) {
		context := BuildContext([]*types.Task{}, currentTask, 5)

		if context != "" {
			t.Error("Expected empty context with no recent tasks")
		}
	})
}

func TestFormatVerdict(t *testing.T) {
	formatter := NewTaskFormatter()

	tests := []struct {
		name     string
		verdict  types.TaskVerdict
		expected string
	}{
		{"Pass", types.TaskVerdictPass, "✅ Pass"},
		{"Fail", types.TaskVerdictFail, "❌ Fail"},
		{"Blocked", types.TaskVerdictBlocked, "🚧 Blocked"},
		{"Unknown", types.TaskVerdictUnknown, "❓ Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatter.formatVerdict(tt.verdict)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	formatter := NewTaskFormatter()

	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{"Just now", 30 * time.Second, "just now"},
		{"1 minute", 1 * time.Minute, "1 minute ago"},
		{"5 minutes", 5 * time.Minute, "5 minutes ago"},
		{"1 hour", 1 * time.Hour, "1 hour ago"},
		{"3 hours", 3 * time.Hour, "3 hours ago"},
		{"1 day", 24 * time.Hour, "1 day ago"},
		{"2 days", 48 * time.Hour, "2 days ago"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatter.formatDuration(tt.duration)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestTruncateSummary(t *testing.T) {
	formatter := NewTaskFormatter()

	t.Run("Short summary", func(t *testing.T) {
		summary := "This is a short summary"
		result := formatter.truncateSummary(summary)
		if result != summary {
			t.Errorf("Expected unchanged summary, got %q", result)
		}
	})

	t.Run("Long summary", func(t *testing.T) {
		// Create a summary that's definitely longer than maxSummaryLength (200)
		// The summary is about 260 characters
		summary := "This is a very long summary that exceeds the maximum length and must be truncated because it is way too long for the formatter to handle properly and therefore should result in a truncated version with ellipsis at the end to indicate that more content was removed from the original summary text."
		result := formatter.truncateSummary(summary)
		if len(result) >= len(summary) {
			t.Errorf("Expected truncated summary, got %d chars (original was %d)", len(result), len(summary))
		}
		// The truncated version should end with ...
		if !contains(result, "...") {
			t.Errorf("Expected truncated summary to contain '...', got %q", result)
		}
		// Truncated summary should be shorter than original
		if len(result) >= len(summary) {
			t.Errorf("Truncated summary should be shorter than original")
		}
	})
}

func TestBuildContextWithCurrentTask(t *testing.T) {
	now := time.Now().Unix()

	recentTasks := []*types.Task{
		{
			ID:        "task-1",
			Title:     "Previous task",
			Verdict:   types.TaskVerdictPass,
			UpdatedAt: now - 3600,
		},
	}

	currentTask := &types.Task{
		ID:          "task-2",
		Title:       "Current task",
		Description: "Do the work",
		Type:        types.TaskTypeFeature,
	}

	result := BuildContextWithCurrentTask(recentTasks, currentTask, 5)

	// Check for both sections
	if !contains(result, "## Recent Task Context") {
		t.Error("Expected recent task context section")
	}
	if !contains(result, "## Current Task") {
		t.Error("Expected current task section")
	}
	if !contains(result, "Current task") {
		t.Error("Expected current task title")
	}
	if !contains(result, "Do the work") {
		t.Error("Expected current task description")
	}
	// The format should be **Type:** not "Type:" (markdown bold)
	if !contains(result, "**Type:**") {
		t.Error("Expected **Type:** (markdown bold)")
	}
}

func TestSkipCurrentTask(t *testing.T) {
	now := time.Now().Unix()

	recentTasks := []*types.Task{
		{
			ID:        "task-1",
			Title:     "Previous task",
			Verdict:   types.TaskVerdictPass,
			UpdatedAt: now - 3600,
		},
	}

	currentTask := &types.Task{
		ID:    "task-1", // Same ID as recent task
		Title: "Current task",
	}

	formatter := NewTaskFormatter()
	result := formatter.FormatTasksForContext(recentTasks, currentTask.ID)

	// Should skip the task since it has the same ID
	if contains(result, "task-1") {
		t.Error("Expected current task to be skipped from context")
	}
	if contains(result, "Previous task") {
		t.Error("Expected current task to be skipped from context")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func countLines(s string) int {
	count := 0
	for _, c := range s {
		if c == '\n' {
			count++
		}
	}
	return count
}
