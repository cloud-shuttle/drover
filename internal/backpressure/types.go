// Package backpressure provides quality validation and backpressure mechanisms
package backpressure

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

// CheckType defines the type of validation check
type CheckType string

const (
	CheckTypeCommand CheckType = "command" // Run shell command
	CheckTypeLLM     CheckType = "llm"     // LLM-based validation
	CheckTypeFile    CheckType = "file"    // File existence check
	CheckTypeRegex   CheckType = "regex"   // Regex pattern matching
)

// Check defines a validation check
type Check struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Type        CheckType              `json:"type"`
	Description string                 `json:"description"`
	Enabled     bool                   `json:"enabled"`
	Config      map[string]interface{} `json:"config"`
	CreatedAt   int64                  `json:"created_at"`
	UpdatedAt   int64                  `json:"updated_at"`
}

// CheckResult is the result of a validation check
type CheckResult struct {
	CheckID    string        `json:"check_id"`
	Passed     bool          `json:"passed"`
	Message    string        `json:"message"`
	Duration   time.Duration `json:"duration"`
	Timestamp  int64         `json:"timestamp"`
	Metadata   interface{}   `json:"metadata,omitempty"`
}

// Rule defines a backpressure rule
type Rule struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Enabled     bool     `json:"enabled"`
	CheckIDs    []string `json:"check_ids"` // Checks to run
	Mode        string   `json:"mode"`      // "all" (all must pass) or "any" (one must pass)
	CreatedAt   int64    `json:"created_at"`
	UpdatedAt   int64    `json:"updated_at"`
}

// RuleResult is the result of a rule evaluation
type RuleResult struct {
	RuleID     string         `json:"rule_id"`
	Passed     bool           `json:"passed"`
	CheckResults []*CheckResult `json:"check_results"`
	Timestamp  int64          `json:"timestamp"`
}

// Manager manages backpressure checks and rules
type Manager struct {
	mu       sync.RWMutex
	checks   map[string]*Check
	rules    map[string]*Rule
	logger   *log.Logger
	configPath string
}

// Config holds manager configuration
type Config struct {
	ConfigPath string
	Logger     *log.Logger
}

// NewManager creates a new backpressure manager
func NewManager(cfg Config) (*Manager, error) {
	m := &Manager{
		checks:     make(map[string]*Check),
		rules:      make(map[string]*Rule),
		logger:     cfg.Logger,
		configPath: cfg.ConfigPath,
	}

	if m.logger == nil {
		m.logger = log.New(os.Stdout, "[backpressure] ", log.LstdFlags)
	}

	// Register built-in checks
	m.registerBuiltinChecks()

	// Load from config if exists
	if cfg.ConfigPath != "" {
		if err := m.LoadFromFile(cfg.ConfigPath); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("load backpressure config: %w", err)
		}
	}

	return m, nil
}

// SetLogger sets the logger for the manager
func (m *Manager) SetLogger(logger *log.Logger) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logger = logger
}

// registerBuiltinChecks registers built-in validation checks
func (m *Manager) registerBuiltinChecks() {
	now := time.Now().Unix()

	// Command check: Verify git status is clean
	m.checks["git_clean"] = &Check{
		ID:          "git_clean",
		Name:        "Git Working Directory Clean",
		Type:        CheckTypeCommand,
		Description: "Ensure git working directory is clean before running tasks",
		Enabled:     true,
		Config: map[string]interface{}{
			"command": "git",
			"args":    []string{"diff", "--quiet"},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	// File check: Verify go.mod exists
	m.checks["go_mod_exists"] = &Check{
		ID:          "go_mod_exists",
		Name:        "Go Module File Exists",
		Type:        CheckTypeFile,
		Description: "Ensure go.mod exists in the project root",
		Enabled:     true,
		Config: map[string]interface{}{
			"paths": []string{"go.mod"},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Regex check: Validate no TODO comments in new code
	m.checks["no_todo_comments"] = &Check{
		ID:          "no_todo_comments",
		Name:        "No TODO Comments",
		Type:        CheckTypeRegex,
		Description: "Ensure no TODO or FIXME comments in output",
		Enabled:     false, // Disabled by default
		Config: map[string]interface{}{
			"patterns": []string{`(?i)TODO:`, `(?i)FIXME:`},
			"invert":   true, // Invert: fail if pattern matches
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	// LLM check: Code quality validation
	m.checks["llm_quality"] = &Check{
		ID:          "llm_quality",
		Name:        "LLM Quality Check",
		Type:        CheckTypeLLM,
		Description: "Use LLM to validate code quality",
		Enabled:     false, // Requires API key
		Config: map[string]interface{}{
			"prompt": "Evaluate the following code for quality, security, and best practices. Pass only if the code is production-ready.",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Register built-in rule
	m.rules["default"] = &Rule{
		ID:          "default",
		Name:        "Default Validation",
		Description: "Default backpressure rule for all tasks",
		Enabled:     true,
		CheckIDs:    []string{"git_clean", "go_mod_exists"},
		Mode:        "all",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// RegisterCheck registers a new validation check
func (m *Manager) RegisterCheck(check *Check) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if check.ID == "" {
		return fmt.Errorf("check ID is required")
	}

	check.CreatedAt = time.Now().Unix()
	check.UpdatedAt = check.CreatedAt
	m.checks[check.ID] = check

	m.logger.Printf("[backpressure] registered check: %s", check.ID)
	return nil
}

// GetCheck retrieves a check by ID
func (m *Manager) GetCheck(id string) (*Check, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	check, ok := m.checks[id]
	return check, ok
}

// ListChecks returns all checks
func (m *Manager) ListChecks() []*Check {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Check, 0, len(m.checks))
	for _, check := range m.checks {
		checkCopy := *check
		result = append(result, &checkCopy)
	}
	return result
}

// EnableCheck enables a check
func (m *Manager) EnableCheck(id string, enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	check, ok := m.checks[id]
	if !ok {
		return fmt.Errorf("check %s not found", id)
	}

	checkCopy := *check // Make a copy to avoid modifying the original pointer in the map
	checkCopy.Enabled = enabled
	checkCopy.UpdatedAt = time.Now().Unix()
	m.checks[id] = &checkCopy

	m.logger.Printf("[backpressure] check %s: enabled=%v", id, enabled)
	return nil
}

// CreateRule creates a new rule
func (m *Manager) CreateRule(rule *Rule) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if rule.ID == "" {
		return fmt.Errorf("rule ID is required")
	}

	// Validate all checks exist
	for _, checkID := range rule.CheckIDs {
		if _, ok := m.checks[checkID]; !ok {
			return fmt.Errorf("check %s not found", checkID)
		}
	}

	if rule.Mode != "all" && rule.Mode != "any" {
		return fmt.Errorf("mode must be 'all' or 'any'")
	}

	rule.CreatedAt = time.Now().Unix()
	rule.UpdatedAt = rule.CreatedAt
	m.rules[rule.ID] = rule

	m.logger.Printf("[backpressure] created rule: %s with %d checks", rule.ID, len(rule.CheckIDs))
	return nil
}

// GetRule retrieves a rule by ID
func (m *Manager) GetRule(id string) (*Rule, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rule, ok := m.rules[id]
	return rule, ok
}

// ListRules returns all rules
func (m *Manager) ListRules() []*Rule {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Rule, 0, len(m.rules))
	for _, rule := range m.rules {
		ruleCopy := *rule
		result = append(result, &ruleCopy)
	}
	return result
}

// EnableRule enables or disables a rule
func (m *Manager) EnableRule(id string, enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	rule, ok := m.rules[id]
	if !ok {
		return fmt.Errorf("rule %s not found", id)
	}

	rule.Enabled = enabled
	rule.UpdatedAt = time.Now().Unix()

	m.logger.Printf("[backpressure] rule %s: enabled=%v", id, enabled)
	return nil
}

// Validate runs all enabled checks in a rule and returns the result
func (m *Manager) Validate(ctx context.Context, ruleID string, input *ValidateInput) (*RuleResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rule, ok := m.rules[ruleID]
	if !ok {
		return nil, fmt.Errorf("rule %s not found", ruleID)
	}

	if !rule.Enabled {
		// Rule disabled, automatically pass
		return &RuleResult{
			RuleID:    ruleID,
			Passed:    true,
			CheckResults: []*CheckResult{},
			Timestamp: time.Now().Unix(),
		}, nil
	}

	results := make([]*CheckResult, 0, len(rule.CheckIDs))

	for _, checkID := range rule.CheckIDs {
		check, ok := m.checks[checkID]
		if !ok || !check.Enabled {
			continue
		}

		result, err := m.runCheck(ctx, check, input)
		if err != nil {
			m.logger.Printf("[backpressure] check %s error: %v", checkID, err)
			result = &CheckResult{
				CheckID:   checkID,
				Passed:    false,
				Message:   fmt.Sprintf("Check error: %v", err),
				Timestamp: time.Now().Unix(),
			}
		}

		results = append(results, result)

		// Early exit for "any" mode if one passed
		if rule.Mode == "any" && result.Passed {
			break
		}

		// Early exit for "all" mode if one failed
		if rule.Mode == "all" && !result.Passed {
			break
		}
	}

	// Determine final result
	var passed bool
	if rule.Mode == "all" {
		passed = true
		for _, r := range results {
			if !r.Passed {
				passed = false
				break
			}
		}
	} else { // any
		passed = false
		for _, r := range results {
			if r.Passed {
				passed = true
				break
			}
		}
	}

	return &RuleResult{
		RuleID:       ruleID,
		Passed:       passed,
		CheckResults: results,
		Timestamp:    time.Now().Unix(),
	}, nil
}

// ValidateInput is input for validation
type ValidateInput struct {
	TaskID      string            `json:"task_id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Output      string            `json:"output"`
	ProjectDir  string            `json:"project_dir"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// runCheck executes a single validation check
func (m *Manager) runCheck(ctx context.Context, check *Check, input *ValidateInput) (*CheckResult, error) {
	start := time.Now()
	result := &CheckResult{
		CheckID:   check.ID,
		Timestamp: time.Now().Unix(),
	}

	var err error
	switch check.Type {
	case CheckTypeCommand:
		err = m.runCommandCheck(ctx, check, input, result)
	case CheckTypeLLM:
		err = m.runLLMCheck(ctx, check, input, result)
	case CheckTypeFile:
		err = m.runFileCheck(ctx, check, input, result)
	case CheckTypeRegex:
		err = m.runRegexCheck(ctx, check, input, result)
	default:
		return nil, fmt.Errorf("unknown check type: %s", check.Type)
	}

	if err != nil {
		return nil, err
	}

	result.Duration = time.Since(start)
	return result, nil
}

// LoadFromFile loads checks and rules from a JSON file
func (m *Manager) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var config struct {
		Checks []*Check `json:"checks"`
		Rules  []*Rule  `json:"rules"`
	}

	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Load checks
	for _, check := range config.Checks {
		m.checks[check.ID] = check
	}

	// Load rules
	for _, rule := range config.Rules {
		m.rules[rule.ID] = rule
	}

	m.logger.Printf("[backpressure] loaded %d checks and %d rules from %s",
		len(config.Checks), len(config.Rules), path)
	return nil
}

// SaveToFile saves checks and rules to a JSON file
func (m *Manager) SaveToFile(path string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	config := struct {
		Checks []*Check `json:"checks"`
		Rules  []*Rule  `json:"rules"`
	}{
		Checks: make([]*Check, 0, len(m.checks)),
		Rules:  make([]*Rule, 0, len(m.rules)),
	}

	for _, check := range m.checks {
		checkCopy := *check
		config.Checks = append(config.Checks, &checkCopy)
	}

	for _, rule := range m.rules {
		ruleCopy := *rule
		config.Rules = append(config.Rules, &ruleCopy)
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}
