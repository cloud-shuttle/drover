package dashboard

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Stats represents overall project statistics
type Stats struct {
	Total      int `json:"total"`
	Ready      int `json:"ready"`
	Claimed    int `json:"claimed"`
	InProgress int `json:"in_progress"`
	Paused     int `json:"paused"`
	Blocked    int `json:"blocked"`
	Completed  int `json:"completed"`
	Failed     int `json:"failed"`
	Progress   int `json:"progress"` // Percentage
}

// EpicWithCount represents an epic with task counts
type EpicWithCount struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	TaskCount   int    `json:"task_count"`
	Completed   int    `json:"completed"`
	Ready       int    `json:"ready"`
	Active      int    `json:"active"`
}

// TaskWithEpic represents a task with epic information
type TaskWithEpic struct {
	ID             string  `json:"id"`
	Title          string  `json:"title"`
	Description    string  `json:"description"`
	EpicID         string  `json:"epic_id"`
	EpicTitle      string  `json:"epic_title"`
	ParentID       string  `json:"parent_id"`
	SequenceNumber int     `json:"sequence_number"`
	Priority       int     `json:"priority"`
	Status         string  `json:"status"`
	Attempts       int     `json:"attempts"`
	MaxAttempts    int     `json:"max_attempts"`
	LastError      string  `json:"last_error"`
	ClaimedBy      string  `json:"claimed_by"`
	ClaimedAt      int64   `json:"claimed_at"`
	Operator       string  `json:"operator"`
	CreatedAt      int64   `json:"created_at"`
	UpdatedAt      int64   `json:"updated_at"`
}

// WorkerInfo represents active worker information
type WorkerInfo struct {
	WorkerID  string `json:"worker_id"`
	TaskID    string `json:"task_id"`
	Title     string `json:"title"`
	Duration  int64  `json:"duration"` // Seconds since claim
}

// GraphEdge represents a dependency edge
type GraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// GraphNode represents a task in the dependency graph
type GraphNode struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

// Graph represents the full dependency graph
type Graph struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// getStatus retrieves overall project statistics
func (s *Server) getStatus() (*Stats, error) {
	stats := &Stats{}

	// Count by status
	rows, err := s.db.Query(`
		SELECT status, COUNT(*) FROM tasks GROUP BY status
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			continue
		}
		switch status {
		case "ready":
			stats.Ready = count
		case "claimed":
			stats.Claimed = count
		case "in_progress":
			stats.InProgress = count
		case "paused":
			stats.Paused = count
		case "blocked":
			stats.Blocked = count
		case "completed":
			stats.Completed = count
		case "failed":
			stats.Failed = count
		}
	}

	stats.Total = stats.Ready + stats.Claimed + stats.InProgress +
		stats.Paused + stats.Blocked + stats.Completed + stats.Failed

	// Calculate progress percentage
	if stats.Total > 0 {
		stats.Progress = int((stats.Completed * 100) / stats.Total)
	}

	return stats, nil
}

// getEpics retrieves all epics with task counts
func (s *Server) getEpics() ([]EpicWithCount, error) {
	query := `
		SELECT
			e.id,
			e.title,
			COALESCE(e.description, ''),
			e.status,
			COALESCE(COUNT(t.id), 0) as task_count,
			COALESCE(SUM(CASE WHEN t.status = 'completed' THEN 1 ELSE 0 END), 0) as completed,
			COALESCE(SUM(CASE WHEN t.status = 'ready' THEN 1 ELSE 0 END), 0) as ready,
			COALESCE(SUM(CASE WHEN t.status IN ('claimed', 'in_progress') THEN 1 ELSE 0 END), 0) as active
		FROM epics e
		LEFT JOIN tasks t ON e.id = t.epic_id
		GROUP BY e.id
		ORDER BY e.created_at ASC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var epics []EpicWithCount
	for rows.Next() {
		var e EpicWithCount
		if err := rows.Scan(&e.ID, &e.Title, &e.Description, &e.Status,
			&e.TaskCount, &e.Completed, &e.Ready, &e.Active); err != nil {
			continue
		}
		epics = append(epics, e)
	}

	return epics, nil
}

// getTasks retrieves tasks with optional filters
func (s *Server) getTasks(epic, status string) ([]TaskWithEpic, error) {
	var rows *sql.Rows
	var err error

	query := `
		SELECT
			t.id, t.title, COALESCE(t.description, ''),
			COALESCE(t.epic_id, ''), COALESCE(e.title, ''),
			COALESCE(t.parent_id, ''), t.sequence_number,
			t.priority, t.status, t.attempts, t.max_attempts,
			COALESCE(t.last_error, ''),
			COALESCE(t.claimed_by, ''), COALESCE(t.claimed_at, 0),
			COALESCE(t.operator, ''),
			t.created_at, t.updated_at
		FROM tasks t
		LEFT JOIN epics e ON t.epic_id = e.id
	`

	// Build WHERE clause
	whereClause := ""
	args := []interface{}{}

	if epic != "" && status != "" {
		whereClause = " WHERE t.epic_id = ? AND t.status = ?"
		args = []interface{}{epic, status}
	} else if epic != "" {
		whereClause = " WHERE t.epic_id = ?"
		args = []interface{}{epic}
	} else if status != "" {
		whereClause = " WHERE t.status = ?"
		args = []interface{}{status}
	}

	query += whereClause + " ORDER BY t.priority DESC, t.created_at ASC"

	if len(args) > 0 {
		rows, err = s.db.Query(query, args...)
	} else {
		rows, err = s.db.Query(query)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []TaskWithEpic
	for rows.Next() {
		var t TaskWithEpic
		if err := rows.Scan(
			&t.ID, &t.Title, &t.Description,
			&t.EpicID, &t.EpicTitle,
			&t.ParentID, &t.SequenceNumber,
			&t.Priority, &t.Status, &t.Attempts, &t.MaxAttempts,
			&t.LastError,
			&t.ClaimedBy, &t.ClaimedAt,
			&t.Operator,
			&t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			continue
		}
		tasks = append(tasks, t)
	}

	return tasks, nil
}

// getTask retrieves a single task by ID
func (s *Server) getTask(id string) (*TaskWithEpic, error) {
	query := `
		SELECT
			t.id, t.title, COALESCE(t.description, ''),
			COALESCE(t.epic_id, ''), COALESCE(e.title, ''),
			COALESCE(t.parent_id, ''), t.sequence_number,
			t.priority, t.status, t.attempts, t.max_attempts,
			COALESCE(t.last_error, ''),
			COALESCE(t.claimed_by, ''), COALESCE(t.claimed_at, 0),
			COALESCE(t.operator, ''),
			t.created_at, t.updated_at
		FROM tasks t
		LEFT JOIN epics e ON t.epic_id = e.id
		WHERE t.id = ?
	`

	var t TaskWithEpic
	err := s.db.QueryRow(query, id).Scan(
		&t.ID, &t.Title, &t.Description,
		&t.EpicID, &t.EpicTitle,
		&t.ParentID, &t.SequenceNumber,
		&t.Priority, &t.Status, &t.Attempts, &t.MaxAttempts,
		&t.LastError,
		&t.ClaimedBy, &t.ClaimedAt,
		&t.Operator,
		&t.CreatedAt, &t.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	return &t, nil
}

// getWorkers retrieves active worker information
func (s *Server) getWorkers() ([]WorkerInfo, error) {
	query := `
		SELECT
			claimed_by,
			id,
			title,
			claimed_at
		FROM tasks
		WHERE status IN ('claimed', 'in_progress')
		AND claimed_by IS NOT NULL
		ORDER BY claimed_at ASC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workers []WorkerInfo
	now := time.Now().Unix()

	for rows.Next() {
		var w WorkerInfo
		var claimedAt int64
		if err := rows.Scan(&w.WorkerID, &w.TaskID, &w.Title, &claimedAt); err != nil {
			continue
		}
		w.Duration = now - claimedAt
		workers = append(workers, w)
	}

	return workers, nil
}

// getGraph retrieves the dependency graph
func (s *Server) getGraph() (*Graph, error) {
	graph := &Graph{
		Nodes: []GraphNode{},
		Edges: []GraphEdge{},
	}

	// Get all tasks as nodes
	nodeQuery := `
		SELECT id, title, status
		FROM tasks
		ORDER BY created_at ASC
	`

	rows, err := s.db.Query(nodeQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var n GraphNode
		if err := rows.Scan(&n.ID, &n.Title, &n.Status); err != nil {
			continue
		}
		graph.Nodes = append(graph.Nodes, n)
	}

	// Get all dependencies as edges
	edgeQuery := `
		SELECT task_id, blocked_by
		FROM task_dependencies
		ORDER BY task_id, blocked_by
	`

	rows, err = s.db.Query(edgeQuery)
	if err != nil {
		return graph, nil // Return nodes even if edges fail
	}
	defer rows.Close()

	for rows.Next() {
		var e GraphEdge
		if err := rows.Scan(&e.To, &e.From); err != nil {
			continue
		}
		graph.Edges = append(graph.Edges, e)
	}

	return graph, nil
}

// WorktreeFile represents a file in a worktree
type WorktreeFile struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Type     string `json:"type"` // "file" or "dir"
	Size     int64  `json:"size"`
	Modified int64  `json:"modified,omitempty"`
}

// getWorktreeFiles lists files in a task's worktree
func (s *Server) getWorktreeFiles(taskID, path string) ([]WorktreeFile, error) {
	// Get task info to verify it exists and get worktree details
	var worktreePath string
	var taskStatus string
	err := s.db.QueryRow(`
		SELECT w.path, t.status
		FROM worktrees w
		LEFT JOIN tasks t ON w.task_id = t.id
		WHERE w.task_id = ?
	`, taskID).Scan(&worktreePath, &taskStatus)
	if err != nil {
		return nil, err
	}

	// Build full path to requested directory
	fullPath := filepath.Join(worktreePath, path)

	// Read directory
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return nil, err
	}

	var files []WorktreeFile
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		fileType := "file"
		if entry.IsDir() {
			fileType = "dir"
		}

		files = append(files, WorktreeFile{
			Name: entry.Name(),
			Path: filepath.Join(path, entry.Name()),
			Type: fileType,
			Size: info.Size(),
			Modified: info.ModTime().Unix(),
		})
	}

	return files, nil
}

// getWorktreeFileContents reads a file's contents from a worktree
func (s *Server) getWorktreeFileContents(taskID, filePath string) (string, error) {
	// Get worktree path
	var worktreePath string
	err := s.db.QueryRow(`
		SELECT path FROM worktrees WHERE task_id = ?
	`, taskID).Scan(&worktreePath)
	if err != nil {
		return "", err
	}

	// Security check: prevent directory traversal
	cleanPath := filepath.Join(worktreePath, filePath)
	if !strings.HasPrefix(cleanPath, worktreePath) {
		return "", fmt.Errorf("invalid file path")
	}

	// Read file (limit to 1MB for security)
	content, err := os.ReadFile(cleanPath)
	if err != nil {
		return "", err
	}

	if len(content) > 1024*1024 {
		return "", fmt.Errorf("file too large")
	}

	return string(content), nil
}
