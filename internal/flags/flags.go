// Package flags provides a runtime feature flag system for Drover.
// Supports gradual rollouts, kill switches, and per-project overrides.
package flags

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FlagType represents the type of a flag value
type FlagType string

const (
	TypeBool   FlagType = "bool"
	TypeInt    FlagType = "int"
	TypeFloat  FlagType = "float"
	TypeString FlagType = "string"
)

// FlagRollout defines percent-based rollout configuration
type FlagRollout struct {
	Enabled    bool    `json:"enabled"`
	Percentage int     `json:"percentage"` // 0-100
	Whitelist  []string `json:"whitelist"`  // Project IDs that always get the feature
	Blacklist  []string `json:"blacklist"`  // Project IDs that never get the feature
}

// Flag represents a single feature flag
type Flag struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Type        FlagType      `json:"type"`
	Default     interface{}   `json:"default"`
	Value       interface{}   `json:"value"`
	Rollout     *FlagRollout  `json:"rollout,omitempty"`
	CreatedAt   int64         `json:"created_at"`
	UpdatedAt   int64         `json:"updated_at"`
}

// Manager manages feature flags with runtime configuration
type Manager struct {
	mu       sync.RWMutex
	flags    map[string]*Flag
	watchers []chan *FlagUpdate
	logger   *log.Logger
	configPath string
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// FlagUpdate represents a flag change notification
type FlagUpdate struct {
	FlagID  string
	OldValue interface{}
	NewValue interface{}
	Timestamp int64
}

// Config holds manager configuration
type Config struct {
	ConfigPath string
	Watch      bool // Enable file watching for hot reload
}

// NewManager creates a new feature flag manager
func NewManager(cfg Config) (*Manager, error) {
	m := &Manager{
		flags:      make(map[string]*Flag),
		watchers:   make([]chan *FlagUpdate, 0),
		logger:     log.New(os.Stdout, "[flags] ", log.LstdFlags),
		configPath: cfg.ConfigPath,
		stopCh:     make(chan struct{}),
	}

	// Load built-in flags
	m.registerBuiltinFlags()

	// Load from config file if exists
	if cfg.ConfigPath != "" {
		if err := m.LoadFromFile(cfg.ConfigPath); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("load flags: %w", err)
		}

		// Start file watcher if enabled
		if cfg.Watch {
			m.startWatcher()
		}
	}

	return m, nil
}

// SetLogger sets the logger for the flag manager
func (m *Manager) SetLogger(logger *log.Logger) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logger = logger
}

// registerBuiltinFlags registers the built-in Drover feature flags
func (m *Manager) registerBuiltinFlags() {
	now := time.Now().Unix()

	// Parallel execution flag
	m.flags["parallel_execution_enabled"] = &Flag{
		ID:          "parallel_execution_enabled",
		Name:        "Parallel Execution",
		Description: "Enable parallel task execution across multiple workers",
		Type:        TypeBool,
		Default:     true,
		Value:       true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// Max concurrent workers
	m.flags["max_concurrent_workers"] = &Flag{
		ID:          "max_concurrent_workers",
		Name:        "Max Concurrent Workers",
		Description: "Maximum number of workers running concurrently",
		Type:        TypeInt,
		Default:     4,
		Value:       4,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// Kill switch for all workers
	m.flags["kill_switch_all_workers"] = &Flag{
		ID:          "kill_switch_all_workers",
		Name:        "Kill Switch All Workers",
		Description: "Emergency stop - prevents all workers from starting new tasks",
		Type:        TypeBool,
		Default:     false,
		Value:       false,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// Backpressure enabled
	m.flags["backpressure_enabled"] = &Flag{
		ID:          "backpressure_enabled",
		Name:        "Backpressure Enabled",
		Description: "Enable backpressure checks for task validation",
		Type:        TypeBool,
		Default:     true,
		Value:       true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// Worktree prewarming (with rollout)
	m.flags["worktree_prewarming"] = &Flag{
		ID:          "worktree_prewarming",
		Name:        "Worktree Prewarming",
		Description: "Enable worktree prewarming for faster task startup",
		Type:        TypeBool,
		Default:     false,
		Value:       false,
		Rollout: &FlagRollout{
			Enabled:    true,
			Percentage: 0, // Start at 0%, gradually increase
			Whitelist:  []string{},
			Blacklist:  []string{},
		},
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// FTS5 search enabled
	m.flags["fts5_search_enabled"] = &Flag{
		ID:          "fts5_search_enabled",
		Name:        "FTS5 Search Enabled",
		Description: "Enable full-text search across tasks and outputs",
		Type:        TypeBool,
		Default:     true,
		Value:       true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// Analytics enabled
	m.flags["analytics_enabled"] = &Flag{
		ID:          "analytics_enabled",
		Name:        "Analytics Enabled",
		Description: "Enable analytics and metrics collection",
		Type:        TypeBool,
		Default:     true,
		Value:       true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// Planning/building separation
	m.flags["planning_building_separation"] = &Flag{
		ID:          "planning_building_separation",
		Name:        "Planning/Building Separation",
		Description: "Enable two-phase execution (planning then building)",
		Type:        TypeBool,
		Default:     false,
		Value:       false,
		Rollout: &FlagRollout{
			Enabled:    true,
			Percentage: 0,
			Whitelist:  []string{},
			Blacklist:  []string{},
		},
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// LLM proxy mode
	m.flags["llm_proxy_mode"] = &Flag{
		ID:          "llm_proxy_mode",
		Name:        "LLM Proxy Mode",
		Description: "Route all LLM calls through the proxy server",
		Type:        TypeBool,
		Default:     false,
		Value:       false,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// Task timeout (seconds)
	m.flags["task_timeout_seconds"] = &Flag{
		ID:          "task_timeout_seconds",
		Name:        "Task Timeout",
		Description: "Maximum time a task can run before being terminated",
		Type:        TypeInt,
		Default:     3600, // 1 hour
		Value:       3600,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// GetBool returns a boolean flag value
func (m *Manager) GetBool(id string) bool {
	flag := m.get(id)
	if flag == nil || flag.Type != TypeBool {
		return false
	}

	val, ok := flag.Value.(bool)
	if !ok {
		return flag.Default.(bool)
	}
	return val
}

// GetBoolForProject returns a boolean flag value for a specific project
// Takes rollout configuration into account
func (m *Manager) GetBoolForProject(id, projectID string) bool {
	flag := m.get(id)
	if flag == nil || flag.Type != TypeBool {
		return false
	}

	// Check kill switch first - if blacklist contains project or explicitly false
	if flag.Rollout != nil {
		// Blacklist check
		for _, id := range flag.Rollout.Blacklist {
			if id == projectID {
				return false
			}
		}

		// Whitelist check
		for _, id := range flag.Rollout.Whitelist {
			if id == projectID {
				return true
			}
		}

		// Percentage rollout
		if flag.Rollout.Enabled && flag.Rollout.Percentage > 0 {
			// Use project ID hash for consistent bucketing
			hash := simpleHash(projectID)
			bucket := hash % 100
			return bucket < flag.Rollout.Percentage
		}
	}

	val, ok := flag.Value.(bool)
	if !ok {
		return flag.Default.(bool)
	}
	return val
}

// GetInt returns an integer flag value
func (m *Manager) GetInt(id string) int {
	flag := m.get(id)
	if flag == nil || flag.Type != TypeInt {
		return 0
	}

	val, ok := flag.Value.(int)
	if !ok {
		// Try float64 (JSON unmarshaling converts int to float64)
		if floatVal, ok := flag.Value.(float64); ok {
			return int(floatVal)
		}
		if intVal, ok := flag.Default.(int); ok {
			return intVal
		}
		return 0
	}
	return val
}

// GetFloat returns a float flag value
func (m *Manager) GetFloat(id string) float64 {
	flag := m.get(id)
	if flag == nil || flag.Type != TypeFloat {
		return 0
	}

	val, ok := flag.Value.(float64)
	if !ok {
		if floatVal, ok := flag.Default.(float64); ok {
			return floatVal
		}
		return 0
	}
	return val
}

// GetString returns a string flag value
func (m *Manager) GetString(id string) string {
	flag := m.get(id)
	if flag == nil || flag.Type != TypeString {
		return ""
	}

	val, ok := flag.Value.(string)
	if !ok {
		if strVal, ok := flag.Default.(string); ok {
			return strVal
		}
		return ""
	}
	return val
}

// Set sets a flag value
func (m *Manager) Set(id string, value interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	flag, exists := m.flags[id]
	if !exists {
		// Auto-create a simple bool flag for testing/convenience
		flag = &Flag{
			ID:        id,
			Name:      id,
			Type:      inferType(value),
			Default:   value,
			CreatedAt: time.Now().Unix(),
			UpdatedAt: time.Now().Unix(),
		}
		m.flags[id] = flag
	}

	// Type validation
	switch flag.Type {
	case TypeBool:
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("expected bool value for flag %s", id)
		}
	case TypeInt:
		if _, ok := value.(int); ok {
			// Valid
		} else if v, ok := value.(float64); ok {
			value = int(v)
		} else {
			return fmt.Errorf("expected int value for flag %s", id)
		}
	case TypeFloat:
		if _, ok := value.(float64); !ok {
			return fmt.Errorf("expected float value for flag %s", id)
		}
	case TypeString:
		if _, ok := value.(string); !ok {
			return fmt.Errorf("expected string value for flag %s", id)
		}
	}

	oldValue := flag.Value
	flag.Value = value
	flag.UpdatedAt = time.Now().Unix()

	// Notify watchers
	m.notifyWatchers(id, oldValue, value)

	m.logger.Printf("[flags] %s = %v", id, value)
	return nil
}

// inferType infers the flag type from a value
func inferType(value interface{}) FlagType {
	switch value.(type) {
	case bool:
		return TypeBool
	case int:
		return TypeInt
	case float64:
		return TypeFloat
	case string:
		return TypeString
	default:
		return TypeString
	}
}

// SetRolloutConfig sets the rollout configuration for a flag
func (m *Manager) SetRolloutConfig(id string, rollout *FlagRollout) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	flag, exists := m.flags[id]
	if !exists {
		return fmt.Errorf("flag %s not found", id)
	}

	flag.Rollout = rollout
	flag.UpdatedAt = time.Now().Unix()

	m.logger.Printf("[flags] %s rollout config updated: enabled=%v, percentage=%d",
		id, rollout.Enabled, rollout.Percentage)
	return nil
}

// GetRolloutConfig returns the rollout configuration for a flag
func (m *Manager) GetRolloutConfig(id string) (*FlagRollout, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	flag, exists := m.flags[id]
	if !exists {
		return nil, fmt.Errorf("flag %s not found", id)
	}

	return flag.Rollout, nil
}

// AddWhitelist adds a project ID to a flag's whitelist
func (m *Manager) AddWhitelist(flagID, projectID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	flag, exists := m.flags[flagID]
	if !exists {
		return fmt.Errorf("flag %s not found", flagID)
	}

	if flag.Rollout == nil {
		flag.Rollout = &FlagRollout{Enabled: true}
	}

	// Check if already in whitelist
	for _, id := range flag.Rollout.Whitelist {
		if id == projectID {
			return nil // Already present
		}
	}

	flag.Rollout.Whitelist = append(flag.Rollout.Whitelist, projectID)
	flag.UpdatedAt = time.Now().Unix()

	m.logger.Printf("[flags] %s: added %s to whitelist", flagID, projectID)
	return nil
}

// AddBlacklist adds a project ID to a flag's blacklist
func (m *Manager) AddBlacklist(flagID, projectID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	flag, exists := m.flags[flagID]
	if !exists {
		return fmt.Errorf("flag %s not found", flagID)
	}

	if flag.Rollout == nil {
		flag.Rollout = &FlagRollout{Enabled: true}
	}

	// Check if already in blacklist
	for _, id := range flag.Rollout.Blacklist {
		if id == projectID {
			return nil // Already present
		}
	}

	flag.Rollout.Blacklist = append(flag.Rollout.Blacklist, projectID)
	flag.UpdatedAt = time.Now().Unix()

	m.logger.Printf("[flags] %s: added %s to blacklist", flagID, projectID)
	return nil
}

// SetPercentage sets the rollout percentage for a flag
func (m *Manager) SetPercentage(flagID string, percentage int) error {
	if percentage < 0 || percentage > 100 {
		return fmt.Errorf("percentage must be between 0 and 100")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	flag, exists := m.flags[flagID]
	if !exists {
		return fmt.Errorf("flag %s not found", flagID)
	}

	if flag.Rollout == nil {
		flag.Rollout = &FlagRollout{Enabled: true}
	}

	flag.Rollout.Percentage = percentage
	flag.UpdatedAt = time.Now().Unix()

	m.logger.Printf("[flags] %s: rollout percentage set to %d%%", flagID, percentage)
	return nil
}

// get returns a flag by ID (without lock)
func (m *Manager) get(id string) *Flag {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.flags[id]
}

// Get returns a flag by ID (public accessor)
func (m *Manager) Get(id string) *Flag {
	return m.get(id)
}

// List returns all flags
func (m *Manager) List() []*Flag {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Flag, 0, len(m.flags))
	for _, flag := range m.flags {
		flagCopy := *flag
		result = append(result, &flagCopy)
	}
	return result
}

// Watch subscribes to flag changes
func (m *Manager) Watch(ctx context.Context) <-chan *FlagUpdate {
	ch := make(chan *FlagUpdate, 10)

	m.mu.Lock()
	m.watchers = append(m.watchers, ch)
	m.mu.Unlock()

	// Cleanup on context cancel
	go func() {
		<-ctx.Done()
		m.mu.Lock()
		for i, w := range m.watchers {
			if w == ch {
				m.watchers = append(m.watchers[:i], m.watchers[i+1:]...)
				break
			}
		}
		m.mu.Unlock()
		close(ch)
	}()

	return ch
}

// notifyWatchers sends update notifications to all watchers
func (m *Manager) notifyWatchers(flagID string, oldValue, newValue interface{}) {
	update := &FlagUpdate{
		FlagID:    flagID,
		OldValue:  oldValue,
		NewValue:  newValue,
		Timestamp: time.Now().Unix(),
	}

	for _, ch := range m.watchers {
		select {
		case ch <- update:
		default:
			// Channel full, skip
		}
	}
}

// LoadFromFile loads flags from a JSON file
func (m *Manager) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var flags []*Flag
	if err := json.Unmarshal(data, &flags); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, flag := range flags {
		// Always update flags from file (including built-ins)
		m.flags[flag.ID] = flag
	}

	m.logger.Printf("[flags] loaded %d flags from %s", len(flags), path)
	return nil
}

// SaveToFile saves flags to a JSON file
func (m *Manager) SaveToFile(path string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	flags := make([]*Flag, 0, len(m.flags))
	for _, flag := range m.flags {
		flagCopy := *flag
		flags = append(flags, &flagCopy)
	}

	data, err := json.MarshalIndent(flags, "", "  ")
	if err != nil {
		return err
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// startWatcher starts the file watcher for hot reload
func (m *Manager) startWatcher() {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		var lastMod time.Time

		for {
			select {
			case <-ticker.C:
				info, err := os.Stat(m.configPath)
				if err != nil {
					continue
				}

				if info.ModTime().After(lastMod) {
					lastMod = info.ModTime()
					if err := m.LoadFromFile(m.configPath); err != nil {
						m.logger.Printf("[flags] reload error: %v", err)
					} else {
						m.logger.Printf("[flags] reloaded from %s", m.configPath)
					}
				}
			case <-m.stopCh:
				return
			}
		}
	}()
}

// Stop stops the flag manager
func (m *Manager) Stop() {
	close(m.stopCh)
	m.wg.Wait()
}

// builtinFlags tracks built-in flag IDs
var builtinFlags = map[string]bool{
	"parallel_execution_enabled":      true,
	"max_concurrent_workers":          true,
	"kill_switch_all_workers":         true,
	"backpressure_enabled":            true,
	"worktree_prewarming":             true,
	"fts5_search_enabled":             true,
	"analytics_enabled":               true,
	"planning_building_separation":    true,
	"llm_proxy_mode":                  true,
	"task_timeout_seconds":            true,
}

// simpleHash creates a simple hash from a string for consistent bucketing
func simpleHash(s string) int {
	hash := 0
	for _, c := range s {
		hash = ((hash << 5) - hash) + int(c)
		hash = hash & hash // Keep within int range
	}
	if hash < 0 {
		hash = -hash
	}
	return hash
}
