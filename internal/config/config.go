// Package config handles Drover configuration
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// AgentType represents the AI agent to use for task execution
type AgentType string

const (
	AgentTypeClaudeCode AgentType = "claude-code"
	AgentTypeOpenCode   AgentType = "opencode"
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
	ClaimTimeout time.Duration
	StallTimeout time.Duration
	PollInterval time.Duration
	AutoUnblock  bool

	// Git settings
	WorktreeDir string

	// Agent settings
	AgentType         AgentType // "claude-code" or "opencode"
	ClaudePath        string    // Path to Claude CLI (default: "claude")
	OpenCodePath      string    // Path to OpenCode CLI (default: "opencode")
	OpenCodeModel     string    // Model in format "provider/model" (e.g., "anthropic/claude-sonnet-4-20250514")
	OpenCodeURL       string    // Optional remote OpenCode server URL
	MergeTargetBranch string    // Branch to merge changes to (default: "main")

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
		DatabaseURL:       defaultDatabaseURL(),
		Workers:           3,
		TaskTimeout:       60 * time.Minute,
		MaxTaskAttempts:   3,
		ClaimTimeout:      5 * time.Minute,
		StallTimeout:      5 * time.Minute,
		PollInterval:      2 * time.Second,
		AutoUnblock:       true,
		WorktreeDir:       ".drover/worktrees",
		ClaudePath:        "claude",
		OpenCodePath:      "opencode",
		OpenCodeModel:     "anthropic/claude-sonnet-4-20250514",
		AgentType:         AgentTypeClaudeCode,
		MergeTargetBranch: "main",
		AutoSyncBeads:     false,
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
		cfg.AgentType = AgentType(v)
	}
	if v := os.Getenv("DROVER_CLAUDE_PATH"); v != "" {
		cfg.ClaudePath = v
	}
	if v := os.Getenv("DROVER_OPENCODE_PATH"); v != "" {
		cfg.OpenCodePath = v
	}
	if v := os.Getenv("DROVER_OPENCODE_MODEL"); v != "" {
		cfg.OpenCodeModel = v
	}
	if v := os.Getenv("DROVER_OPENCODE_URL"); v != "" {
		cfg.OpenCodeURL = v
	}
	if v := os.Getenv("DROVER_MERGE_TARGET_BRANCH"); v != "" {
		cfg.MergeTargetBranch = v
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

// ValidateOpenCodeModel validates that the model format is "provider/model"
func ValidateOpenCodeModel(model string) error {
	parts := strings.Split(model, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("invalid OpenCode model format: %s (expected provider/model, e.g., anthropic/claude-sonnet-4-20250514)", model)
	}
	return nil
}

// GetAgentExecutorPath returns the path to the agent CLI based on agent type
func (c *Config) GetAgentExecutorPath() string {
	if c.AgentType == AgentTypeOpenCode {
		return c.OpenCodePath
	}
	return c.ClaudePath
}
