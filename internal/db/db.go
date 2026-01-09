// Package db handles database operations for Drover
package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
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

	-- Worktrees track git worktree lifecycle for cleanup
	CREATE TABLE IF NOT EXISTS worktrees (
		task_id TEXT PRIMARY KEY,
		path TEXT NOT NULL,
		branch TEXT NOT NULL,
		created_at INTEGER NOT NULL,
		last_used_at INTEGER NOT NULL,
		status TEXT DEFAULT 'active',
		disk_size INTEGER DEFAULT 0,
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
	);

	-- Indexes for common queries
	CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
	CREATE INDEX IF NOT EXISTS idx_tasks_epic ON tasks(epic_id);
	CREATE INDEX IF NOT EXISTS idx_tasks_priority ON tasks(priority DESC);
	CREATE INDEX IF NOT EXISTS idx_dependencies_blocked_by ON task_dependencies(blocked_by);
	CREATE INDEX IF NOT EXISTS idx_worktrees_status ON worktrees(status);
	CREATE INDEX IF NOT EXISTS idx_worktrees_created ON worktrees(created_at);
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

// ListTasks returns all tasks in the database
func (s *Store) ListTasks() ([]*types.Task, error) {
	rows, err := s.DB.Query(`
		SELECT id, title, COALESCE(description, ''), COALESCE(epic_id, ''),
		       priority, status, attempts, max_attempts,
		       COALESCE(claimed_by, ''), COALESCE(claimed_at, 0),
		       created_at, updated_at
		FROM tasks
		ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("querying tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*types.Task
	for rows.Next() {
		var task types.Task
		var claimedBy sql.NullString
		var claimedAt sql.NullInt64
		var epicID sql.NullString
		var description sql.NullString

		err := rows.Scan(
			&task.ID, &task.Title, &description, &epicID,
			&task.Priority, &task.Status, &task.Attempts, &task.MaxAttempts,
			&claimedBy, &claimedAt,
			&task.CreatedAt, &task.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning task: %w", err)
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

		tasks = append(tasks, &task)
	}

	return tasks, nil
}

// GetBlockedBy returns the list of task IDs that block the given task
func (s *Store) GetBlockedBy(taskID string) ([]string, error) {
	rows, err := s.DB.Query(`
		SELECT blocked_by
		FROM task_dependencies
		WHERE task_id = ?
	`, taskID)
	if err != nil {
		return nil, fmt.Errorf("querying dependencies: %w", err)
	}
	defer rows.Close()

	var blockedBy []string
	for rows.Next() {
		var blockerID string
		if err := rows.Scan(&blockerID); err != nil {
			return nil, fmt.Errorf("scanning dependency: %w", err)
		}
		blockedBy = append(blockedBy, blockerID)
	}

	return blockedBy, nil
}

// ListEpics returns all epics in the database
func (s *Store) ListEpics() ([]*types.Epic, error) {
	rows, err := s.DB.Query(`
		SELECT id, title, COALESCE(description, ''), status, created_at
		FROM epics
		ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("querying epics: %w", err)
	}
	defer rows.Close()

	var epics []*types.Epic
	for rows.Next() {
		var epic types.Epic
		var description sql.NullString

		err := rows.Scan(
			&epic.ID, &epic.Title, &description, &epic.Status, &epic.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning epic: %w", err)
		}

		epic.Description = description.String
		epics = append(epics, &epic)
	}

	return epics, nil
}

// ListAllDependencies returns all task dependencies in the database
func (s *Store) ListAllDependencies() ([]types.TaskDependency, error) {
	rows, err := s.DB.Query(`
		SELECT task_id, blocked_by
		FROM task_dependencies
		ORDER BY task_id, blocked_by
	`)
	if err != nil {
		return nil, fmt.Errorf("querying dependencies: %w", err)
	}
	defer rows.Close()

	var deps []types.TaskDependency
	for rows.Next() {
		var dep types.TaskDependency
		if err := rows.Scan(&dep.TaskID, &dep.BlockedBy); err != nil {
			return nil, fmt.Errorf("scanning dependency: %w", err)
		}
		deps = append(deps, dep)
	}

	return deps, nil
}

// WorktreeInfo represents a worktree with its metadata
type WorktreeInfo struct {
	TaskID      string
	Path        string
	Branch      string
	CreatedAt   int64
	LastUsedAt  int64
	Status      string
	DiskSize    int64
	TaskStatus  string
	TaskTitle   string
}

// CreateWorktree records a new worktree in the database
func (s *Store) CreateWorktree(taskID, path, branch string) error {
	now := time.Now().Unix()
	_, err := s.DB.Exec(`
		INSERT INTO worktrees (task_id, path, branch, created_at, last_used_at, status)
		VALUES (?, ?, ?, ?, ?, 'active')
	`, taskID, path, branch, now, now)
	if err != nil {
		return fmt.Errorf("creating worktree record: %w", err)
	}
	return nil
}

// UpdateWorktreeStatus updates the status of a worktree
func (s *Store) UpdateWorktreeStatus(taskID, status string) error {
	now := time.Now().Unix()
	_, err := s.DB.Exec(`
		UPDATE worktrees
		SET status = ?, last_used_at = ?
		WHERE task_id = ?
	`, status, now, taskID)
	if err != nil {
		return fmt.Errorf("updating worktree status: %w", err)
	}
	return nil
}

// UpdateWorktreeDiskSize updates the disk size of a worktree
func (s *Store) UpdateWorktreeDiskSize(taskID string, size int64) error {
	_, err := s.DB.Exec(`
		UPDATE worktrees
		SET disk_size = ?
		WHERE task_id = ?
	`, size, taskID)
	if err != nil {
		return fmt.Errorf("updating worktree disk size: %w", err)
	}
	return nil
}

// TouchWorktree updates the last_used_at timestamp
func (s *Store) TouchWorktree(taskID string) error {
	now := time.Now().Unix()
	_, err := s.DB.Exec(`
		UPDATE worktrees
		SET last_used_at = ?
		WHERE task_id = ?
	`, now, taskID)
	if err != nil {
		return fmt.Errorf("touching worktree: %w", err)
	}
	return nil
}

// ListWorktrees returns all worktrees with their task information
func (s *Store) ListWorktrees() ([]*WorktreeInfo, error) {
	// Try to query with task information (LEFT JOIN with tasks table)
	rows, err := s.DB.Query(`
		SELECT w.task_id, w.path, w.branch, w.created_at, w.last_used_at,
		       w.status, w.disk_size, COALESCE(t.status, ''), COALESCE(t.title, '')
		FROM worktrees w
		LEFT JOIN tasks t ON w.task_id = t.id
		ORDER BY w.created_at DESC
	`)

	// If the tasks table doesn't exist (DBOS mode), fall back to simpler query
	if err != nil {
		rows, err = s.DB.Query(`
			SELECT task_id, path, branch, created_at, last_used_at, status, disk_size
			FROM worktrees
			ORDER BY created_at DESC
		`)
		if err != nil {
			return nil, fmt.Errorf("querying worktrees: %w", err)
		}
		defer rows.Close()

		var worktrees []*WorktreeInfo
		for rows.Next() {
			var w WorktreeInfo
			err := rows.Scan(
				&w.TaskID, &w.Path, &w.Branch, &w.CreatedAt, &w.LastUsedAt,
				&w.Status, &w.DiskSize,
			)
			if err != nil {
				return nil, fmt.Errorf("scanning worktree: %w", err)
			}
			worktrees = append(worktrees, &w)
		}
		return worktrees, nil
	}
	defer rows.Close()

	var worktrees []*WorktreeInfo
	for rows.Next() {
		var w WorktreeInfo
		err := rows.Scan(
			&w.TaskID, &w.Path, &w.Branch, &w.CreatedAt, &w.LastUsedAt,
			&w.Status, &w.DiskSize, &w.TaskStatus, &w.TaskTitle,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning worktree: %w", err)
		}
		worktrees = append(worktrees, &w)
	}

	return worktrees, nil
}

// GetWorktreesForCleanup returns worktrees that can be cleaned up
func (s *Store) GetWorktreesForCleanup(completedOnly bool) ([]*WorktreeInfo, error) {
	var rows *sql.Rows
	var err error

	if completedOnly {
		// Only return worktrees for completed tasks
		rows, err = s.DB.Query(`
			SELECT w.task_id, w.path, w.branch, w.created_at, w.last_used_at,
			       w.status, w.disk_size, COALESCE(t.status, ''), COALESCE(t.title, '')
			FROM worktrees w
			LEFT JOIN tasks t ON w.task_id = t.id
			WHERE w.status != 'removed' AND (t.status = 'completed' OR t.status = 'failed')
			ORDER BY w.created_at ASC
		`)
	} else {
		// Return all worktrees except removed ones
		rows, err = s.DB.Query(`
			SELECT w.task_id, w.path, w.branch, w.created_at, w.last_used_at,
			       w.status, w.disk_size, COALESCE(t.status, ''), COALESCE(t.title, '')
			FROM worktrees w
			LEFT JOIN tasks t ON w.task_id = t.id
			WHERE w.status != 'removed'
			ORDER BY w.created_at ASC
		`)
	}

	// If the tasks table doesn't exist (DBOS mode), fall back to simpler query
	if err != nil {
		if completedOnly {
			// In DBOS mode without task status, just return all active worktrees
			// The user will need to manually select which to clean up
			rows, err = s.DB.Query(`
				SELECT task_id, path, branch, created_at, last_used_at, status, disk_size
				FROM worktrees
				WHERE status != 'removed'
				ORDER BY created_at ASC
			`)
		} else {
			rows, err = s.DB.Query(`
				SELECT task_id, path, branch, created_at, last_used_at, status, disk_size
				FROM worktrees
				WHERE status != 'removed'
				ORDER BY created_at ASC
			`)
		}
		if err != nil {
			return nil, fmt.Errorf("querying worktrees for cleanup: %w", err)
		}
		defer rows.Close()

		var worktrees []*WorktreeInfo
		for rows.Next() {
			var w WorktreeInfo
			err := rows.Scan(
				&w.TaskID, &w.Path, &w.Branch, &w.CreatedAt, &w.LastUsedAt,
				&w.Status, &w.DiskSize,
			)
			if err != nil {
				return nil, fmt.Errorf("scanning worktree: %w", err)
			}
			worktrees = append(worktrees, &w)
		}
		return worktrees, nil
	}
	defer rows.Close()

	var worktrees []*WorktreeInfo
	for rows.Next() {
		var w WorktreeInfo
		err := rows.Scan(
			&w.TaskID, &w.Path, &w.Branch, &w.CreatedAt, &w.LastUsedAt,
			&w.Status, &w.DiskSize, &w.TaskStatus, &w.TaskTitle,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning worktree: %w", err)
		}
		worktrees = append(worktrees, &w)
	}

	return worktrees, nil
}

// DeleteWorktree removes a worktree record from the database
func (s *Store) DeleteWorktree(taskID string) error {
	_, err := s.DB.Exec(`
		DELETE FROM worktrees WHERE task_id = ?
	`, taskID)
	if err != nil {
		return fmt.Errorf("deleting worktree record: %w", err)
	}
	return nil
}

// GetOrphanedWorktrees returns worktrees that exist on disk but not in the database
// or have no corresponding task (task was deleted)
func (s *Store) GetOrphanedWorktrees(worktreeDir string) ([]string, error) {
	// Get all task IDs that have worktrees
	rows, err := s.DB.Query(`
		SELECT task_id FROM worktrees WHERE status != 'removed'
	`)
	if err != nil {
		return nil, fmt.Errorf("querying active worktrees: %w", err)
	}
	defer rows.Close()

	activeTaskIDs := make(map[string]bool)
	for rows.Next() {
		var taskID string
		if err := rows.Scan(&taskID); err != nil {
			continue
		}
		activeTaskIDs[taskID] = true
	}

	// Check what directories exist on disk
	entries, err := os.ReadDir(worktreeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("reading worktree directory: %w", err)
	}

	var orphaned []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		taskID := entry.Name()
		// Skip if it's an active worktree in the database
		if activeTaskIDs[taskID] {
			continue
		}
		orphaned = append(orphaned, filepath.Join(worktreeDir, taskID))
	}

	return orphaned, nil
}

// GetWorktreeStats returns statistics about worktrees
func (s *Store) GetWorktreeStats() (map[string]int64, error) {
	stats := make(map[string]int64)

	// Count by status
	rows, err := s.DB.Query(`
		SELECT status, COUNT(*), COALESCE(SUM(disk_size), 0) FROM worktrees GROUP BY status
	`)
	if err != nil {
		return nil, fmt.Errorf("querying worktree stats: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var count int64
		var totalSize int64
		if err := rows.Scan(&status, &count, &totalSize); err != nil {
			continue
		}
		stats[status+"_count"] = count
		stats[status+"_size"] = totalSize
	}

	return stats, nil
}
