package backpressure

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	m, err := NewManager(Config{})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Check built-in checks exist
	checks := m.ListChecks()
	if len(checks) < 4 {
		t.Errorf("Expected at least 4 built-in checks, got %d", len(checks))
	}

	// Check built-in rule exists
	rules := m.ListRules()
	if len(rules) < 1 {
		t.Errorf("Expected at least 1 built-in rule, got %d", len(rules))
	}
}

func TestRegisterCheck(t *testing.T) {
	m, err := NewManager(Config{})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	check := &Check{
		ID:          "test-check",
		Name:        "Test Check",
		Type:        CheckTypeFile,
		Description: "Test description",
		Enabled:     true,
		Config: map[string]interface{}{
			"paths": []string{"test.txt"},
		},
	}

	if err := m.RegisterCheck(check); err != nil {
		t.Fatalf("RegisterCheck failed: %v", err)
	}

	retrieved, ok := m.GetCheck("test-check")
	if !ok {
		t.Fatal("Check not found after registration")
	}

	if retrieved.Name != "Test Check" {
		t.Errorf("Expected name 'Test Check', got '%s'", retrieved.Name)
	}
}

func TestRegisterCheckEmptyID(t *testing.T) {
	m, err := NewManager(Config{})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	check := &Check{
		ID:   "",
		Name: "Test Check",
		Type: CheckTypeFile,
	}

	if err := m.RegisterCheck(check); err == nil {
		t.Error("Expected error for empty ID, got nil")
	}
}

func TestEnableCheck(t *testing.T) {
	m, err := NewManager(Config{})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Disable git_clean check
	if err := m.EnableCheck("git_clean", false); err != nil {
		t.Fatalf("EnableCheck failed: %v", err)
	}

	check, ok := m.GetCheck("git_clean")
	if !ok {
		t.Fatal("Check not found")
	}

	if check.Enabled {
		t.Error("Expected check to be disabled")
	}

	// Re-enable
	if err := m.EnableCheck("git_clean", true); err != nil {
		t.Fatalf("EnableCheck failed: %v", err)
	}

	check, ok = m.GetCheck("git_clean")
	if !ok {
		t.Fatal("Check not found")
	}

	if !check.Enabled {
		t.Error("Expected check to be enabled")
	}
}

func TestCreateRule(t *testing.T) {
	m, err := NewManager(Config{})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	rule := &Rule{
		ID:          "test-rule",
		Name:        "Test Rule",
		Description: "Test description",
		Enabled:     true,
		CheckIDs:    []string{"git_clean"},
		Mode:        "all",
	}

	if err := m.CreateRule(rule); err != nil {
		t.Fatalf("CreateRule failed: %v", err)
	}

	retrieved, ok := m.GetRule("test-rule")
	if !ok {
		t.Fatal("Rule not found after creation")
	}

	if retrieved.Name != "Test Rule" {
		t.Errorf("Expected name 'Test Rule', got '%s'", retrieved.Name)
	}
}

func TestCreateRuleInvalidMode(t *testing.T) {
	m, err := NewManager(Config{})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	rule := &Rule{
		ID:       "test-rule",
		CheckIDs: []string{"git_clean"},
		Mode:     "invalid",
	}

	if err := m.CreateRule(rule); err == nil {
		t.Error("Expected error for invalid mode, got nil")
	}
}

func TestCreateRuleInvalidCheck(t *testing.T) {
	m, err := NewManager(Config{})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	rule := &Rule{
		ID:       "test-rule",
		CheckIDs: []string{"nonexistent-check"},
		Mode:     "all",
	}

	if err := m.CreateRule(rule); err == nil {
		t.Error("Expected error for nonexistent check, got nil")
	}
}

func TestValidateWithFileCheck(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	m, err := NewManager(Config{})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Create a rule with file check
	m.CreateRule(&Rule{
		ID:          "file-test",
		Name:        "File Test",
		CheckIDs:    []string{"go_mod_exists"},
		Mode:        "all",
		Enabled:     true,
	})

	// The check should fail because test.txt exists but we're checking for go.mod
	input := &ValidateInput{
		TaskID:     "task-1",
		Title:      "Test Task",
		ProjectDir: tmpDir,
	}

	result, err := m.Validate(context.Background(), "file-test", input)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	if result.Passed {
		t.Error("Expected validation to fail (go.mod doesn't exist)")
	}

	// Now create go.mod
	goMod := filepath.Join(tmpDir, "go.mod")
	if err := os.WriteFile(goMod, []byte("module test"), 0644); err != nil {
		t.Fatalf("Failed to create go.mod: %v", err)
	}

	result, err = m.Validate(context.Background(), "file-test", input)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	if !result.Passed {
		t.Errorf("Expected validation to pass (go.mod exists), got message: %s", result.CheckResults[0].Message)
	}
}

func TestValidateWithRegexCheck(t *testing.T) {
	m, err := NewManager(Config{})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Create a regex check for TODO comments
	check := &Check{
		ID:   "no-todo",
		Name: "No TODO",
		Type: CheckTypeRegex,
		Config: map[string]interface{}{
			"patterns": []string{`(?i)TODO:`},
			"invert":   true,
		},
		Enabled: true,
	}

	if err := m.RegisterCheck(check); err != nil {
		t.Fatalf("RegisterCheck failed: %v", err)
	}

	m.CreateRule(&Rule{
		ID:          "regex-test",
		Name:        "Regex Test",
		CheckIDs:    []string{"no-todo"},
		Mode:        "all",
		Enabled:     true,
	})

	// Test with TODO in output (should fail)
	input := &ValidateInput{
		TaskID: "task-1",
		Title:  "Test Task",
		Output: "Some code with TODO: fix this later",
	}

	result, err := m.Validate(context.Background(), "regex-test", input)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	if result.Passed {
		t.Error("Expected validation to fail (output contains TODO)")
	}

	// Test without TODO (should pass)
	input.Output = "Some clean code"
	result, err = m.Validate(context.Background(), "regex-test", input)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	if !result.Passed {
		t.Error("Expected validation to pass (output doesn't contain TODO)")
	}
}

func TestValidateRuleModeAny(t *testing.T) {
	tmpDir := t.TempDir()

	// Create go.mod
	goMod := filepath.Join(tmpDir, "go.mod")
	if err := os.WriteFile(goMod, []byte("module test"), 0644); err != nil {
		t.Fatalf("Failed to create go.mod: %v", err)
	}

	m, err := NewManager(Config{})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Create a rule with multiple checks in "any" mode
	m.CreateRule(&Rule{
		ID:          "any-test",
		Name:        "Any Test",
		CheckIDs:    []string{"go_mod_exists", "git_clean"},
		Mode:        "any",
		Enabled:     true,
	})

	input := &ValidateInput{
		TaskID:     "task-1",
		Title:      "Test Task",
		ProjectDir: tmpDir,
	}

	result, err := m.Validate(context.Background(), "any-test", input)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	// Should pass because go_mod_exists passes (any mode)
	if !result.Passed {
		t.Error("Expected validation to pass (at least one check should pass in 'any' mode)")
	}
}

func TestValidateDisabledRule(t *testing.T) {
	m, err := NewManager(Config{})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Disable the default rule
	if err := m.EnableRule("default", false); err != nil {
		t.Fatalf("EnableRule failed: %v", err)
	}

	input := &ValidateInput{
		TaskID: "task-1",
		Title:  "Test Task",
	}

	result, err := m.Validate(context.Background(), "default", input)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	if !result.Passed {
		t.Error("Disabled rule should auto-pass")
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "backpressure.json")

	m1, err := NewManager(Config{ConfigPath: configPath})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Add a custom check
	check := &Check{
		ID:          "custom-check",
		Name:        "Custom Check",
		Type:        CheckTypeRegex,
		Description: "Custom validation",
		Enabled:     true,
		Config: map[string]interface{}{
			"patterns": []string{"test"},
		},
	}
	if err := m1.RegisterCheck(check); err != nil {
		t.Fatalf("RegisterCheck failed: %v", err)
	}

	// Save to file
	if err := m1.SaveToFile(configPath); err != nil {
		t.Fatalf("SaveToFile failed: %v", err)
	}

	// Load into a new manager
	m2, err := NewManager(Config{ConfigPath: configPath})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Verify the custom check was loaded
	retrieved, ok := m2.GetCheck("custom-check")
	if !ok {
		t.Fatal("Custom check not found after load")
	}

	if retrieved.Name != "Custom Check" {
		t.Errorf("Expected name 'Custom Check', got '%s'", retrieved.Name)
	}
}

func TestValidateWithCommandCheck(t *testing.T) {
	m, err := NewManager(Config{})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Create a command check that should succeed (echo)
	check := &Check{
		ID:   "echo-test",
		Name: "Echo Test",
		Type: CheckTypeCommand,
		Config: map[string]interface{}{
			"command": "echo",
			"args":    []string{"hello"},
		},
		Enabled: true,
	}

	if err := m.RegisterCheck(check); err != nil {
		t.Fatalf("RegisterCheck failed: %v", err)
	}

	m.CreateRule(&Rule{
		ID:          "cmd-test",
		Name:        "Command Test",
		CheckIDs:    []string{"echo-test"},
		Mode:        "all",
		Enabled:     true,
	})

	input := &ValidateInput{
		TaskID: "task-1",
		Title:  "Test Task",
	}

	result, err := m.Validate(context.Background(), "cmd-test", input)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	if !result.Passed {
		t.Errorf("Expected validation to pass (echo command succeeds), got message: %s", result.CheckResults[0].Message)
	}
}

func TestCheckTimestamps(t *testing.T) {
	m, err := NewManager(Config{})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	check := &Check{
		ID:          "timestamp-test",
		Name:        "Timestamp Test",
		Type:        CheckTypeFile,
		Description: "Test timestamps",
		Enabled:     true,
		Config:      map[string]interface{}{},
	}

	beforeReg := time.Now().Unix()
	if err := m.RegisterCheck(check); err != nil {
		t.Fatalf("RegisterCheck failed: %v", err)
	}

	retrieved, _ := m.GetCheck("timestamp-test")
	if retrieved.CreatedAt < beforeReg {
		t.Error("CreatedAt not set correctly")
	}
	if retrieved.CreatedAt != retrieved.UpdatedAt {
		t.Error("CreatedAt and UpdatedAt should be equal after registration")
	}

	// EnableCheck should update UpdatedAt (make it at least as recent)
	if err := m.EnableCheck("timestamp-test", false); err != nil {
		t.Fatalf("EnableCheck failed: %v", err)
	}

	// Re-fetch to get the updated entry
	retrieved, _ = m.GetCheck("timestamp-test")
	if retrieved.UpdatedAt < retrieved.CreatedAt {
		t.Error("UpdatedAt should not be before CreatedAt")
	}
}
