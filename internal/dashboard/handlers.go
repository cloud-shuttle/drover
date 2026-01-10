package dashboard

import (
	"encoding/json"
	"net/http"
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
		validStatuses := []string{"ready", "claimed", "in_progress", "blocked", "completed", "failed"}
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
