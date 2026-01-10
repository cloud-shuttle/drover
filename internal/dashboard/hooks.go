package dashboard

import (
	"sync"
)

// Global dashboard instance for broadcasting (set by the dashboard command)
var (
	globalDashboard *Server
	globalMu        sync.RWMutex
)

// SetGlobal sets the global dashboard for event broadcasting
func SetGlobal(s *Server) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalDashboard = s
}

// GetGlobal returns the global dashboard instance
func GetGlobal() *Server {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalDashboard
}

// Event types for WebSocket broadcasting
const (
	EventTaskClaimed   = "task_claimed"
	EventTaskStarted   = "task_started"
	EventTaskCompleted = "task_completed"
	EventTaskFailed    = "task_failed"
	EventTaskBlocked   = "task_blocked"
	EventWorkerStatus  = "worker_status"
	EventStatsUpdate   = "stats_update"
)

// TaskEvent is broadcast when a task state changes
type TaskEvent struct {
	TaskID string `json:"task_id"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Worker string `json:"worker,omitempty"`
	Error  string `json:"error,omitempty"`
	EpicID string `json:"epic_id,omitempty"`
}

// BroadcastTaskClaimed broadcasts when a worker claims a task
func BroadcastTaskClaimed(taskID, title, worker string) {
	dash := GetGlobal()
	if dash == nil {
		return
	}
	dash.Broadcast(EventTaskClaimed, TaskEvent{
		TaskID: taskID,
		Title:  title,
		Status: "claimed",
		Worker: worker,
	})
}

// BroadcastTaskStarted broadcasts when execution begins
func BroadcastTaskStarted(taskID, title, worker string) {
	dash := GetGlobal()
	if dash == nil {
		return
	}
	dash.Broadcast(EventTaskStarted, TaskEvent{
		TaskID: taskID,
		Title:  title,
		Status: "in_progress",
		Worker: worker,
	})
}

// BroadcastTaskCompleted broadcasts when a task finishes successfully
func BroadcastTaskCompleted(taskID, title string) {
	dash := GetGlobal()
	if dash == nil {
		return
	}
	dash.Broadcast(EventTaskCompleted, TaskEvent{
		TaskID: taskID,
		Title:  title,
		Status: "completed",
	})
}

// BroadcastTaskFailed broadcasts when a task fails
func BroadcastTaskFailed(taskID, title, errMsg string) {
	dash := GetGlobal()
	if dash == nil {
		return
	}
	dash.Broadcast(EventTaskFailed, TaskEvent{
		TaskID: taskID,
		Title:  title,
		Status: "failed",
		Error:  errMsg,
	})
}

// BroadcastTaskBlocked broadcasts when a task is blocked
func BroadcastTaskBlocked(taskID, title string) {
	dash := GetGlobal()
	if dash == nil {
		return
	}
	dash.Broadcast(EventTaskBlocked, TaskEvent{
		TaskID: taskID,
		Title:  title,
		Status: "blocked",
	})
}

// BroadcastStatsUpdate triggers an immediate stats update
func BroadcastStatsUpdate() {
	dash := GetGlobal()
	if dash == nil {
		return
	}
	stats, err := dash.getStatus()
	if err != nil {
		return
	}
	dash.Broadcast(EventStatsUpdate, stats)
}
