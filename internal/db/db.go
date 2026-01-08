// Package db handles database operations for Drover
package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/cloud-shuttle/drover/pkg/types"
	_ "github.com/mattn/go-sqlite3"
)

// Store manages database operations
type Store struct {
	DB *sql.DB
}

// ProjectStatus summarizes the current state
type ProjectStatus struct {
	Total      int
	Ready      int
	Claimed    int
	InProgress int
	Blocked    int
	Completed  int
	Failed     int
}

// Open opens a SQLite database at the given path
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, fmt.Errorf("enabling foreign keys: %w", err)
	}

	// Enable WAL mode for better concurrent access
	if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		return nil, fmt.Errorf("enabling WAL mode: %w", err)
	}

	// Set busy timeout to handle lock contention gracefully
	if _, err := db.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		return nil, fmt.Errorf("setting busy timeout: %w", err)
	}

	return &Store{DB: db}, nil
}

// Close closes the database connection
func (s *Store) Close() error {
	return s.DB.Close()
}

// InitSchema creates the database schema
func (s *Store) InitSchema() error {
	schema := `
	-- Epics group related tasks
	CREATE TABLE IF NOT EXISTS epics (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		description TEXT,
		status TEXT DEFAULT 'open',
		created_at INTEGER NOT NULL
	);

	-- Tasks are the unit of work
	CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		description TEXT,
		epic_id TEXT,
		priority INTEGER DEFAULT 0,
		status TEXT DEFAULT 'ready',
		attempts INTEGER DEFAULT 0,
		max_attempts INTEGER DEFAULT 3,
		last_error TEXT,
		claimed_by TEXT,
		claimed_at INTEGER,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		FOREIGN KEY (epic_id) REFERENCES epics(id)
	);

	-- Dependencies define blocked-by relationships
	CREATE TABLE IF NOT EXISTS task_dependencies (
		task_id TEXT NOT NULL,
		blocked_by TEXT NOT NULL,
		PRIMARY KEY (task_id, blocked_by),
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
		FOREIGN KEY (blocked_by) REFERENCES tasks(id) ON DELETE CASCADE
	);

	-- Indexes for common queries
	CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
	CREATE INDEX IF NOT EXISTS idx_tasks_epic ON tasks(epic_id);
	CREATE INDEX IF NOT EXISTS idx_tasks_priority ON tasks(priority DESC);
	CREATE INDEX IF NOT EXISTS idx_dependencies_blocked_by ON task_dependencies(blocked_by);
	`

	_, err := s.DB.Exec(schema)
	return err
}

// CreateEpic creates a new epic
func (s *Store) CreateEpic(title, description string) (*types.Epic, error) {
	id := generateID("epic")
	now := time.Now().Unix()

	epic := &types.Epic{
		ID:          id,
		Title:       title,
		Description: description,
		Status:      types.EpicStatusOpen,
		CreatedAt:   now,
	}

	_, err := s.DB.Exec(`
		INSERT INTO epics (id, title, description, status, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, epic.ID, epic.Title, epic.Description, epic.Status, epic.CreatedAt)

	if err != nil {
		return nil, fmt.Errorf("creating epic: %w", err)
	}

	return epic, nil
}

// CreateTask creates a new task with optional dependencies
func (s *Store) CreateTask(title, description, epicID string, priority int, blockedBy []string) (*types.Task, error) {
	id := generateID("task")
	now := time.Now().Unix()

	task := &types.Task{
		ID:          id,
		Title:       title,
		Description: description,
		EpicID:      epicID,
		Priority:    priority,
		Status:      types.TaskStatusReady,
		MaxAttempts: 3,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// Check if task should start as blocked
	if len(blockedBy) > 0 {
		task.Status = types.TaskStatusBlocked
	}

	tx, err := s.DB.Begin()
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert task (convert empty epic_id to NULL for foreign key constraint)
	var epicIDValue interface{} = task.EpicID
	if epicIDValue == "" {
		epicIDValue = nil
	}
	_, err = tx.Exec(`
		INSERT INTO tasks (id, title, description, epic_id, priority, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, task.ID, task.Title, task.Description, epicIDValue, task.Priority, task.Status, task.CreatedAt, task.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("creating task: %w", err)
	}

	// Insert dependencies
	for _, blockerID := range blockedBy {
		_, err = tx.Exec(`
			INSERT INTO task_dependencies (task_id, blocked_by)
			VALUES (?, ?)
		`, task.ID, blockerID)
		if err != nil {
			return nil, fmt.Errorf("adding dependency: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing transaction: %w", err)
	}

	return task, nil
}

// GetProjectStatus returns overall project status
func (s *Store) GetProjectStatus() (*ProjectStatus, error) {
	status := &ProjectStatus{}

	// Count by status
	rows, err := s.DB.Query(`
		SELECT status, COUNT(*) FROM tasks GROUP BY status
	`)
	if err != nil {
		return nil, fmt.Errorf("querying status: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var taskStatus string
		var count int
		if err := rows.Scan(&taskStatus, &count); err != nil {
			continue
		}
		switch types.TaskStatus(taskStatus) {
		case types.TaskStatusReady:
			status.Ready = count
		case types.TaskStatusClaimed:
			status.Claimed = count
		case types.TaskStatusInProgress:
			status.InProgress = count
		case types.TaskStatusBlocked:
			status.Blocked = count
		case types.TaskStatusCompleted:
			status.Completed = count
		case types.TaskStatusFailed:
			status.Failed = count
		}
	}

	status.Total = status.Ready + status.Claimed + status.InProgress +
		status.Blocked + status.Completed + status.Failed

	return status, nil
}

// ClaimTask attempts to atomically claim a ready task
//
// Uses UPDATE with ORDER BY and LIMIT to atomically find and claim a task
// in a single operation, avoiding race conditions between SELECT and UPDATE.
func (s *Store) ClaimTask(workerID string) (*types.Task, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	now := time.Now().Unix()

	// Atomically find and claim a ready task (highest priority first)
	// by using UPDATE with ORDER BY and LIMIT. This ensures that even if
	// multiple workers execute this concurrently, each will claim a different
	// task or none at all, without any race condition.
	var task types.Task
	err = tx.QueryRow(`
		UPDATE tasks
		SET status = 'claimed',
		    claimed_by = ?,
		    claimed_at = ?,
		    updated_at = ?
		WHERE id = (
			SELECT id FROM tasks
			WHERE status = 'ready'
			ORDER BY priority DESC, created_at ASC
			LIMIT 1
		)
		RETURNING id, title, COALESCE(description, ''), COALESCE(epic_id, ''),
		          priority, status, attempts, max_attempts, created_at, updated_at
	`, workerID, now, now).Scan(&task.ID, &task.Title, &task.Description, &task.EpicID,
		&task.Priority, &task.Status, &task.Attempts, &task.MaxAttempts,
		&task.CreatedAt, &task.UpdatedAt)

	if err == sql.ErrNoRows {
		// No tasks were claimed - either no ready tasks exist, or another worker
		// claimed the last ready task between our subquery read and the UPDATE.
		// Either way, returning nil is the correct behavior.
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claiming task: %w", err)
	}

	task.Status = types.TaskStatusClaimed
	task.ClaimedBy = workerID
	task.ClaimedAt = &now

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing claim: %w", err)
	}

	return &task, nil
}

// GetTaskStatus returns the current status of a task
func (s *Store) GetTaskStatus(taskID string) (types.TaskStatus, error) {
	var status string
	err := s.DB.QueryRow(`
		SELECT status FROM tasks WHERE id = ?
	`, taskID).Scan(&status)
	if err != nil {
		return "", err
	}
	return types.TaskStatus(status), nil
}

// UpdateTaskStatus updates a task's status
func (s *Store) UpdateTaskStatus(taskID string, status types.TaskStatus, lastError string) error {
	now := time.Now().Unix()
	_, err := s.DB.Exec(`
		UPDATE tasks
		SET status = ?, last_error = ?, updated_at = ?
		WHERE id = ?
	`, status, lastError, now, taskID)
	return err
}

// IncrementTaskAttempts increments the attempt counter for a task
func (s *Store) IncrementTaskAttempts(taskID string) error {
	now := time.Now().Unix()
	_, err := s.DB.Exec(`
		UPDATE tasks
		SET attempts = attempts + 1, updated_at = ?
		WHERE id = ?
	`, now, taskID)
	return err
}

// GetTask retrieves a task by ID
func (s *Store) GetTask(taskID string) (*types.Task, error) {
	var task types.Task
	var claimedBy sql.NullString
	var claimedAt sql.NullInt64
	var epicID sql.NullString
	var description sql.NullString

	err := s.DB.QueryRow(`
		SELECT id, title, COALESCE(description, ''), COALESCE(epic_id, ''),
		       priority, status, attempts, max_attempts,
		       COALESCE(claimed_by, ''), COALESCE(claimed_at, 0),
		       created_at, updated_at
		FROM tasks
		WHERE id = ?
	`, taskID).Scan(
		&task.ID, &task.Title, &description, &epicID,
		&task.Priority, &task.Status, &task.Attempts, &task.MaxAttempts,
		&claimedBy, &claimedAt,
		&task.CreatedAt, &task.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	task.Description = description.String
	task.EpicID = epicID.String
	if claimedBy.Valid {
		task.ClaimedBy = claimedBy.String
	}
	if claimedAt.Valid {
		unix := claimedAt.Int64
		task.ClaimedAt = &unix
	}

	return &task, nil
}

// CompleteTask marks a task as completed and unblocks dependents
func (s *Store) CompleteTask(taskID string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Mark as completed
	now := time.Now().Unix()
	_, err = tx.Exec(`
		UPDATE tasks
		SET status = 'completed', claimed_by = NULL, updated_at = ?
		WHERE id = ?
	`, now, taskID)
	if err != nil {
		return err
	}

	// Find tasks blocked by this one
	rows, err := tx.Query(`
		SELECT td.task_id
		FROM task_dependencies td
		WHERE td.blocked_by = ?
	`, taskID)
	if err != nil {
		return err
	}
	defer rows.Close()

	var dependentIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		dependentIDs = append(dependentIDs, id)
	}

	// For each dependent, check if all blockers are complete
	for _, depID := range dependentIDs {
		var remainingCount int
		err = tx.QueryRow(`
			SELECT COUNT(*)
			FROM task_dependencies td
			JOIN tasks t ON td.blocked_by = t.id
			WHERE td.task_id = ? AND t.status != 'completed'
		`, depID).Scan(&remainingCount)
		if err != nil {
			continue
		}

		// If no remaining blockers, mark as ready
		if remainingCount == 0 {
			_, err = tx.Exec(`
				UPDATE tasks
				SET status = 'ready', updated_at = ?
				WHERE id = ?
			`, now, depID)
			if err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

// ResetTasks resets tasks with given statuses back to ready
func (s *Store) ResetTasks(statusesToReset []types.TaskStatus) (int, error) {
	now := time.Now().Unix()

	// Build placeholder list for SQL IN clause
	placeholders := make([]string, len(statusesToReset))
	args := make([]interface{}, len(statusesToReset)+1)
	// Put timestamp first since it's for updated_at = ?
	args[0] = now
	for i, status := range statusesToReset {
		placeholders[i] = "?"
		args[i+1] = string(status)
	}

	// Reset tasks to ready status
	query := fmt.Sprintf(`
		UPDATE tasks
		SET status = 'ready', claimed_by = NULL, claimed_at = NULL,
		    attempts = 0, last_error = NULL, updated_at = ?
		WHERE status IN (%s)
	`, fmt.Sprintf("%s", strings.Join(placeholders, ", ")))

	result, err := s.DB.Exec(query, args...)
	if err != nil {
		return 0, fmt.Errorf("resetting tasks: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("getting affected rows: %w", err)
	}

	return int(rowsAffected), nil
}

// generateID generates a unique ID with the given prefix
func generateID(prefix string) string {
	// Simple ID generation - in production use UUID or similar
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}
