// Package config handles Drover configuration
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cloud-shuttle/drover/internal/analytics"
	"github.com/cloud-shuttle/drover/internal/modes"
	"github.com/cloud-shuttle/drover/internal/webhooks"
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

	// Process-isolated worker settings (for OOM prevention)
	UseWorkerSubprocess bool   // use drover-worker for process isolation
	WorkerBinary        string // path to drover-worker binary (default: "drover-worker")
	WorkerMemoryLimit   string // memory limit for worker processes (e.g., "512M", "2G")

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
	PoolEnabled      bool
	PoolMinSize      int
	PoolMaxSize      int
	PoolWarmup       time.Duration
	PoolCleanupOnExit bool

	// Modes configuration (for planning/building separation)
	Modes *modes.Config

	// Webhook settings
	WebhooksEnabled bool
	WebhookURL      string
	WebhookSecret   string
	WebhookWorkers  int

	// Analytics settings
	AnalyticsEnabled  bool
	AnalyticsConfig   string
	AnalyticsMaxMetrics int
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
		UseWorkerSubprocess: false, // Process-isolated workers disabled by default
		WorkerBinary:        "drover-worker",
		WorkerMemoryLimit:   "",  // No memory limit by default
		WorkerMode:      modes.ModeCombined, // Default to combined mode
		RequireApproval: false,    // Default to no approval required
		Modes:           modes.DefaultConfig(), // Default modes configuration
		WebhookWorkers:  3,        // Default webhook delivery workers
		AnalyticsMaxMetrics: 10000, // Default max metrics in memory
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
	if v := os.Getenv("DROVER_USE_WORKER_SUBPROCESS"); v != "" {
		cfg.UseWorkerSubprocess = v == "true" || v == "1"
	}
	if v := os.Getenv("DROVER_WORKER_BINARY"); v != "" {
		cfg.WorkerBinary = v
	}
	if v := os.Getenv("DROVER_WORKER_MEMORY_LIMIT"); v != "" {
		cfg.WorkerMemoryLimit = v
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
	if v := os.Getenv("DROVER_WEBHOOKS_ENABLED"); v != "" {
		cfg.WebhooksEnabled = v == "true" || v == "1"
	}
	if v := os.Getenv("DROVER_WEBHOOK_URL"); v != "" {
		cfg.WebhookURL = v
	}
	if v := os.Getenv("DROVER_WEBHOOK_SECRET"); v != "" {
		cfg.WebhookSecret = v
	}
	if v := os.Getenv("DROVER_WEBHOOK_WORKERS"); v != "" {
		cfg.WebhookWorkers = parseIntOrDefault(v, 3)
	}
	if v := os.Getenv("DROVER_ANALYTICS_ENABLED"); v != "" {
		cfg.AnalyticsEnabled = v == "true" || v == "1"
	}
	if v := os.Getenv("DROVER_ANALYTICS_CONFIG"); v != "" {
		cfg.AnalyticsConfig = v
	} else if cfg.AnalyticsEnabled {
		// Default to .drover/analytics.json if enabled but no config specified
		cfg.AnalyticsConfig = ".drover/analytics.json"
	}
	if v := os.Getenv("DROVER_ANALYTICS_MAX_METRICS"); v != "" {
		cfg.AnalyticsMaxMetrics = parseIntOrDefault(v, 10000)
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

// CreateWebhookManager creates and configures a webhook manager from the config
func (c *Config) CreateWebhookManager() *webhooks.Manager {
	mgr := webhooks.NewManager()
	if c.WebhookWorkers > 0 {
		mgr.SetTimeout(30 * time.Second)
	}

	// Register webhook if configured
	if c.WebhooksEnabled && c.WebhookURL != "" {
		webhook := &webhooks.Webhook{
			ID:     "default",
			URL:    c.WebhookURL,
			Secret: c.WebhookSecret,
			Events: []webhooks.EventType{
				webhooks.EventTaskCreated,
				webhooks.EventTaskClaimed,
				webhooks.EventTaskStarted,
				webhooks.EventTaskPaused,
				webhooks.EventTaskResumed,
				webhooks.EventTaskBlocked,
				webhooks.EventTaskCompleted,
				webhooks.EventTaskFailed,
				webhooks.EventWorkerStarted,
				webhooks.EventWorkerStopped,
			},
			Enabled: true,
		}
		if err := mgr.Register(webhook); err != nil {
			// Log error but don't fail - webhooks are optional
			fmt.Printf("[config] warning: failed to register webhook: %v\n", err)
		} else {
			fmt.Printf("[config] webhooks enabled: %s\n", c.WebhookURL)
		}
	}

	return mgr
}

// CreateAnalyticsManager creates and configures an analytics manager from the config
func (c *Config) CreateAnalyticsManager() (*analytics.Manager, error) {
	if !c.AnalyticsEnabled {
		return nil, nil
	}

	mgr, err := analytics.NewManager(analytics.Config{
		ConfigPath: c.AnalyticsConfig,
		MaxMetrics: c.AnalyticsMaxMetrics,
	})
	if err != nil {
		return nil, fmt.Errorf("creating analytics manager: %w", err)
	}

	fmt.Printf("[config] analytics enabled: %s (maxMetrics=%d)\n", c.AnalyticsConfig, c.AnalyticsMaxMetrics)
	return mgr, nil
}
