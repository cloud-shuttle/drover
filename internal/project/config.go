// Package project provides per-project configuration management
package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Config holds per-project Drover configuration
type Config struct {
	// Agent configuration
	Agent        string        `toml:"agent"`
	MaxWorkers   int           `toml:"max_workers"`
	TaskTimeout  time.Duration `toml:"task_timeout"`
	MaxAttempts  int           `toml:"max_attempts"`

	// Context settings
	TaskContextCount  int           `toml:"task_context_count"`
	TaskContextMaxAge time.Duration `toml:"task_context_max_age"`

	// Size thresholds
	MaxDescriptionSize ByteSize `toml:"max_description_size"`
	MaxDiffSize       ByteSize `toml:"max_diff_size"`
	MaxFileSize       ByteSize `toml:"max_file_size"`

	// Project-specific guidelines
	Guidelines string `toml:"guidelines"`

	// Labels to apply to all tasks
	DefaultLabels []string `toml:"default_labels"`

	// File path where this config was loaded
	configPath string
}

// ByteSize represents a size in bytes (supports KB, MB, GB suffixes in TOML)
type ByteSize int64

// UnmarshalText parses byte sizes from TOML (e.g., "250KB", "1MB")
func (b *ByteSize) UnmarshalText(text []byte) error {
	s := strings.TrimSpace(string(text))
	if s == "" {
		*b = 0
		return nil
	}

	var multiplier int64 = 1
	var numStr string

	// Extract suffix
	if strings.HasSuffix(s, "GB") || strings.HasSuffix(s, "gb") || strings.HasSuffix(s, "Gb") || strings.HasSuffix(s, "gB") {
		multiplier = 1024 * 1024 * 1024
		numStr = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(s, "GB"), "gb"), "Gb"), "gB")
	} else if strings.HasSuffix(s, "MB") || strings.HasSuffix(s, "mb") || strings.HasSuffix(s, "Mb") || strings.HasSuffix(s, "mB") {
		multiplier = 1024 * 1024
		numStr = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(s, "MB"), "mb"), "Mb"), "mB")
	} else if strings.HasSuffix(s, "KB") || strings.HasSuffix(s, "kb") || strings.HasSuffix(s, "Kb") || strings.HasSuffix(s, "kB") {
		multiplier = 1024
		numStr = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(s, "KB"), "kb"), "Kb"), "kB")
	} else {
		numStr = s
	}

	// Parse number
	var size int64
	_, err := fmt.Sscanf(numStr, "%d", &size)
	if err != nil {
		return fmt.Errorf("invalid byte size format: %q", s)
	}

	*b = ByteSize(size * multiplier)
	return nil
}

// Bytes returns the size in bytes
func (b ByteSize) Bytes() int64 {
	return int64(b)
}

// String returns a human-readable representation
func (b ByteSize) String() string {
	if b >= 1024*1024*1024 {
		return fmt.Sprintf("%.1fGB", float64(b)/(1024*1024*1024))
	}
	if b >= 1024*1024 {
		return fmt.Sprintf("%.1fMB", float64(b)/(1024*1024))
	}
	if b >= 1024 {
		return fmt.Sprintf("%.1fKB", float64(b)/1024)
	}
	return fmt.Sprintf("%dB", b)
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		Agent:             "claude",
		MaxWorkers:        3,
		TaskTimeout:       60 * time.Minute,
		MaxAttempts:       3,
		TaskContextCount:  5,
		TaskContextMaxAge: 24 * time.Hour, // 24 hours
		MaxDescriptionSize: 250 * 1024 * 1024, // 250MB
		MaxDiffSize:        250 * 1024 * 1024, // 250MB
		MaxFileSize:        1024 * 1024,       // 1MB
		DefaultLabels:      []string{},
	}
}

// Load loads the project configuration from the project directory
// If no .drover.toml exists, returns a default config
func Load(projectDir string) (*Config, error) {
	cfg := DefaultConfig()
	cfg.configPath = filepath.Join(projectDir, ".drover.toml")

	// Try to load .drover.toml
	data, err := os.ReadFile(cfg.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// No config file, return defaults
			return cfg, nil
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	// Parse TOML
	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", cfg.configPath, err)
	}

	return cfg, nil
}

// Save saves the configuration to .drover.toml
func (c *Config) Save() error {
	if c.configPath == "" {
		return fmt.Errorf("no config path set")
	}

	// Ensure directory exists
	dir := filepath.Dir(c.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	// Write config file
	f, err := os.Create(c.configPath)
	if err != nil {
		return fmt.Errorf("creating config file: %w", err)
	}
	defer f.Close()

	encoder := toml.NewEncoder(f)
	if err := encoder.Encode(c); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	return nil
}

// ConfigPath returns the path to the config file
func (c *Config) ConfigPath() string {
	return c.configPath
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.MaxWorkers < 1 {
		return fmt.Errorf("max_workers must be at least 1")
	}
	if c.MaxWorkers > 20 {
		return fmt.Errorf("max_workers cannot exceed 20")
	}
	if c.TaskTimeout < time.Minute {
		return fmt.Errorf("task_timeout must be at least 1 minute")
	}
	if c.TaskTimeout > 4*time.Hour {
		return fmt.Errorf("task_timeout cannot exceed 4 hours")
	}
	if c.MaxAttempts < 1 {
		return fmt.Errorf("max_attempts must be at least 1")
	}
	if c.MaxAttempts > 10 {
		return fmt.Errorf("max_attempts cannot exceed 10")
	}
	if c.TaskContextCount < 0 {
		return fmt.Errorf("task_context_count cannot be negative")
	}
	if c.TaskContextCount > 20 {
		return fmt.Errorf("task_context_count cannot exceed 20")
	}
	if c.TaskContextMaxAge < time.Minute {
		return fmt.Errorf("task_context_max_age must be at least 1 minute")
	}
	if c.TaskContextMaxAge > 168*time.Hour { // 7 days
		return fmt.Errorf("task_context_max_age cannot exceed 7 days")
	}

	// Validate agent type
	validAgents := map[string]bool{
		"claude":  true,
		"codex":   true,
		"amp":     true,
		"opencode": true,
	}
	if c.Agent != "" && !validAgents[c.Agent] {
		return fmt.Errorf("unknown agent type: %s (valid: claude, codex, amp, opencode)", c.Agent)
	}

	return nil
}

// MergeWith merges project config with global config values
// Global config takes precedence for values not set in project config
func (c *Config) MergeWithGlobal(agent string, workers int, timeout time.Duration, maxAttempts int) {
	// Only use project config values if they differ from defaults
	if c.Agent == "" || c.Agent == "claude" {
		c.Agent = agent
	}
	if c.MaxWorkers <= 3 {
		c.MaxWorkers = workers
	}
	if c.TaskTimeout <= 60*time.Minute {
		c.TaskTimeout = timeout
	}
	if c.MaxAttempts <= 3 {
		c.MaxAttempts = maxAttempts
	}
}

// GetGuidelines returns the project guidelines formatted for prompt inclusion
func (c *Config) GetGuidelines() string {
	if c.Guidelines == "" {
		return ""
	}
	return strings.TrimSpace(c.Guidelines)
}

// HasLabels returns true if default labels are configured
func (c *Config) HasLabels() bool {
	return len(c.DefaultLabels) > 0
}

// GetLabels returns the default labels
func (c *Config) GetLabels() []string {
	if c.DefaultLabels == nil {
		return []string{}
	}
	return c.DefaultLabels
}
