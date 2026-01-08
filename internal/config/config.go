// Package config handles Drover configuration
package config

import (
	"os"
	"path/filepath"
	"time"
)

// Config holds Drover configuration
type Config struct {
	// Database connection
	DatabaseURL string

	// Worker settings
	Workers int

	// Task settings
	TaskTimeout     time.Duration
	MaxTaskAttempts int

	// Retry settings
	ClaimTimeout  time.Duration
	StallTimeout  time.Duration
	PollInterval  time.Duration
	AutoUnblock   bool

	// Git settings
	WorktreeDir string

	// Claude settings
	ClaudePath string

	// Beads sync settings
	AutoSyncBeads bool

	// Project directory (detected)
	ProjectDir string

	// Verbose mode for debugging
	Verbose bool
}

// Load loads configuration from environment and defaults
func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL:     defaultDatabaseURL(),
		Workers:         1,
		TaskTimeout:     60 * time.Minute,
		MaxTaskAttempts: 3,
		ClaimTimeout:    5 * time.Minute,
		StallTimeout:    5 * time.Minute,
		PollInterval:    2 * time.Second,
		AutoUnblock:     true,
		WorktreeDir:     ".drover/worktrees",
		ClaudePath:      "claude",
		AutoSyncBeads:   false, // Default to off for backwards compatibility
	}

	// Environment overrides
	if v := os.Getenv("DROVER_DATABASE_URL"); v != "" {
		cfg.DatabaseURL = v
	}
	if v := os.Getenv("DROVER_WORKERS"); v != "" {
		cfg.Workers = parseIntOrDefault(v, 4)
	}
	if v := os.Getenv("DROVER_TASK_TIMEOUT"); v != "" {
		cfg.TaskTimeout = parseDurationOrDefault(v, 10*time.Minute)
	}
	if v := os.Getenv("DROVER_AUTO_SYNC_BEADS"); v != "" {
		cfg.AutoSyncBeads = v == "true" || v == "1"
	}

	return cfg, nil
}

// defaultDatabaseURL returns SQLite in project directory
func defaultDatabaseURL() string {
	dir, err := os.Getwd()
	if err != nil {
		return "sqlite://.drover/db"
	}
	return "sqlite://" + filepath.Join(dir, ".drover", "drover.db")
}

func parseIntOrDefault(s string, def int) int {
	// Simple parsing
	return def
}

func parseDurationOrDefault(s string, def time.Duration) time.Duration {
	// Simple parsing
	return def
}
