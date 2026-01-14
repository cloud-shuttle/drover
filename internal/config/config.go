// Package config handles Drover configuration
package config

import (
	"fmt"
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

	// Agent settings
	AgentType  string  // "claude", "codex", or "amp"
	AgentPath  string  // path to agent binary
	ClaudePath string  // deprecated: use AgentPath instead

	// Beads sync settings
	AutoSyncBeads bool

	// Project directory (detected)
	ProjectDir string

	// Verbose mode for debugging
	Verbose bool

	// Worktree pool settings
	PoolEnabled      bool
	PoolMinSize      int
	PoolMaxSize      int
	PoolWarmup       time.Duration
	PoolCleanupOnExit bool
}

// Load loads configuration from environment and defaults
func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL:     defaultDatabaseURL(),
		Workers:         3,
		TaskTimeout:     60 * time.Minute,
		MaxTaskAttempts: 3,
		ClaimTimeout:    5 * time.Minute,
		StallTimeout:    5 * time.Minute,
		PollInterval:    2 * time.Second,
		AutoUnblock:     true,
		WorktreeDir:     ".drover/worktrees",
		AgentType:       "claude", // Default to Claude for backwards compatibility
		AgentPath:       "claude", // Will be resolved based on AgentType
		ClaudePath:      "claude", // Deprecated but kept for backwards compatibility
		AutoSyncBeads:   false,    // Default to off for backwards compatibility
		PoolEnabled:     false,    // Worktree pooling disabled by default
		PoolMinSize:     2,        // Minimum warm worktrees
		PoolMaxSize:     10,       // Maximum pooled worktrees
		PoolWarmup:      5 * time.Minute,
		PoolCleanupOnExit: true,   // Clean up pooled worktrees on exit
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
	if v := os.Getenv("DROVER_AGENT_TYPE"); v != "" {
		cfg.AgentType = v
	}
	if v := os.Getenv("DROVER_AGENT_PATH"); v != "" {
		cfg.AgentPath = v
	} else if v := os.Getenv("DROVER_CLAUDE_PATH"); v != "" {
		// Deprecated: DROVER_CLAUDE_PATH for backwards compatibility
		cfg.AgentPath = v
		cfg.ClaudePath = v
	}
	if v := os.Getenv("DROVER_POOL_ENABLED"); v != "" {
		cfg.PoolEnabled = v == "true" || v == "1"
	}
	if v := os.Getenv("DROVER_POOL_MIN_SIZE"); v != "" {
		cfg.PoolMinSize = parseIntOrDefault(v, 2)
	}
	if v := os.Getenv("DROVER_POOL_MAX_SIZE"); v != "" {
		cfg.PoolMaxSize = parseIntOrDefault(v, 10)
	}
	if v := os.Getenv("DROVER_POOL_WARMUP"); v != "" {
		cfg.PoolWarmup = parseDurationOrDefault(v, 5*time.Minute)
	}
	if v := os.Getenv("DROVER_POOL_CLEANUP_ON_EXIT"); v != "" {
		cfg.PoolCleanupOnExit = v == "true" || v == "1"
	}

	// Resolve AgentPath based on AgentType if not explicitly set
	if cfg.AgentPath == "claude" && cfg.AgentType != "claude" {
		// AgentPath wasn't explicitly set, use default for the agent type
		switch cfg.AgentType {
		case "codex":
			cfg.AgentPath = "codex"
		case "amp":
			cfg.AgentPath = "amp"
		}
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
	var i int
	if _, err := fmt.Sscanf(s, "%d", &i); err != nil {
		return def
	}
	return i
}

func parseDurationOrDefault(s string, def time.Duration) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		return def
	}
	return d
}
