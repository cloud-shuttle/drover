// Package config handles Drover configuration
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cloud-shuttle/drover/internal/modes"
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
	AgentType  string // "claude", "codex", "copilot", "amp" or "opencode"
	AgentPath  string // path to agent binary
	ClaudePath string // deprecated: use AgentPath instead

	// Worker mode settings (for planning/building separation)
	WorkerMode    modes.WorkerMode // "combined", "planning", or "building"
	RequireApproval bool             // require manual approval for plans

	// Beads sync settings
	AutoSyncBeads bool

	// Project directory (detected)
	ProjectDir string

	// Verbose mode for debugging
	Verbose bool

	// Worktree pool settings
	PoolEnabled       bool
	PoolMinSize       int
	PoolMaxSize       int
	PoolWarmup        time.Duration
	PoolCleanupOnExit bool

	// Modes configuration (for planning/building separation)
	Modes *modes.Config
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
		WorkerMode:      modes.ModeCombined, // Default to combined mode
		RequireApproval: false,    // Default to no approval required
		Modes:           modes.DefaultConfig(), // Default modes configuration
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
	if v := os.Getenv("DROVER_WORKER_MODE"); v != "" {
		cfg.WorkerMode = modes.WorkerMode(v)
	}
	if v := os.Getenv("DROVER_REQUIRE_APPROVAL"); v != "" {
		cfg.RequireApproval = v == "true" || v == "1"
	}
	if v := os.Getenv("DROVER_PLANNING_REQUIRE_APPROVAL"); v != "" {
		cfg.Modes.Planning.RequireApproval = v == "true" || v == "1"
	}
	if v := os.Getenv("DROVER_PLANNING_AUTO_APPROVE_LOW"); v != "" {
		cfg.Modes.Planning.AutoApproveLowComplexity = v == "true" || v == "1"
	}
	if v := os.Getenv("DROVER_PLANNING_MAX_STEPS"); v != "" {
		cfg.Modes.Planning.MaxStepsPerPlan = parseIntOrDefault(v, 20)
	}
	if v := os.Getenv("DROVER_BUILDING_APPROVED_ONLY"); v != "" {
		cfg.Modes.Building.ExecuteApprovedOnly = v == "true" || v == "1"
	}
	if v := os.Getenv("DROVER_BUILDING_VERIFY_STEPS"); v != "" {
		cfg.Modes.Building.VerifySteps = v == "true" || v == "1"
	}
	if v := os.Getenv("DROVER_REFINEMENT_ENABLED"); v != "" {
		cfg.Modes.Refinement.Enabled = v == "true" || v == "1"
	}
	if v := os.Getenv("DROVER_REFINEMENT_MAX_REFINEMENTS"); v != "" {
		cfg.Modes.Refinement.MaxRefinements = parseIntOrDefault(v, 3)
	}

	// Resolve AgentPath based on AgentType if not explicitly set
	if cfg.AgentPath == "claude" && cfg.AgentType != "claude" {
		// AgentPath wasn't explicitly set, use default for the agent type
		switch cfg.AgentType {
		case "codex":
			cfg.AgentPath = "codex"
		case "copilot":
			cfg.AgentPath = "copilot"
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

// GetOperator returns the current operator name from environment or config file
func GetOperator() string {
	if v := os.Getenv("DROVER_OPERATOR"); v != "" {
		return v
	}

	// Try to read from config file
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	configFile := filepath.Join(homeDir, ".drover", "config.json")
	data, err := os.ReadFile(configFile)
	if err != nil {
		return ""
	}

	// Simple JSON parsing for just the operator field
	type ConfigFile struct {
		Operator string `json:"operator"`
	}
	var cfg ConfigFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		return ""
	}

	return cfg.Operator
}

// SetOperator saves the operator name to the config file
func SetOperator(name string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("getting home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, ".drover")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	configFile := filepath.Join(configDir, "config.json")

	// Read existing config or create new
	type ConfigFile struct {
		Operator string `json:"operator"`
	}
	var cfg ConfigFile

	data, err := os.ReadFile(configFile)
	if err == nil {
		json.Unmarshal(data, &cfg)
	}

	// Update operator
	cfg.Operator = name

	// Write back
	data, err = json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(configFile, data, 0644); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}

	return nil
}
