// Package config handles Drover configuration
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// ByteSize represents a size in bytes (for parsing from config)
type ByteSize int64

// DroverCodeConfig holds configuration specific to the drover-code agent
type DroverCodeConfig struct {
	Headless         bool   `toml:"headless"`
	JSONL            bool   `toml:"jsonl"`
	PermissionPreset string `toml:"permission_preset"` // "unikernel", "bypass", etc.
	ResultJSONPath   string `toml:"result_json_path"`
	CoordinatorMode  bool   `toml:"coordinator_mode"`
	Verbose          bool   `toml:"verbose"`
}

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
	AgentType  string  // "claude", "codex", "amp", or "opencode"
	AgentPath  string  // path to agent binary
	ClaudePath string  // deprecated: use AgentPath instead

	// Beads sync settings
	AutoSyncBeads bool

	// Project directory (detected)
	ProjectDir string

	// Verbose mode for debugging
	Verbose bool

	// Epic 2: Project-Level Configuration fields
	// Guidelines are project-specific instructions injected into agent prompts
	Guidelines string `toml:"guidelines"`

	// TaskContextCount controls how many recent completed tasks to include in context (Epic 6)
	TaskContextCount int `toml:"task_context_count"`

	// MaxDescriptionSize is the maximum size for task descriptions before using references (Epic 3)
	MaxDescriptionSize ByteSize `toml:"max_description_size"`

	// MaxDiffSize is the maximum size for diffs before using references (Epic 3)
	MaxDiffSize ByteSize `toml:"max_diff_size"`

	// DefaultLabels are labels applied to all tasks in this project
	DefaultLabels []string `toml:"default_labels"`

	// AgentPreferences contains agent-specific settings (model, temperature, etc.)
	AgentPreferences map[string]interface{} `toml:"agent_preferences"`

	// DroverCode settings
	DroverCode DroverCodeConfig `toml:"drover_code"`
}

// ProjectConfig represents the TOML config file structure
// This is separate from Config to allow TOML parsing with proper field names
type ProjectConfig struct {
	Agent             string                 `toml:"agent"`
	MaxWorkers        *int                   `toml:"max_workers"`
	TaskTimeout       string                 `toml:"task_timeout"`
	TaskContextCount  *int                   `toml:"task_context_count"`
	MaxDescriptionSize string                `toml:"max_description_size"`
	MaxDiffSize       string                 `toml:"max_diff_size"`
	Guidelines        string                 `toml:"guidelines"`
	DefaultLabels     []string               `toml:"default_labels"`
	AgentPreferences  map[string]interface{} `toml:"agent_preferences"`
}

// Load loads configuration from environment, TOML files, and defaults
// Configuration hierarchy (highest to lowest priority):
// 1. CLI flags (handled by caller)
// 2. Environment variables
// 3. Project config (.drover.toml)
// 4. Global config (~/.drover/config.toml)
// 5. Built-in defaults
func Load() (*Config, error) {
	projectDir, err := os.Getwd()
	if err != nil {
		projectDir = "."
	}

	cfg := &Config{
		DatabaseURL:       defaultDatabaseURL(),
		Workers:           3,
		TaskTimeout:       60 * time.Minute,
		MaxTaskAttempts:   3,
		ClaimTimeout:      5 * time.Minute,
		StallTimeout:      5 * time.Minute,
		PollInterval:     2 * time.Second,
		AutoUnblock:       true,
		WorktreeDir:       ".drover/worktrees",
		AgentType:         "claude", // Default to Claude for backwards compatibility
		AgentPath:         "claude",  // Will be resolved based on AgentType
		ClaudePath:        "claude",  // Deprecated but kept for backwards compatibility
		AutoSyncBeads:     false,     // Default to off for backwards compatibility
		ProjectDir:        projectDir,
		TaskContextCount:  3,                    // Default: include 3 recent tasks
		MaxDescriptionSize: 250 * 1024,         // Default: 250KB
		MaxDiffSize:       250 * 1024,          // Default: 250KB
		DefaultLabels:     []string{},          // Empty by default
		AgentPreferences:  make(map[string]interface{}),
	}

	// Load global config (lowest priority)
	globalPath := globalConfigPath()
	if err := loadConfigFile(cfg, globalPath); err != nil {
		// Non-fatal: global config is optional
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("loading global config: %w", err)
		}
	}

	// Load project config (overrides global)
	projectPath := filepath.Join(projectDir, ".drover.toml")
	if err := loadConfigFile(cfg, projectPath); err != nil {
		// Non-fatal: project config is optional
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("loading project config: %w", err)
		}
	}

	// Apply environment variables (overrides config files)
	applyEnvOverrides(cfg)

	// Resolve AgentPath based on AgentType if not explicitly set
	if cfg.AgentPath == "claude" && cfg.AgentType != "claude" {
		// AgentPath wasn't explicitly set, use default for the agent type
		switch cfg.AgentType {
		case "codex":
			cfg.AgentPath = "codex"
		case "amp":
			cfg.AgentPath = "amp"
		case "opencode":
			cfg.AgentPath = "opencode"
		}
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// loadConfigFile loads a TOML config file and merges it into cfg
func loadConfigFile(cfg *Config, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var projConfig ProjectConfig
	if err := toml.Unmarshal(data, &projConfig); err != nil {
		return fmt.Errorf("parsing TOML: %w", err)
	}

	// Merge into cfg
	if projConfig.Agent != "" {
		cfg.AgentType = projConfig.Agent
	}
	if projConfig.MaxWorkers != nil {
		cfg.Workers = *projConfig.MaxWorkers
	}
	if projConfig.TaskTimeout != "" {
		d, err := time.ParseDuration(projConfig.TaskTimeout)
		if err != nil {
			return fmt.Errorf("invalid task_timeout: %w", err)
		}
		cfg.TaskTimeout = d
	}
	if projConfig.TaskContextCount != nil {
		cfg.TaskContextCount = *projConfig.TaskContextCount
	}
	if projConfig.MaxDescriptionSize != "" {
		size, err := parseByteSize(projConfig.MaxDescriptionSize)
		if err != nil {
			return fmt.Errorf("invalid max_description_size: %w", err)
		}
		cfg.MaxDescriptionSize = size
	}
	if projConfig.MaxDiffSize != "" {
		size, err := parseByteSize(projConfig.MaxDiffSize)
		if err != nil {
			return fmt.Errorf("invalid max_diff_size: %w", err)
		}
		cfg.MaxDiffSize = size
	}
	if projConfig.Guidelines != "" {
		cfg.Guidelines = projConfig.Guidelines
	}
	if len(projConfig.DefaultLabels) > 0 {
		cfg.DefaultLabels = projConfig.DefaultLabels
	}
	if len(projConfig.AgentPreferences) > 0 {
		cfg.AgentPreferences = projConfig.AgentPreferences
	}

	return nil
}

// applyEnvOverrides applies environment variable overrides to cfg
func applyEnvOverrides(cfg *Config) {
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
	if v := os.Getenv("DROVER_TASK_CONTEXT_COUNT"); v != "" {
		cfg.TaskContextCount = parseIntOrDefault(v, 3)
	}
	if v := os.Getenv("DROVER_GUIDELINES"); v != "" {
		cfg.Guidelines = v
	}

	// Drover-code specific overrides
	if v := os.Getenv("DROVER_CODE_HEADLESS"); v != "" {
		cfg.DroverCode.Headless = v == "true" || v == "1"
	}
	if v := os.Getenv("DROVER_CODE_JSONL"); v != "" {
		cfg.DroverCode.JSONL = v == "true" || v == "1"
	}
	if v := os.Getenv("DROVER_CODE_PERMISSION_PRESET"); v != "" {
		cfg.DroverCode.PermissionPreset = v
	}
	if v := os.Getenv("DROVER_CODE_RESULT_PATH"); v != "" {
		cfg.DroverCode.ResultJSONPath = v
	}
	if v := os.Getenv("DROVER_CODE_COORDINATOR_MODE"); v != "" {
		cfg.DroverCode.CoordinatorMode = v == "true" || v == "1"
	}
	if v := os.Getenv("DROVER_CODE_VERBOSE"); v != "" {
		cfg.DroverCode.Verbose = v == "true" || v == "1"
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Workers < 1 {
		return fmt.Errorf("workers must be positive, got %d", c.Workers)
	}
	if c.TaskTimeout <= 0 {
		return fmt.Errorf("task_timeout must be positive, got %v", c.TaskTimeout)
	}
	if c.TaskContextCount < 0 {
		return fmt.Errorf("task_context_count must be non-negative, got %d", c.TaskContextCount)
	}
	if c.MaxDescriptionSize < 0 {
		return fmt.Errorf("max_description_size must be non-negative, got %d", c.MaxDescriptionSize)
	}
	if c.MaxDiffSize < 0 {
		return fmt.Errorf("max_diff_size must be non-negative, got %d", c.MaxDiffSize)
	}

	// Validate agent type
	validAgents := map[string]bool{
		"claude":     true,
		"codex":      true,
		"amp":        true,
		"opencode":   true,
		"drovercode": true,
	}
	if !validAgents[strings.ToLower(c.AgentType)] {
		return fmt.Errorf("invalid agent type: %s (valid: claude, codex, amp, opencode, drovercode)", c.AgentType)
	}

	return nil
}

// globalConfigPath returns the path to the global config file
func globalConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".drover", "config.toml")
}

// parseByteSize parses a byte size string like "250KB", "10MB", etc.
func parseByteSize(s string) (ByteSize, error) {
	s = strings.TrimSpace(s)
	s = strings.ToUpper(s)

	var multiplier int64 = 1
	if strings.HasSuffix(s, "KB") {
		multiplier = 1024
		s = strings.TrimSuffix(s, "KB")
	} else if strings.HasSuffix(s, "MB") {
		multiplier = 1024 * 1024
		s = strings.TrimSuffix(s, "MB")
	} else if strings.HasSuffix(s, "GB") {
		multiplier = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "GB")
	} else if strings.HasSuffix(s, "B") {
		multiplier = 1
		s = strings.TrimSuffix(s, "B")
	}

	var size int64
	if _, err := fmt.Sscanf(s, "%d", &size); err != nil {
		return 0, fmt.Errorf("invalid size format: %s", s)
	}

	return ByteSize(size * multiplier), nil
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
