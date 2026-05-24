package main

import (
	"fmt"
	"time"

	"github.com/cloud-shuttle/drover/pkg/types"
)

// formatTaskStatus converts task status to a user-friendly string with an icon
func formatTaskStatus(status types.TaskStatus) string {
	switch status {
	case types.TaskStatusReady:
		return "🟢 ready"
	case types.TaskStatusClaimed:
		return "🟡 claimed"
	case types.TaskStatusInProgress:
		return "🔵 in_progress"
	case types.TaskStatusPaused:
		return "⏸️  paused"
	case types.TaskStatusBlocked:
		return "🚫 blocked"
	case types.TaskStatusCompleted:
		return "✅ completed"
	case types.TaskStatusFailed:
		return "❌ failed"
	default:
		return string(status)
	}
}

// formatTimestamp converts int64 Unix timestamp to a human-readable string
func formatTimestamp(timestamp int64) string {
	t := time.Unix(timestamp, 0)
	return t.Format("2006-01-02 15:04:05")
}

// formatBytes converts bytes to a human-readable string
func formatBytes(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	unitIndex := 0
	value := float64(bytes)

	for value >= 1024 && unitIndex < len(units)-1 {
		value /= 1024
		unitIndex++
	}

	return fmt.Sprintf("%.1f %s", value, units[unitIndex])
}
