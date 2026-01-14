package dashboard

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
)

// handleStatus returns the overall project statistics
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	stats, err := s.getStatus()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, stats)
}

// handleEpics returns all epics with task counts
func (s *Server) handleEpics(w http.ResponseWriter, r *http.Request) {
	epics, err := s.getEpics()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, epics)
}

// handleTasks returns tasks with optional filters
func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	epic := r.URL.Query().Get("epic")
	status := r.URL.Query().Get("status")

	// Validate status if provided
	if status != "" {
		validStatuses := []string{"ready", "claimed", "in_progress", "paused", "blocked", "completed", "failed"}
		if !slices.Contains(validStatuses, status) {
			http.Error(w, "invalid status", http.StatusBadRequest)
			return
		}
	}

	tasks, err := s.getTasks(epic, status)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, tasks)
}

// handleTask returns a single task by ID
func (s *Server) handleTask(w http.ResponseWriter, r *http.Request) {
	// Extract ID from path "/api/tasks/{id}"
	path := r.URL.Path
	prefix := "/api/tasks/"
	if !strings.HasPrefix(path, prefix) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	id := strings.TrimPrefix(path, prefix)

	task, err := s.getTask(id)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			http.Error(w, "task not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, task)
}

// handlePauseTask pauses a running task
func (s *Server) handlePauseTask(w http.ResponseWriter, r *http.Request) {
	// Extract ID from path "/api/tasks/{id}/pause"
	path := r.URL.Path
	prefix := "/api/tasks/"
	if !strings.HasPrefix(path, prefix) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	suffix := "/pause"
	if !strings.HasSuffix(path, suffix) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	id := strings.TrimPrefix(strings.TrimSuffix(path, suffix), prefix)

	if err := s.store.PauseTask(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Broadcast pause event
	s.broadcastTaskPaused(id)

	jsonResponse(w, map[string]string{"status": "paused", "id": id})
}

// handleResumeTask resumes a paused task
func (s *Server) handleResumeTask(w http.ResponseWriter, r *http.Request) {
	// Extract ID from path "/api/tasks/{id}/resume"
	path := r.URL.Path
	prefix := "/api/tasks/"
	if !strings.HasPrefix(path, prefix) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	suffix := "/resume"
	if !strings.HasSuffix(path, suffix) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	id := strings.TrimPrefix(strings.TrimSuffix(path, suffix), prefix)

	if err := s.store.ResumeTask(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Broadcast resume event
	s.broadcastTaskResumed(id)

	jsonResponse(w, map[string]string{"status": "resumed", "id": id})
}

// handleAddGuidance adds guidance to a task
func (s *Server) handleAddGuidance(w http.ResponseWriter, r *http.Request) {
	// Extract ID from path "/api/tasks/{id}/guidance"
	path := r.URL.Path
	prefix := "/api/tasks/"
	if !strings.HasPrefix(path, prefix) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	suffix := "/guidance"
	if !strings.HasSuffix(path, suffix) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	id := strings.TrimPrefix(strings.TrimSuffix(path, suffix), prefix)

	// Parse request body
	var req struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Message == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}

	// Add guidance
	guidance, err := s.store.AddGuidance(id, req.Message)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Broadcast guidance event
	s.broadcastTaskGuidance(id, guidance.Message)

	jsonResponse(w, map[string]string{"status": "added", "id": guidance.ID})
}

// handleWorkers returns active worker information
func (s *Server) handleWorkers(w http.ResponseWriter, r *http.Request) {
	workers, err := s.getWorkers()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, workers)
}

// handleGraph returns the dependency graph
func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	graph, err := s.getGraph()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, graph)
}

// jsonResponse writes JSON response
func jsonResponse(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// handleTaskAction routes POST requests for task actions (pause, resume, guidance)
func (s *Server) handleTaskAction(w http.ResponseWriter, r *http.Request) {
	// Extract ID and action from path "/api/tasks/{id}/{action}"
	path := r.URL.Path
	prefix := "/api/tasks/"
	if !strings.HasPrefix(path, prefix) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	suffix := strings.TrimPrefix(path, prefix)

	parts := strings.Split(suffix, "/")
	if len(parts) != 2 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	action := parts[1]

	switch action {
	case "pause":
		s.handlePauseTask(w, r)
	case "resume":
		s.handleResumeTask(w, r)
	case "guidance":
		s.handleAddGuidance(w, r)
	default:
		http.Error(w, "unknown action", http.StatusBadRequest)
	}
}

// broadcastTaskPaused broadcasts a task paused event
func (s *Server) broadcastTaskPaused(taskID string) {
	s.Broadcast(EventTaskPaused, map[string]string{
		"task_id": taskID,
	})
}

// broadcastTaskResumed broadcasts a task resumed event
func (s *Server) broadcastTaskResumed(taskID string) {
	s.Broadcast(EventTaskResumed, map[string]string{
		"task_id": taskID,
	})
}

// broadcastTaskGuidance broadcasts a task guidance event
func (s *Server) broadcastTaskGuidance(taskID, message string) {
	s.Broadcast(EventTaskGuidance, map[string]string{
		"task_id": taskID,
		"message": message,
	})
}

// handleWorktreeFiles returns files in a task's worktree
func (s *Server) handleWorktreeFiles(w http.ResponseWriter, r *http.Request) {
	// Extract task ID and optional path from "/api/worktrees/{taskID}/files?path=xxx"
	path := r.URL.Path
	prefix := "/api/worktrees/"
	if !strings.HasPrefix(path, prefix) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// Parse: /api/worktrees/{taskID}/files
	suffix := strings.TrimPrefix(path, prefix)
	parts := strings.Split(suffix, "/")
	if len(parts) < 2 || parts[1] != "files" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	taskID := parts[0]
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		filePath = "."
	}

	files, err := s.getWorktreeFiles(taskID, filePath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, files)
}

// handleWorktreeFileContents returns file contents from a task's worktree
func (s *Server) handleWorktreeFileContents(w http.ResponseWriter, r *http.Request) {
	// Extract task ID and file path from "/api/worktrees/{taskID}/contents?path=xxx"
	path := r.URL.Path
	prefix := "/api/worktrees/"
	if !strings.HasPrefix(path, prefix) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	suffix := strings.TrimPrefix(path, prefix)
	parts := strings.Split(suffix, "/")
	if len(parts) < 2 || parts[1] != "contents" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	taskID := parts[0]
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}

	content, err := s.getWorktreeFileContents(taskID, filePath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Detect content type for syntax highlighting
	ext := strings.TrimPrefix(filepath.Base(filePath), ".")
	contentType := "text/plain"
	switch ext {
	case "go", "js", "ts", "json", "xml", "yaml", "yml", "md", "txt", "sh":
		contentType = "text/plain"
	case "html":
		contentType = "text/html"
	case "css":
		contentType = "text/css"
	}

	w.Header().Set("Content-Type", contentType)
	w.Write([]byte(content))
}

// handleWorktreeAPI routes worktree requests based on suffix
func (s *Server) handleWorktreeAPI(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if strings.HasSuffix(path, "/contents") {
		s.handleWorktreeFileContents(w, r)
	} else if strings.HasSuffix(path, "/files") {
		s.handleWorktreeFiles(w, r)
	} else {
		http.Error(w, "not found", http.StatusNotFound)
	}
}
