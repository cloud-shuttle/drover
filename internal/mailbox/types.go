// Package mailbox provides file-based task queue implementation
package mailbox

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/cloud-shuttle/drover/pkg/types"
)

// Config holds mailbox configuration
type Config struct {
	// Directory is the base path for the mailbox
	Directory string

	// Retention periods for cleanup
	OutboxRetention  time.Duration
	FailedRetention  time.Duration
	TmpCleanupAge   time.Duration

	// Scan interval for orphaned task recovery
	OrphanScanInterval time.Duration
}

// DefaultConfig returns default mailbox configuration
func DefaultConfig() *Config {
	return &Config{
		Directory:         "/tmp/drover/mailbox",
		OutboxRetention:   7 * 24 * time.Hour,
		FailedRetention:   30 * 24 * time.Hour,
		TmpCleanupAge:     1 * time.Hour,
		OrphanScanInterval: 5 * time.Minute,
	}
}

// FileMailbox implements a file-based task queue
type FileMailbox struct {
	mu      sync.RWMutex
	config  *Config
	closing chan struct{}
	wg      sync.WaitGroup

	// Directory paths (absolute)
	inboxDir      string
	processingDir string
	outboxDir     string
	failedDir     string
	tmpDir        string
}

// NewFileMailbox creates a new file-based mailbox
func NewFileMailbox(cfg *Config) (*FileMailbox, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// Convert to absolute path
	dir, err := filepath.Abs(cfg.Directory)
	if err != nil {
		return nil, fmt.Errorf("getting absolute path: %w", err)
	}

	m := &FileMailbox{
		config:        cfg,
		closing:       make(chan struct{}),
		inboxDir:      filepath.Join(dir, "inbox"),
		processingDir: filepath.Join(dir, "processing"),
		outboxDir:     filepath.Join(dir, "outbox"),
		failedDir:     filepath.Join(dir, "failed"),
		tmpDir:        filepath.Join(dir, ".tmp"),
	}

	// Create directories
	for _, dir := range []string{m.inboxDir, m.processingDir, m.outboxDir, m.failedDir, m.tmpDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("creating directory %s: %w", dir, err)
		}
	}

	return m, nil
}

// Enqueue writes a task to the inbox
func (m *FileMailbox) Enqueue(task *types.Task) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Write to temporary file first
	tmpPath := filepath.Join(m.tmpDir, task.ID+".json.tmp")
	taskPath := filepath.Join(m.inboxDir, task.ID+".json")

	// Check if task already exists
	if _, err := os.Stat(taskPath); err == nil {
		return ErrTaskExists
	}

	// Write task to temp file
	if err := writeTaskFile(tmpPath, task); err != nil {
		return fmt.Errorf("writing task file: %w", err)
	}

	// Atomic rename to inbox
	if err := os.Rename(tmpPath, taskPath); err != nil {
		// Clean up temp file
		os.Remove(tmpPath)
		return fmt.Errorf("moving task to inbox: %w", err)
	}

	return nil
}

// Claim attempts to claim a task from inbox
// Returns nil task and nil error if no tasks available (ErrNoTasks)
func (m *FileMailbox) Claim(workerID string) (*types.Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// List inbox directory
	entries, err := os.ReadDir(m.inboxDir)
	if err != nil {
		return nil, fmt.Errorf("reading inbox: %w", err)
	}

	// Try to claim each task
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Skip non-JSON files
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		taskID := entry.Name()[:len(entry.Name())-5] // Remove .json
		srcPath := filepath.Join(m.inboxDir, entry.Name())
		dstPath := filepath.Join(m.processingDir, entry.Name())

		// Atomic rename to processing
		if err := os.Rename(srcPath, dstPath); err != nil {
			// If file doesn't exist, another worker claimed it
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("claiming task %s: %w", taskID, err)
		}

		// Load the task
		task, err := readTaskFile(dstPath)
		if err != nil {
			// Move back to inbox if we can't read it
			_ = os.Rename(dstPath, srcPath)
			return nil, fmt.Errorf("reading task %s: %w", taskID, err)
		}

		// Update task with claim info
		now := time.Now().Unix()
		task.Status = types.TaskStatusClaimed
		task.ClaimedBy = workerID
		task.ClaimedAt = &now

		// Write updated task back
		if err := writeTaskFile(dstPath, task); err != nil {
			return nil, fmt.Errorf("updating claimed task: %w", err)
		}

		return task, nil
	}

	return nil, ErrNoTasks
}

// Complete moves a task from processing to outbox
func (m *FileMailbox) Complete(taskID string, result *TaskResult) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	processingPath := filepath.Join(m.processingDir, taskID+".json")
	resultPath := filepath.Join(m.outboxDir, taskID+"_result.json")
	tmpResultPath := filepath.Join(m.tmpDir, taskID+"_result.json.tmp")

	// Write result to temp file
	if err := writeResultFile(tmpResultPath, result); err != nil {
		return fmt.Errorf("writing result file: %w", err)
	}

	// Atomic rename to outbox
	if err := os.Rename(tmpResultPath, resultPath); err != nil {
		os.Remove(tmpResultPath)
		return fmt.Errorf("moving result to outbox: %w", err)
	}

	// Remove from processing
	if err := os.Remove(processingPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing task from processing: %w", err)
	}

	return nil
}

// Fail moves a task from processing to failed
func (m *FileMailbox) Fail(taskID string, taskErr *TaskError) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	processingPath := filepath.Join(m.processingDir, taskID+".json")
	errorPath := filepath.Join(m.failedDir, taskID+"_error.json")
	tmpErrorPath := filepath.Join(m.tmpDir, taskID+"_error.json.tmp")

	// Write error info to temp file
	if err := writeErrorFile(tmpErrorPath, taskErr); err != nil {
		return fmt.Errorf("writing error file: %w", err)
	}

	// Atomic rename to failed
	if err := os.Rename(tmpErrorPath, errorPath); err != nil {
		os.Remove(tmpErrorPath)
		return fmt.Errorf("moving error to failed: %w", err)
	}

	// Remove from processing
	if err := os.Remove(processingPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing task from processing: %w", err)
	}

	return nil
}

// Start begins background cleanup routines
func (m *FileMailbox) Start() {
	m.wg.Add(2)
	go m.cleanupRoutine()
	go m.orphanRecoveryRoutine()
}

// Stop gracefully shuts down the mailbox
func (m *FileMailbox) Stop() {
	close(m.closing)
	m.wg.Wait()
}

// cleanupRoutine periodically cleans up old files
func (m *FileMailbox) cleanupRoutine() {
	defer m.wg.Done()

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-m.closing:
			return
		case <-ticker.C:
			m.cleanupOutbox()
			m.cleanupFailed()
			m.cleanupTmp()
		}
	}
}

// orphanRecoveryRoutine recovers tasks from processing/ that were orphaned
func (m *FileMailbox) orphanRecoveryRoutine() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.config.OrphanScanInterval)
	defer ticker.Stop()

	// Run once on startup
	m.recoverOrphans()

	for {
		select {
		case <-m.closing:
			return
		case <-ticker.C:
			m.recoverOrphans()
		}
	}
}

// recoverOrphans moves stale tasks from processing back to inbox
func (m *FileMailbox) recoverOrphans() {
	m.mu.Lock()
	defer m.mu.Unlock()

	entries, err := os.ReadDir(m.processingDir)
	if err != nil {
		return
	}

	now := time.Now()
	staleThreshold := 15 * time.Minute // Tasks older than 15min are considered stale

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Check if file is stale
		if now.Sub(info.ModTime()) > staleThreshold {
			_ = entry.Name()[:len(entry.Name())-5] // Extract task ID for context
			srcPath := filepath.Join(m.processingDir, entry.Name())
			dstPath := filepath.Join(m.inboxDir, entry.Name())

			// Move back to inbox
			if err := os.Rename(srcPath, dstPath); err == nil {
				// Successfully recovered
			}
		}
	}
}

// cleanupOutbox removes old result files
func (m *FileMailbox) cleanupOutbox() {
	cutoff := time.Now().Add(-m.config.OutboxRetention)

	_ = filepath.Walk(m.outboxDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		if info.ModTime().Before(cutoff) {
			os.Remove(path)
		}
		return nil
	})
}

// cleanupFailed removes old error files
func (m *FileMailbox) cleanupFailed() {
	cutoff := time.Now().Add(-m.config.FailedRetention)

	_ = filepath.Walk(m.failedDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		if info.ModTime().Before(cutoff) {
			os.Remove(path)
		}
		return nil
	})
}

// cleanupTmp removes stale temp files
func (m *FileMailbox) cleanupTmp() {
	cutoff := time.Now().Add(-m.config.TmpCleanupAge)

	_ = filepath.Walk(m.tmpDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		if info.ModTime().Before(cutoff) {
			os.Remove(path)
		}
		return nil
	})
}

// TaskResult represents the result of a completed task
type TaskResult struct {
	TaskID      string        `json:"task_id"`
	Status      types.TaskStatus `json:"status"`
	Verdict     types.TaskVerdict `json:"verdict"`
	VerdictReason string     `json:"verdict_reason"`
	Output      string        `json:"output"`
	DurationMs  int64         `json:"duration_ms"`
	CompletedAt int64         `json:"completed_at"`
}

// TaskError represents information about a failed task
type TaskError struct {
	TaskID        string `json:"task_id"`
	Error         string `json:"error"`
	Attempts      int    `json:"attempts"`
	LastAttemptAt int64  `json:"last_attempt_at"`
	FailedAt      int64  `json:"failed_at"`
}

// Errors
var (
	ErrTaskExists  = fmt.Errorf("task already exists in inbox")
	ErrNoTasks     = fmt.Errorf("no tasks available")
	ErrMailboxDir  = fmt.Errorf("mailbox directory error")
	ErrTaskNotFound = fmt.Errorf("task not found")
	ErrInvalidTask = fmt.Errorf("invalid task format")
)
