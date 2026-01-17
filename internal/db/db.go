// Package db handles database operations for Drover
package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloud-shuttle/drover/pkg/types"
	_ "github.com/glebarez/go-sqlite"
)

// Plan types for database storage
// These mirror the types in internal/modes but are defined here to avoid circular dependencies

// PlanStatus represents the approval status of a plan
type PlanStatus string

const (
	PlanStatusDraft     PlanStatus = "draft"
	PlanStatusPending   PlanStatus = "pending"
	PlanStatusApproved  PlanStatus = "approved"
	PlanStatusRejected  PlanStatus = "rejected"
	PlanStatusExecuting PlanStatus = "executing"
	PlanStatusCompleted PlanStatus = "completed"
	PlanStatusFailed    PlanStatus = "failed"
)

// PlanStep represents a single step in the implementation plan
type PlanStep struct {
	Order         int           `json:"order"`
	Title         string        `json:"title"`
	Description   string        `json:"description"`
	Command       string        `json:"command,omitempty"`
	Files         []string      `json:"files,omitempty"`
	Dependencies  []int         `json:"dependencies,omitempty"`
	EstimatedTime time.Duration `json:"estimated_time,omitempty"`
	Verification  string        `json:"verification,omitempty"`
}

// FileSpec represents a file operation
type FileSpec struct {
	Path           string `json:"path"`
	Operation      string `json:"operation"`
	Reason         string `json:"reason,omitempty"`
	EstimatedLines int    `json:"estimated_lines,omitempty"`
}

// Plan represents a stored implementation plan
type Plan struct {
	ID              string        `json:"id"`
	TaskID          string        `json:"task_id"`
	Title           string        `json:"title"`
	Description     string        `json:"description"`
	Steps           []PlanStep    `json:"steps"`
	FilesToCreate   []FileSpec    `json:"files_to_create,omitempty"`
	FilesToModify   []FileSpec    `json:"files_to_modify,omitempty"`
	Dependencies    []string      `json:"dependencies,omitempty"`
	EstimatedTime   time.Duration `json:"estimated_time,omitempty"`
	Complexity      string        `json:"complexity,omitempty"`
	RiskFactors     []string      `json:"risk_factors,omitempty"`
	Status          PlanStatus    `json:"status"`
	ApprovedBy      string        `json:"approved_by,omitempty"`
	ApprovedAt      *time.Time    `json:"approved_at,omitempty"`
	RejectionReason string        `json:"rejection_reason,omitempty"`
	Revision        int           `json:"revision"`
	ParentPlanID    string        `json:"parent_plan_id,omitempty"`
	Feedback        []string      `json:"feedback,omitempty"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
	CreatedBy       string        `json:"created_by,omitempty"`
}

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
	Paused     int
	Blocked    int
	Completed  int
	Failed     int
}

// Open opens a SQLite database at the given path
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
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
		parent_id TEXT,
		sequence_number INTEGER DEFAULT 0,
		type TEXT DEFAULT 'other',
		priority INTEGER DEFAULT 0,
		status TEXT DEFAULT 'ready',
		attempts INTEGER DEFAULT 0,
		max_attempts INTEGER DEFAULT 3,
		last_error TEXT,
		claimed_by TEXT,
		claimed_at INTEGER,
		operator TEXT DEFAULT '',
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		FOREIGN KEY (epic_id) REFERENCES epics(id),
		FOREIGN KEY (parent_id) REFERENCES tasks(id) ON DELETE CASCADE
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

	-- Guidance queue for human-in-the-loop intervention
	CREATE TABLE IF NOT EXISTS guidance_queue (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		message TEXT NOT NULL,
		created_at INTEGER NOT NULL,
		delivered INTEGER DEFAULT 0,
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
	);

	-- Operators for multiplayer collaboration
	CREATE TABLE IF NOT EXISTS operators (
		id TEXT PRIMARY KEY,
		name TEXT UNIQUE NOT NULL,
		api_key TEXT UNIQUE NOT NULL,
		created_at INTEGER NOT NULL,
		last_active INTEGER
	);

	-- Indexes for common queries
	CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
	CREATE INDEX IF NOT EXISTS idx_tasks_epic ON tasks(epic_id);
	CREATE INDEX IF NOT EXISTS idx_tasks_parent ON tasks(parent_id);
	CREATE INDEX IF NOT EXISTS idx_tasks_parent_seq ON tasks(parent_id, sequence_number);
	CREATE INDEX IF NOT EXISTS idx_tasks_priority ON tasks(priority DESC);
	CREATE INDEX IF NOT EXISTS idx_dependencies_blocked_by ON task_dependencies(blocked_by);
	CREATE INDEX IF NOT EXISTS idx_worktrees_status ON worktrees(status);
	CREATE INDEX IF NOT EXISTS idx_worktrees_created ON worktrees(created_at);
	CREATE INDEX IF NOT EXISTS idx_guidance_task ON guidance_queue(task_id);
	CREATE INDEX IF NOT EXISTS idx_guidance_delivered ON guidance_queue(delivered);
	CREATE INDEX IF NOT EXISTS idx_operators_name ON operators(name);
	CREATE INDEX IF NOT EXISTS idx_operators_api_key ON operators(api_key);
	`

	_, err := s.DB.Exec(schema)
	return err
}

// MigrateSchema runs database migrations for existing databases
// This adds new columns that weren't in the original schema
func (s *Store) MigrateSchema() error {
	// Check if parent_id column exists
	var parentIDExists bool
	err := s.DB.QueryRow(`
		SELECT COUNT(*) > 0 FROM pragma_table_info('tasks') WHERE name = 'parent_id'
	`).Scan(&parentIDExists)
	if err != nil {
		return fmt.Errorf("checking for parent_id column: %w", err)
	}

	if !parentIDExists {
		// Add parent_id and sequence_number columns for sub-task hierarchy
		_, err := s.DB.Exec(`
			ALTER TABLE tasks ADD COLUMN parent_id TEXT REFERENCES tasks(id) ON DELETE CASCADE;
			ALTER TABLE tasks ADD COLUMN sequence_number INTEGER DEFAULT 0;
		`)
		if err != nil {
			return fmt.Errorf("adding hierarchy columns: %w", err)
		}

		// Create indexes for the new columns
		_, err = s.DB.Exec(`
			CREATE INDEX IF NOT EXISTS idx_tasks_parent ON tasks(parent_id);
			CREATE INDEX IF NOT EXISTS idx_tasks_parent_seq ON tasks(parent_id, sequence_number);
		`)
		if err != nil {
			return fmt.Errorf("creating hierarchy indexes: %w", err)
		}
	}

	// Check if worktrees table exists (added in a later version)
	var worktreesTableExists bool
	err = s.DB.QueryRow(`
		SELECT COUNT(*) > 0 FROM sqlite_master WHERE type='table' AND name='worktrees'
	`).Scan(&worktreesTableExists)
	if err != nil {
		return fmt.Errorf("checking for worktrees table: %w", err)
	}

	if !worktreesTableExists {
		// Create the worktrees table
		_, err := s.DB.Exec(`
			CREATE TABLE worktrees (
				task_id TEXT PRIMARY KEY,
				path TEXT NOT NULL,
				branch TEXT NOT NULL,
				created_at INTEGER NOT NULL,
				last_used_at INTEGER NOT NULL,
				status TEXT DEFAULT 'active',
				disk_size INTEGER DEFAULT 0,
				FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
			);
			CREATE INDEX IF NOT EXISTS idx_worktrees_status ON worktrees(status);
			CREATE INDEX IF NOT EXISTS idx_worktrees_created ON worktrees(created_at);
		`)
		if err != nil {
			return fmt.Errorf("creating worktrees table: %w", err)
		}
	}

	// Check if operator column exists (added for multiplayer collaboration)
	var operatorExists bool
	err = s.DB.QueryRow(`
		SELECT COUNT(*) > 0 FROM pragma_table_info('tasks') WHERE name = 'operator'
	`).Scan(&operatorExists)
	if err != nil {
		return fmt.Errorf("checking for operator column: %w", err)
	}

	if !operatorExists {
		// Add operator column for tracking who created/owns a task
		_, err := s.DB.Exec(`
			ALTER TABLE tasks ADD COLUMN operator TEXT DEFAULT '';
		`)
		if err != nil {
			return fmt.Errorf("adding operator column: %w", err)
		}
	}

	// Check if type column exists (added for task categorization)
	var typeExists bool
	err = s.DB.QueryRow(`
		SELECT COUNT(*) > 0 FROM pragma_table_info('tasks') WHERE name = 'type'
	`).Scan(&typeExists)
	if err != nil {
		return fmt.Errorf("checking for type column: %w", err)
	}

	if !typeExists {
		// Add type column for categorizing tasks by type
		_, err := s.DB.Exec(`
			ALTER TABLE tasks ADD COLUMN type TEXT DEFAULT 'other';
		`)
		if err != nil {
			return fmt.Errorf("adding type column: %w", err)
		}
		// Create index for type-based queries
		_, err = s.DB.Exec(`
			CREATE INDEX IF NOT EXISTS idx_tasks_type ON tasks(type);
		`)
		if err != nil {
			return fmt.Errorf("creating type index: %w", err)
		}
	}

	// Check if guidance_queue table exists (added for human-in-the-loop intervention)
	var guidanceTableExists bool
	err = s.DB.QueryRow(`
		SELECT COUNT(*) > 0 FROM sqlite_master WHERE type='table' AND name='guidance_queue'
	`).Scan(&guidanceTableExists)
	if err != nil {
		return fmt.Errorf("checking for guidance_queue table: %w", err)
	}

	if !guidanceTableExists {
		// Create the guidance_queue table
		_, err := s.DB.Exec(`
			CREATE TABLE guidance_queue (
				id TEXT PRIMARY KEY,
				task_id TEXT NOT NULL,
				message TEXT NOT NULL,
				created_at INTEGER NOT NULL,
				delivered INTEGER DEFAULT 0,
				FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
			);
			CREATE INDEX IF NOT EXISTS idx_guidance_task ON guidance_queue(task_id);
			CREATE INDEX IF NOT EXISTS idx_guidance_delivered ON guidance_queue(delivered);
		`)
		if err != nil {
			return fmt.Errorf("creating guidance_queue table: %w", err)
		}
	}

	// Check if session_shares table exists (added for multiplayer session handoff)
	var sessionSharesTableExists bool
	err = s.DB.QueryRow(`
		SELECT COUNT(*) > 0 FROM sqlite_master WHERE type='table' AND name='session_shares'
	`).Scan(&sessionSharesTableExists)
	if err != nil {
		return fmt.Errorf("checking for session_shares table: %w", err)
	}

	if !sessionSharesTableExists {
		// Create the session_shares table for shareable session links
		_, err := s.DB.Exec(`
			CREATE TABLE session_shares (
				id TEXT PRIMARY KEY,
				token TEXT UNIQUE NOT NULL,
				session_data TEXT NOT NULL,
				created_by TEXT NOT NULL,
				created_at INTEGER NOT NULL,
				expires_at INTEGER,
				access_count INTEGER DEFAULT 0
			);
			CREATE INDEX IF NOT EXISTS idx_session_shares_token ON session_shares(token);
			CREATE INDEX IF NOT EXISTS idx_session_shares_expires ON session_shares(expires_at);
		`)
		if err != nil {
			return fmt.Errorf("creating session_shares table: %w", err)
		}
	}

	// Check if operators table exists (added for multiplayer authentication)
	var operatorsTableExists bool
	err = s.DB.QueryRow(`
		SELECT COUNT(*) > 0 FROM sqlite_master WHERE type='table' AND name='operators'
	`).Scan(&operatorsTableExists)
	if err != nil {
		return fmt.Errorf("checking for operators table: %w", err)
	}

	if !operatorsTableExists {
		// Create the operators table for authentication
		_, err := s.DB.Exec(`
			CREATE TABLE operators (
				id TEXT PRIMARY KEY,
				name TEXT UNIQUE NOT NULL,
				api_key TEXT UNIQUE NOT NULL,
				created_at INTEGER NOT NULL,
				last_active INTEGER
			);
			CREATE INDEX IF NOT EXISTS idx_operators_name ON operators(name);
			CREATE INDEX IF NOT EXISTS idx_operators_api_key ON operators(api_key);
		`)
		if err != nil {
			return fmt.Errorf("creating operators table: %w", err)
		}
	}

	// Check if plans table exists (added for planning/building separation)
	var plansTableExists bool
	err = s.DB.QueryRow(`
		SELECT COUNT(*) > 0 FROM sqlite_master WHERE type='table' AND name='plans'
	`).Scan(&plansTableExists)
	if err != nil {
		return fmt.Errorf("checking for plans table: %w", err)
	}

	if !plansTableExists {
		// Create the plans table for planning/building separation
		_, err := s.DB.Exec(`
			CREATE TABLE plans (
				id TEXT PRIMARY KEY,
				task_id TEXT NOT NULL,
				title TEXT NOT NULL,
				description TEXT,
				steps TEXT NOT NULL,
				files_to_create TEXT,
				files_to_modify TEXT,
				dependencies TEXT,
				estimated_time INTEGER DEFAULT 0,
				complexity TEXT DEFAULT 'medium',
				risk_factors TEXT,
				status TEXT DEFAULT 'draft',
				approved_by TEXT,
				approved_at INTEGER,
				rejection_reason TEXT,
				revision INTEGER DEFAULT 1,
				parent_plan_id TEXT,
				feedback TEXT,
				created_at INTEGER NOT NULL,
				updated_at INTEGER NOT NULL,
				created_by TEXT,
				FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
			);
			CREATE INDEX IF NOT EXISTS idx_plans_task_id ON plans(task_id);
			CREATE INDEX IF NOT EXISTS idx_plans_status ON plans(status);
			CREATE INDEX IF NOT EXISTS idx_plans_parent ON plans(parent_plan_id);
		`)
		if err != nil {
			return fmt.Errorf("creating plans table: %w", err)
		}
	}

	return nil
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
	return s.CreateTaskWithOperator(title, description, epicID, priority, blockedBy, "")
}

// CreateTaskWithOperator creates a new task with an operator (user/creator)
func (s *Store) CreateTaskWithOperator(title, description, epicID string, priority int, blockedBy []string, operator string) (*types.Task, error) {
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
		Operator:    operator,
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
		INSERT INTO tasks (id, title, description, epic_id, type, priority, status, operator, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, task.ID, task.Title, task.Description, epicIDValue, task.Type, task.Priority, task.Status, task.Operator, task.CreatedAt, task.UpdatedAt)
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

// CreateSubTask creates a new sub-task with a hierarchical ID
func (s *Store) CreateSubTask(title, description, parentID string, priority int, blockedBy []string) (*types.Task, error) {
	// Verify parent exists and is not itself a sub-task (max 2 levels)
	parent, err := s.GetTask(parentID)
	if err != nil {
		return nil, fmt.Errorf("parent task not found: %w", err)
	}
	if parent.ParentID != "" {
		return nil, fmt.Errorf("parent task is already a sub-task (max depth is 2 levels)")
	}

	// Get the next sequence number for this parent
	var nextSeq int
	err = s.DB.QueryRow(`
		SELECT COALESCE(MAX(sequence_number), 0) + 1
		FROM tasks
		WHERE parent_id = ?
	`, parentID).Scan(&nextSeq)
	if err != nil {
		return nil, fmt.Errorf("getting next sequence number: %w", err)
	}

	// Generate hierarchical ID: parentID.sequence
	id := fmt.Sprintf("%s.%d", parentID, nextSeq)
	now := time.Now().Unix()

	// Inherit operator from parent task
	task := &types.Task{
		ID:             id,
		Title:          title,
		Description:    description,
		EpicID:         parent.EpicID,
		ParentID:       parentID,
		SequenceNumber: nextSeq,
		Priority:       priority,
		Status:         types.TaskStatusReady,
		MaxAttempts:    3,
		Operator:       parent.Operator, // Inherit from parent
		CreatedAt:      now,
		UpdatedAt:      now,
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

	// Convert empty epic_id to NULL for foreign key constraint
	var epicIDValue interface{} = task.EpicID
	if epicIDValue == "" {
		epicIDValue = nil
	}

	// Insert task
	_, err = tx.Exec(`
		INSERT INTO tasks (id, title, description, epic_id, parent_id, sequence_number,
		                  type, priority, status, operator, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, task.ID, task.Title, task.Description, epicIDValue, task.ParentID, task.SequenceNumber,
		task.Type, task.Priority, task.Status, task.Operator, task.CreatedAt, task.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("creating sub-task: %w", err)
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

// CreateSubTaskWithSequence creates a new sub-task with a specific hierarchical ID
// This is used when the user specifies a sequence number via CLI syntax (e.g., task-123.5)
func (s *Store) CreateSubTaskWithSequence(title, description, parentID string, sequence int, priority int, blockedBy []string) (*types.Task, error) {
	// Verify parent exists and is not itself a sub-task (max 2 levels)
	parent, err := s.GetTask(parentID)
	if err != nil {
		return nil, fmt.Errorf("parent task not found: %w", err)
	}
	if parent.ParentID != "" {
		return nil, fmt.Errorf("parent task is already a sub-task (max depth is 2 levels)")
	}

	// Check if the specified sequence number is already taken
	var existingCount int
	err = s.DB.QueryRow(`
		SELECT COUNT(*) FROM tasks WHERE parent_id = ? AND sequence_number = ?
	`, parentID, sequence).Scan(&existingCount)
	if err != nil {
		return nil, fmt.Errorf("checking existing sequence: %w", err)
	}
	if existingCount > 0 {
		return nil, fmt.Errorf("sequence number %d already exists for parent %s", sequence, parentID)
	}

	// Generate hierarchical ID: parentID.sequence
	id := fmt.Sprintf("%s.%d", parentID, sequence)
	now := time.Now().Unix()

	// Inherit operator from parent task
	task := &types.Task{
		ID:             id,
		Title:          title,
		Description:    description,
		EpicID:         parent.EpicID,
		ParentID:       parentID,
		SequenceNumber: sequence,
		Priority:       priority,
		Status:         types.TaskStatusReady,
		MaxAttempts:    3,
		Operator:       parent.Operator, // Inherit from parent
		CreatedAt:      now,
		UpdatedAt:      now,
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

	// Convert empty epic_id to NULL for foreign key constraint
	var epicIDValue interface{} = task.EpicID
	if epicIDValue == "" {
		epicIDValue = nil
	}

	// Insert task
	_, err = tx.Exec(`
		INSERT INTO tasks (id, title, description, epic_id, parent_id, sequence_number,
		                  type, priority, status, operator, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, task.ID, task.Title, task.Description, epicIDValue, task.ParentID, task.SequenceNumber,
		task.Type, task.Priority, task.Status, task.Operator, task.CreatedAt, task.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("creating sub-task: %w", err)
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
		case types.TaskStatusPaused:
			status.Paused = count
		case types.TaskStatusBlocked:
			status.Blocked = count
		case types.TaskStatusCompleted:
			status.Completed = count
		case types.TaskStatusFailed:
			status.Failed = count
		}
	}

	status.Total = status.Ready + status.Claimed + status.InProgress +
		status.Paused + status.Blocked + status.Completed + status.Failed

	return status, nil
}

// ClaimTask attempts to atomically claim a ready task
//
// Uses UPDATE with ORDER BY and LIMIT to atomically find and claim a task
// in a single operation, avoiding race conditions between SELECT and UPDATE.
func (s *Store) ClaimTask(workerID string) (*types.Task, error) {
	return s.ClaimTaskForEpic(workerID, "")
}

// ClaimTaskForEpic attempts to atomically claim a ready task, optionally filtered by epic
//
// Uses UPDATE with ORDER BY and LIMIT to atomically find and claim a task
// in a single operation, avoiding race conditions between SELECT and UPDATE.
// If epicID is empty, claims any ready task. If epicID is set, only claims tasks in that epic.
func (s *Store) ClaimTaskForEpic(workerID, epicID string) (*types.Task, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	now := time.Now().Unix()

	// Build the query with optional epic filtering
	var task types.Task
	if epicID != "" {
		// Filter by epic_id and exclude sub-tasks (they run via parent)
		err = tx.QueryRow(`
			UPDATE tasks
			SET status = 'claimed',
			    claimed_by = ?,
			    claimed_at = ?,
			    updated_at = ?
			WHERE id = (
				SELECT id FROM tasks
				WHERE status = 'ready' AND epic_id = ? AND parent_id IS NULL
				ORDER BY priority DESC, created_at ASC
				LIMIT 1
			)
			RETURNING id, title, COALESCE(description, ''), COALESCE(epic_id, ''),
			          COALESCE(parent_id, ''), sequence_number,
			          COALESCE(type, 'other'),
			          priority, status, attempts, max_attempts,
			          COALESCE(operator, ''), created_at, updated_at
		`, workerID, now, now, epicID).Scan(&task.ID, &task.Title, &task.Description, &task.EpicID,
			&task.ParentID, &task.SequenceNumber,
			&task.Type,
			&task.Priority, &task.Status, &task.Attempts, &task.MaxAttempts,
			&task.Operator, &task.CreatedAt, &task.UpdatedAt)
	} else {
		// No epic filtering, exclude sub-tasks (they run via parent)
		err = tx.QueryRow(`
			UPDATE tasks
			SET status = 'claimed',
			    claimed_by = ?,
			    claimed_at = ?,
			    updated_at = ?
			WHERE id = (
				SELECT id FROM tasks
				WHERE status = 'ready' AND parent_id IS NULL
				ORDER BY priority DESC, created_at ASC
				LIMIT 1
			)
			RETURNING id, title, COALESCE(description, ''), COALESCE(epic_id, ''),
			          COALESCE(parent_id, ''), sequence_number,
			          priority, status, attempts, max_attempts,
			          COALESCE(operator, ''), created_at, updated_at
		`, workerID, now, now).Scan(&task.ID, &task.Title, &task.Description, &task.EpicID,
			&task.ParentID, &task.SequenceNumber,
			&task.Priority, &task.Status, &task.Attempts, &task.MaxAttempts,
			&task.Operator, &task.CreatedAt, &task.UpdatedAt)
	}

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
	var operator sql.NullString

	err := s.DB.QueryRow(`
		SELECT id, title, COALESCE(description, ''), COALESCE(epic_id, ''),
		       COALESCE(parent_id, ''), sequence_number,
		       COALESCE(type, 'other'),
		       priority, status, attempts, max_attempts,
		       COALESCE(claimed_by, ''), COALESCE(claimed_at, 0),
		       COALESCE(operator, ''), created_at, updated_at
		FROM tasks
		WHERE id = ?
	`, taskID).Scan(
		&task.ID, &task.Title, &description, &epicID,
		&task.ParentID, &task.SequenceNumber,
		&task.Type,
		&task.Priority, &task.Status, &task.Attempts, &task.MaxAttempts,
		&claimedBy, &claimedAt, &operator,
		&task.CreatedAt, &task.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	task.Description = description.String
	task.EpicID = epicID.String
	task.Operator = operator.String
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

// ResetTasksByIDs resets specific tasks by their IDs back to ready status
func (s *Store) ResetTasksByIDs(taskIDs []string) (int, error) {
	if len(taskIDs) == 0 {
		return 0, nil
	}

	now := time.Now().Unix()

	// Build placeholder list for SQL IN clause
	placeholders := make([]string, len(taskIDs))
	args := make([]interface{}, len(taskIDs)+1)
	// Put timestamp first since it's for updated_at = ?
	args[0] = now
	for i, id := range taskIDs {
		placeholders[i] = "?"
		args[i+1] = id
	}

	// Reset tasks to ready status
	query := fmt.Sprintf(`
		UPDATE tasks
		SET status = 'ready', claimed_by = NULL, claimed_at = NULL,
		    attempts = 0, last_error = NULL, updated_at = ?
		WHERE id IN (%s)
	`, strings.Join(placeholders, ", "))

	result, err := s.DB.Exec(query, args...)
	if err != nil {
		return 0, fmt.Errorf("resetting tasks by IDs: %w", err)
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
	return s.ListTasksByEpic("")
}

// ListTasksByEpic returns tasks filtered by epic ID
// If epicID is empty, returns all tasks
func (s *Store) ListTasksByEpic(epicID string) ([]*types.Task, error) {
	var rows *sql.Rows
	var err error

	if epicID != "" {
		// Filter by epic ID
		rows, err = s.DB.Query(`
			SELECT id, title, COALESCE(description, ''), COALESCE(epic_id, ''),
			       COALESCE(parent_id, ''), sequence_number,
			       COALESCE(type, 'other'),
			       priority, status, attempts, max_attempts,
			       COALESCE(claimed_by, ''), COALESCE(claimed_at, 0),
			       COALESCE(operator, ''), created_at, updated_at
			FROM tasks
			WHERE epic_id = ?
			ORDER BY created_at ASC
		`, epicID)
	} else {
		// Return all tasks
		rows, err = s.DB.Query(`
			SELECT id, title, COALESCE(description, ''), COALESCE(epic_id, ''),
			       COALESCE(parent_id, ''), sequence_number,
			       COALESCE(type, 'other'),
			       priority, status, attempts, max_attempts,
			       COALESCE(claimed_by, ''), COALESCE(claimed_at, 0),
			       COALESCE(operator, ''), created_at, updated_at
			FROM tasks
			ORDER BY created_at ASC
		`)
	}

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
		var parentID sql.NullString
		var operator sql.NullString

		err := rows.Scan(
			&task.ID, &task.Title, &description, &epicID,
			&parentID, &task.SequenceNumber,
			&task.Type,
			&task.Priority, &task.Status, &task.Attempts, &task.MaxAttempts,
			&claimedBy, &claimedAt, &operator,
			&task.CreatedAt, &task.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning task: %w", err)
		}

		task.Description = description.String
		task.EpicID = epicID.String
		task.ParentID = parentID.String
		task.Operator = operator.String
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
	TaskID     string
	Path       string
	Branch     string
	CreatedAt  int64
	LastUsedAt int64
	Status     string
	DiskSize   int64
	TaskStatus string
	TaskTitle  string
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

// GetSubTasks retrieves all direct sub-tasks of a parent task
func (s *Store) GetSubTasks(parentID string) ([]*types.Task, error) {
	rows, err := s.DB.Query(`
		SELECT id, title, COALESCE(description, ''), COALESCE(epic_id, ''),
		       COALESCE(parent_id, ''), sequence_number,
		       COALESCE(type, 'other'),
		       priority, status, attempts, max_attempts,
		       COALESCE(claimed_by, ''), COALESCE(claimed_at, 0),
		       COALESCE(operator, ''), created_at, updated_at
		FROM tasks
		WHERE parent_id = ?
		ORDER BY sequence_number ASC
	`, parentID)
	if err != nil {
		return nil, fmt.Errorf("querying sub-tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*types.Task
	for rows.Next() {
		var task types.Task
		var claimedBy sql.NullString
		var claimedAt sql.NullInt64
		var epicID sql.NullString
		var description sql.NullString
		var parentID sql.NullString
		var operator sql.NullString

		err := rows.Scan(
			&task.ID, &task.Title, &description, &epicID,
			&parentID, &task.SequenceNumber,
			&task.Type,
			&task.Priority, &task.Status, &task.Attempts, &task.MaxAttempts,
			&claimedBy, &claimedAt, &operator,
			&task.CreatedAt, &task.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning sub-task: %w", err)
		}

		task.Description = description.String
		task.EpicID = epicID.String
		task.ParentID = parentID.String
		task.Operator = operator.String
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

// GetTaskTree retrieves a task with all its descendants (sub-tasks recursively)
func (s *Store) GetTaskTree(taskID string) (*types.Task, error) {
	task, err := s.GetTask(taskID)
	if err != nil {
		return nil, err
	}

	// Note: Use GetSubTasks() to get the children separately if needed
	// This method returns the parent task which can be used to look up children
	// via GetSubTasks(taskID)

	return task, nil
}

// GetParentTask retrieves the parent task of a sub-task
func (s *Store) GetParentTask(taskID string) (*types.Task, error) {
	var parentID string
	err := s.DB.QueryRow(`
		SELECT COALESCE(parent_id, '') FROM tasks WHERE id = ?
	`, taskID).Scan(&parentID)
	if err != nil {
		return nil, fmt.Errorf("getting parent ID: %w", err)
	}

	if parentID == "" {
		return nil, fmt.Errorf("task has no parent")
	}

	return s.GetTask(parentID)
}

// HasSubTasks returns true if a task has any sub-tasks
func (s *Store) HasSubTasks(taskID string) (bool, error) {
	var count int
	err := s.DB.QueryRow(`
		SELECT COUNT(*) FROM tasks WHERE parent_id = ?
	`, taskID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("checking for sub-tasks: %w", err)
	}
	return count > 0, nil
}

// AddGuidance adds a guidance message to a task's queue
func (s *Store) AddGuidance(taskID, message string) (*types.GuidanceMessage, error) {
	id := generateID("guidance")
	now := time.Now().Unix()

	guidance := &types.GuidanceMessage{
		ID:        id,
		TaskID:    taskID,
		Message:   message,
		CreatedAt: now,
		Delivered: false,
	}

	_, err := s.DB.Exec(`
		INSERT INTO guidance_queue (id, task_id, message, created_at, delivered)
		VALUES (?, ?, ?, ?, 0)
	`, guidance.ID, guidance.TaskID, guidance.Message, guidance.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("adding guidance: %w", err)
	}

	return guidance, nil
}

// GetPendingGuidance retrieves undelivered guidance messages for a task
func (s *Store) GetPendingGuidance(taskID string) ([]*types.GuidanceMessage, error) {
	rows, err := s.DB.Query(`
		SELECT id, task_id, message, created_at, delivered
		FROM guidance_queue
		WHERE task_id = ? AND delivered = 0
		ORDER BY created_at ASC
	`, taskID)
	if err != nil {
		return nil, fmt.Errorf("querying guidance: %w", err)
	}
	defer rows.Close()

	var messages []*types.GuidanceMessage
	for rows.Next() {
		var g types.GuidanceMessage
		var delivered int
		err := rows.Scan(&g.ID, &g.TaskID, &g.Message, &g.CreatedAt, &delivered)
		if err != nil {
			return nil, fmt.Errorf("scanning guidance: %w", err)
		}
		g.Delivered = delivered != 0
		messages = append(messages, &g)
	}

	return messages, nil
}

// MarkGuidanceDelivered marks guidance messages as delivered
func (s *Store) MarkGuidanceDelivered(guidanceIDs []string) error {
	if len(guidanceIDs) == 0 {
		return nil
	}

	placeholders := make([]string, len(guidanceIDs))
	args := make([]interface{}, len(guidanceIDs))
	for i, id := range guidanceIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		UPDATE guidance_queue
		SET delivered = 1
		WHERE id IN (%s)
	`, strings.Join(placeholders, ", "))

	_, err := s.DB.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("marking guidance delivered: %w", err)
	}

	return nil
}

// ClearGuidance removes all guidance messages for a task
func (s *Store) ClearGuidance(taskID string) error {
	_, err := s.DB.Exec(`
		DELETE FROM guidance_queue WHERE task_id = ?
	`, taskID)
	if err != nil {
		return fmt.Errorf("clearing guidance: %w", err)
	}
	return nil
}

// PauseTask pauses a running task, preserving its state
func (s *Store) PauseTask(taskID string) error {
	now := time.Now().Unix()

	// Check if task is in_progress or claimed (can only pause active tasks)
	var status string
	err := s.DB.QueryRow(`SELECT status FROM tasks WHERE id = ?`, taskID).Scan(&status)
	if err != nil {
		return fmt.Errorf("getting task status: %w", err)
	}

	if status != string(types.TaskStatusInProgress) && status != string(types.TaskStatusClaimed) {
		return fmt.Errorf("cannot pause task with status %s (only in_progress or claimed tasks can be paused)", status)
	}

	// Update status to paused
	_, err = s.DB.Exec(`
		UPDATE tasks
		SET status = 'paused', updated_at = ?
		WHERE id = ?
	`, now, taskID)
	if err != nil {
		return fmt.Errorf("pausing task: %w", err)
	}

	return nil
}

// ResumeTask resumes a paused task
func (s *Store) ResumeTask(taskID string) error {
	now := time.Now().Unix()

	// Check if task is paused
	var status string
	err := s.DB.QueryRow(`SELECT status FROM tasks WHERE id = ?`, taskID).Scan(&status)
	if err != nil {
		return fmt.Errorf("getting task status: %w", err)
	}

	if status != string(types.TaskStatusPaused) {
		return fmt.Errorf("cannot resume task with status %s (only paused tasks can be resumed)", status)
	}

	// Reset status to ready so it can be claimed again
	_, err = s.DB.Exec(`
		UPDATE tasks
		SET status = 'ready', claimed_by = NULL, claimed_at = NULL, updated_at = ?
		WHERE id = ?
	`, now, taskID)
	if err != nil {
		return fmt.Errorf("resuming task: %w", err)
	}

	return nil
}

// SessionExport represents a complete exported session
type SessionExport struct {
	Version      string                 `json:"version"`
	ExportedAt   string                 `json:"exportedAt"`
	Repository   string                 `json:"repository"`
	Tasks        []*types.Task          `json:"tasks"`
	Epics        []*types.Epic          `json:"epics"`
	Dependencies []types.TaskDependency `json:"dependencies"`
	Worktrees    []*WorktreeInfo        `json:"worktrees"`
}

// ImportSession imports a session from an export
func (s *Store) ImportSession(session *SessionExport) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	// Import epics
	for _, epic := range session.Epics {
		// Check if epic already exists
		var exists int
		err := tx.QueryRow(`SELECT COUNT(*) FROM epics WHERE id = ?`, epic.ID).Scan(&exists)
		if err != nil {
			return fmt.Errorf("checking epic existence: %w", err)
		}
		if exists == 0 {
			_, err = tx.Exec(`
				INSERT INTO epics (id, title, description, status, created_at)
				VALUES (?, ?, ?, ?, ?)
			`, epic.ID, epic.Title, epic.Description, epic.Status, epic.CreatedAt)
			if err != nil {
				return fmt.Errorf("importing epic: %w", err)
			}
		}
	}

	// Import tasks
	for _, task := range session.Tasks {
		// Check if task already exists
		var exists int
		err := tx.QueryRow(`SELECT COUNT(*) FROM tasks WHERE id = ?`, task.ID).Scan(&exists)
		if err != nil {
			return fmt.Errorf("checking task existence: %w", err)
		}
		if exists == 0 {
			// Convert empty epic_id to NULL for foreign key constraint
			var epicIDValue interface{} = task.EpicID
			if epicIDValue == "" {
				epicIDValue = nil
			}
			// Convert empty parent_id to NULL
			var parentIDValue interface{} = task.ParentID
			if parentIDValue == "" {
				parentIDValue = nil
			}

			_, err = tx.Exec(`
				INSERT INTO tasks (id, title, description, epic_id, parent_id, sequence_number,
				                  type, priority, status, attempts, max_attempts, last_error,
				                  claimed_by, claimed_at, operator, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`, task.ID, task.Title, task.Description, epicIDValue, parentIDValue, task.SequenceNumber,
				task.Type, task.Priority, task.Status, task.Attempts, task.MaxAttempts, task.LastError,
				task.ClaimedBy, task.ClaimedAt, task.Operator, task.CreatedAt, task.UpdatedAt)
			if err != nil {
				return fmt.Errorf("importing task: %w", err)
			}
		}
	}

	// Import dependencies
	for _, dep := range session.Dependencies {
		// Check if dependency already exists
		var exists int
		err := tx.QueryRow(`
			SELECT COUNT(*) FROM task_dependencies WHERE task_id = ? AND blocked_by = ?
		`, dep.TaskID, dep.BlockedBy).Scan(&exists)
		if err != nil {
			return fmt.Errorf("checking dependency existence: %w", err)
		}
		if exists == 0 {
			_, err = tx.Exec(`
				INSERT INTO task_dependencies (task_id, blocked_by)
				VALUES (?, ?)
			`, dep.TaskID, dep.BlockedBy)
			if err != nil {
				return fmt.Errorf("importing dependency: %w", err)
			}
		}
	}

	// Note: We don't import worktrees as they are specific to the original machine
	// The worktrees will be created as needed when tasks are executed

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing import: %w", err)
	}

	return nil
}

// SessionShare represents a shareable session link
type SessionShare struct {
	ID          string
	Token       string
	SessionData string
	CreatedBy   string
	CreatedAt   int64
	ExpiresAt   *int64
	AccessCount int
}

// CreateSessionShare creates a new shareable session link
func (s *Store) CreateSessionShare(sessionJSON, createdBy string, expiresHours int) (*SessionShare, error) {
	id := generateID("share")
	token := generateToken()

	now := time.Now().Unix()
	var expiresAt *int64
	if expiresHours > 0 {
		expiry := now + int64(expiresHours*3600)
		expiresAt = &expiry
	}

	share := &SessionShare{
		ID:          id,
		Token:       token,
		SessionData: sessionJSON,
		CreatedBy:   createdBy,
		CreatedAt:   now,
		ExpiresAt:   expiresAt,
		AccessCount: 0,
	}

	var expiresAtValue interface{} = nil
	if expiresAt != nil {
		expiresAtValue = *expiresAt
	}

	_, err := s.DB.Exec(`
		INSERT INTO session_shares (id, token, session_data, created_by, created_at, expires_at, access_count)
		VALUES (?, ?, ?, ?, ?, ?, 0)
	`, share.ID, share.Token, share.SessionData, share.CreatedBy, share.CreatedAt, expiresAtValue)
	if err != nil {
		return nil, fmt.Errorf("creating session share: %w", err)
	}

	return share, nil
}

// GetSessionShareByToken retrieves a session share by its token
func (s *Store) GetSessionShareByToken(token string) (*SessionShare, error) {
	var share SessionShare
	var expiresAt sql.NullInt64

	err := s.DB.QueryRow(`
		SELECT id, token, session_data, created_by, created_at, expires_at, access_count
		FROM session_shares
		WHERE token = ?
	`, token).Scan(&share.ID, &share.Token, &share.SessionData, &share.CreatedBy,
		&share.CreatedAt, &expiresAt, &share.AccessCount)

	if err != nil {
		return nil, err
	}

	if expiresAt.Valid {
		share.ExpiresAt = &expiresAt.Int64
	}

	// Check if expired
	if share.ExpiresAt != nil && *share.ExpiresAt < time.Now().Unix() {
		return nil, fmt.Errorf("session share has expired")
	}

	return &share, nil
}

// IncrementShareAccess increments the access count for a session share
func (s *Store) IncrementShareAccess(token string) error {
	_, err := s.DB.Exec(`
		UPDATE session_shares
		SET access_count = access_count + 1
		WHERE token = ?
	`, token)
	if err != nil {
		return fmt.Errorf("incrementing share access: %w", err)
	}
	return nil
}

// DeleteSessionShare deletes a session share by token
func (s *Store) DeleteSessionShare(token string) error {
	_, err := s.DB.Exec(`
		DELETE FROM session_shares WHERE token = ?
	`, token)
	if err != nil {
		return fmt.Errorf("deleting session share: %w", err)
	}
	return nil
}

// generateToken generates a random token for sharing
func generateToken() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 16)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}

// Operator represents a user/operator in the system
type Operator struct {
	ID         string
	Name       string
	APIKey     string
	CreatedAt  int64
	LastActive *int64
}

// CreateOperator creates a new operator with an API key
func (s *Store) CreateOperator(name string) (*Operator, error) {
	id := generateID("op")
	apiKey := generateAPIKey()
	now := time.Now().Unix()

	op := &Operator{
		ID:        id,
		Name:      name,
		APIKey:    apiKey,
		CreatedAt: now,
	}

	_, err := s.DB.Exec(`
		INSERT INTO operators (id, name, api_key, created_at)
		VALUES (?, ?, ?, ?)
	`, op.ID, op.Name, op.APIKey, op.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("creating operator: %w", err)
	}

	return op, nil
}

// GetOperatorByName retrieves an operator by name
func (s *Store) GetOperatorByName(name string) (*Operator, error) {
	var op Operator
	var lastActive sql.NullInt64

	err := s.DB.QueryRow(`
		SELECT id, name, api_key, created_at, last_active
		FROM operators
		WHERE name = ?
	`, name).Scan(&op.ID, &op.Name, &op.APIKey, &op.CreatedAt, &lastActive)

	if err != nil {
		return nil, err
	}

	if lastActive.Valid {
		op.LastActive = &lastActive.Int64
	}

	return &op, nil
}

// GetOperatorByAPIKey retrieves an operator by API key
func (s *Store) GetOperatorByAPIKey(apiKey string) (*Operator, error) {
	var op Operator
	var lastActive sql.NullInt64

	err := s.DB.QueryRow(`
		SELECT id, name, api_key, created_at, last_active
		FROM operators
		WHERE api_key = ?
	`, apiKey).Scan(&op.ID, &op.Name, &op.APIKey, &op.CreatedAt, &lastActive)

	if err != nil {
		return nil, err
	}

	if lastActive.Valid {
		op.LastActive = &lastActive.Int64
	}

	return &op, nil
}

// UpdateOperatorLastActive updates the last active timestamp
func (s *Store) UpdateOperatorLastActive(name string) error {
	now := time.Now().Unix()
	_, err := s.DB.Exec(`
		UPDATE operators
		SET last_active = ?
		WHERE name = ?
	`, now, name)
	if err != nil {
		return fmt.Errorf("updating operator last active: %w", err)
	}
	return nil
}

// ListOperators returns all operators
func (s *Store) ListOperators() ([]*Operator, error) {
	rows, err := s.DB.Query(`
		SELECT id, name, api_key, created_at, last_active
		FROM operators
		ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("querying operators: %w", err)
	}
	defer rows.Close()

	var operators []*Operator
	for rows.Next() {
		var op Operator
		var lastActive sql.NullInt64

		err := rows.Scan(&op.ID, &op.Name, &op.APIKey, &op.CreatedAt, &lastActive)
		if err != nil {
			return nil, fmt.Errorf("scanning operator: %w", err)
		}

		if lastActive.Valid {
			op.LastActive = &lastActive.Int64
		}

		operators = append(operators, &op)
	}

	return operators, nil
}

// DeleteOperator deletes an operator by name
func (s *Store) DeleteOperator(name string) error {
	_, err := s.DB.Exec(`
		DELETE FROM operators WHERE name = ?
	`, name)
	if err != nil {
		return fmt.Errorf("deleting operator: %w", err)
	}
	return nil
}

// SavePlan saves a plan to the database
func (s *Store) SavePlan(plan *Plan) error {
	stepsJSON, err := jsonMarshal(plan.Steps)
	if err != nil {
		return fmt.Errorf("marshaling steps: %w", err)
	}

	filesToCreateJSON, err := jsonMarshal(plan.FilesToCreate)
	if err != nil {
		return fmt.Errorf("marshaling files_to_create: %w", err)
	}

	filesToModifyJSON, err := jsonMarshal(plan.FilesToModify)
	if err != nil {
		return fmt.Errorf("marshaling files_to_modify: %w", err)
	}

	dependenciesJSON, err := jsonMarshal(plan.Dependencies)
	if err != nil {
		return fmt.Errorf("marshaling dependencies: %w", err)
	}

	riskFactorsJSON, err := jsonMarshal(plan.RiskFactors)
	if err != nil {
		return fmt.Errorf("marshaling risk_factors: %w", err)
	}

	feedbackJSON, err := jsonMarshal(plan.Feedback)
	if err != nil {
		return fmt.Errorf("marshaling feedback: %w", err)
	}

	var approvedAt int64
	if plan.ApprovedAt != nil {
		approvedAt = plan.ApprovedAt.Unix()
	}

	_, err = s.DB.Exec(`
		INSERT OR REPLACE INTO plans (
			id, task_id, title, description, steps, files_to_create, files_to_modify,
			dependencies, estimated_time, complexity, risk_factors, status,
			approved_by, approved_at, rejection_reason, revision, parent_plan_id,
			feedback, created_at, updated_at, created_by
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, plan.ID, plan.TaskID, plan.Title, plan.Description, stepsJSON,
		filesToCreateJSON, filesToModifyJSON, dependenciesJSON,
		int64(plan.EstimatedTime.Seconds()), plan.Complexity, riskFactorsJSON,
		string(plan.Status), plan.ApprovedBy, approvedAt, plan.RejectionReason,
		plan.Revision, plan.ParentPlanID, feedbackJSON,
		plan.CreatedAt.Unix(), plan.UpdatedAt.Unix(), plan.CreatedBy)

	if err != nil {
		return fmt.Errorf("saving plan: %w", err)
	}
	return nil
}

// GetPlan retrieves a plan by ID
func (s *Store) GetPlan(planID string) (*Plan, error) {
	var plan Plan
	var approvedAt sql.NullInt64

	var stepsJSON, filesToCreateJSON, filesToModifyJSON, dependenciesJSON, riskFactorsJSON, feedbackJSON string

	err := s.DB.QueryRow(`
		SELECT id, task_id, title, description, steps, files_to_create, files_to_modify,
			dependencies, estimated_time, complexity, risk_factors, status,
			approved_by, approved_at, rejection_reason, revision, parent_plan_id,
			feedback, created_at, updated_at, created_by
		FROM plans WHERE id = ?
	`, planID).Scan(
		&plan.ID, &plan.TaskID, &plan.Title, &plan.Description, &stepsJSON,
		&filesToCreateJSON, &filesToModifyJSON, &dependenciesJSON,
		&plan.EstimatedTime, &plan.Complexity, &riskFactorsJSON,
		&plan.Status, &plan.ApprovedBy, &approvedAt, &plan.RejectionReason,
		&plan.Revision, &plan.ParentPlanID, &feedbackJSON,
		&plan.CreatedAt, &plan.UpdatedAt, &plan.CreatedBy,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("plan not found: %s", planID)
	}
	if err != nil {
		return nil, fmt.Errorf("querying plan: %w", err)
	}

	// Unmarshal JSON fields
	if err := jsonUnmarshal(stepsJSON, &plan.Steps); err != nil {
		return nil, fmt.Errorf("unmarshaling steps: %w", err)
	}
	if err := jsonUnmarshal(filesToCreateJSON, &plan.FilesToCreate); err != nil {
		return nil, fmt.Errorf("unmarshaling files_to_create: %w", err)
	}
	if err := jsonUnmarshal(filesToModifyJSON, &plan.FilesToModify); err != nil {
		return nil, fmt.Errorf("unmarshaling files_to_modify: %w", err)
	}
	if err := jsonUnmarshal(dependenciesJSON, &plan.Dependencies); err != nil {
		return nil, fmt.Errorf("unmarshaling dependencies: %w", err)
	}
	if err := jsonUnmarshal(riskFactorsJSON, &plan.RiskFactors); err != nil {
		return nil, fmt.Errorf("unmarshaling risk_factors: %w", err)
	}
	if err := jsonUnmarshal(feedbackJSON, &plan.Feedback); err != nil {
		return nil, fmt.Errorf("unmarshaling feedback: %w", err)
	}

	// Convert estimated_time from seconds to duration
	plan.EstimatedTime = time.Duration(plan.EstimatedTime) * time.Second

	if approvedAt.Valid {
		t := time.Unix(approvedAt.Int64, 0)
		plan.ApprovedAt = &t
	}

	return &plan, nil
}

// GetPlanByTaskID retrieves the latest plan for a specific task
func (s *Store) GetPlanByTaskID(taskID string) (*Plan, error) {
	var planID string
	err := s.DB.QueryRow(`
		SELECT id FROM plans WHERE task_id = ?
		ORDER BY created_at DESC, revision DESC LIMIT 1
	`, taskID).Scan(&planID)

	if err == sql.ErrNoRows {
		return nil, nil // No plan found
	}
	if err != nil {
		return nil, fmt.Errorf("querying plan by task_id: %w", err)
	}

	return s.GetPlan(planID)
}

// ListPlans lists all plans, optionally filtered by status
func (s *Store) ListPlans(status PlanStatus) ([]*Plan, error) {
	var rows *sql.Rows
	var err error

	if status != "" {
		rows, err = s.DB.Query(`
			SELECT id FROM plans WHERE status = ?
			ORDER BY created_at DESC
		`, string(status))
	} else {
		rows, err = s.DB.Query(`
			SELECT id FROM plans ORDER BY created_at DESC
		`)
	}

	if err != nil {
		return nil, fmt.Errorf("querying plans: %w", err)
	}
	defer rows.Close()

	var plans []*Plan
	for rows.Next() {
		var planID string
		if err := rows.Scan(&planID); err != nil {
			return nil, fmt.Errorf("scanning plan id: %w", err)
		}

		plan, err := s.GetPlan(planID)
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}

	return plans, nil
}

// UpdatePlanStatus updates the status of a plan
func (s *Store) UpdatePlanStatus(planID string, status PlanStatus, reason string) error {
	_, err := s.DB.Exec(`
		UPDATE plans SET status = ?, updated_at = ?, rejection_reason = ?
		WHERE id = ?
	`, string(status), time.Now().Unix(), reason, planID)

	if err != nil {
		return fmt.Errorf("updating plan status: %w", err)
	}
	return nil
}

// AddFeedback adds feedback to a plan
func (s *Store) AddFeedback(planID string, feedback string) error {
	// Get existing plan
	plan, err := s.GetPlan(planID)
	if err != nil {
		return err
	}

	// Append feedback
	plan.Feedback = append(plan.Feedback, feedback)
	plan.UpdatedAt = time.Now()

	// Save updated plan
	return s.SavePlan(plan)
}

// ApprovePlan approves a plan
func (s *Store) ApprovePlan(planID, approvedBy string) error {
	now := time.Now()
	_, err := s.DB.Exec(`
		UPDATE plans SET status = 'approved', approved_by = ?, approved_at = ?, updated_at = ?
		WHERE id = ?
	`, approvedBy, now.Unix(), now.Unix(), planID)

	if err != nil {
		return fmt.Errorf("approving plan: %w", err)
	}
	return nil
}

// RejectPlan rejects a plan
func (s *Store) RejectPlan(planID, reason string) error {
	_, err := s.DB.Exec(`
		UPDATE plans SET status = 'rejected', rejection_reason = ?, updated_at = ?
		WHERE id = ?
	`, reason, time.Now().Unix(), planID)

	if err != nil {
		return fmt.Errorf("rejecting plan: %w", err)
	}
	return nil
}

// DeletePlan deletes a plan
func (s *Store) DeletePlan(planID string) error {
	result, err := s.DB.Exec(`DELETE FROM plans WHERE id = ?`, planID)
	if err != nil {
		return fmt.Errorf("deleting plan: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("plan not found: %s", planID)
	}
	return nil
}

// generateAPIKey generates a random API key
func generateAPIKey() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 32)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return "dro_" + string(b)
}

// jsonMarshal marshals a value to JSON
func jsonMarshal(v interface{}) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// jsonUnmarshal unmarshals JSON to a value
func jsonUnmarshal(s string, v interface{}) error {
	return json.Unmarshal([]byte(s), v)
}
