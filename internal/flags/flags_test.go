package flags

import (
	"context"
	"testing"
	"time"
)

// TestBuiltinFlags tests that all built-in flags are registered
func TestBuiltinFlags(t *testing.T) {
	m, err := NewManager(Config{})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Check that all builtin flags exist
	for flagID := range builtinFlags {
		if m.flags[flagID] == nil {
			t.Errorf("Builtin flag %s not registered", flagID)
		}
	}
}

// TestGetBool tests boolean flag retrieval
func TestGetBool(t *testing.T) {
	m, _ := NewManager(Config{})

	// Test default true value
	if !m.GetBool("parallel_execution_enabled") {
		t.Error("Expected parallel_execution_enabled to be true by default")
	}

	// Test default false value
	if m.GetBool("kill_switch_all_workers") {
		t.Error("Expected kill_switch_all_workers to be false by default")
	}

	// Test unknown flag
	if m.GetBool("unknown_flag") {
		t.Error("Expected unknown flag to return false")
	}
}

// TestGetInt tests integer flag retrieval
func TestGetInt(t *testing.T) {
	m, _ := NewManager(Config{})

	val := m.GetInt("max_concurrent_workers")
	if val != 4 {
		t.Errorf("Expected max_concurrent_workers = 4, got %d", val)
	}

	val = m.GetInt("task_timeout_seconds")
	if val != 3600 {
		t.Errorf("Expected task_timeout_seconds = 3600, got %d", val)
	}
}

// TestSet tests setting flag values
func TestSet(t *testing.T) {
	m, _ := NewManager(Config{})

	// Set a boolean flag
	if err := m.Set("kill_switch_all_workers", true); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	if !m.GetBool("kill_switch_all_workers") {
		t.Error("Expected kill_switch_all_workers to be true after Set")
	}

	// Set an integer flag
	if err := m.Set("max_concurrent_workers", 8); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	if m.GetInt("max_concurrent_workers") != 8 {
		t.Error("Expected max_concurrent_workers to be 8 after Set")
	}
}

// TestSetTypeValidation tests type validation when setting flags
func TestSetTypeValidation(t *testing.T) {
	m, _ := NewManager(Config{})

	// Try to set bool flag with wrong type
	if err := m.Set("parallel_execution_enabled", "not a bool"); err == nil {
		t.Error("Expected error when setting bool flag with string value")
	}

	// Try to set int flag with wrong type
	if err := m.Set("max_concurrent_workers", "not an int"); err == nil {
		t.Error("Expected error when setting int flag with string value")
	}
}

// TestGetBoolForProject tests project-specific flag evaluation
func TestGetBoolForProject(t *testing.T) {
	m, _ := NewManager(Config{})

	// Test with flag disabled
	m.Set("test_flag", false)
	if m.GetBoolForProject("test_flag", "project-1") {
		t.Error("Expected flag to be false for project-1 when disabled")
	}

	// Test with whitelist
	m.Set("test_flag", true)
	m.AddWhitelist("test_flag", "project-1")
	if !m.GetBoolForProject("test_flag", "project-1") {
		t.Error("Expected flag to be true for whitelisted project-1")
	}

	// Test with blacklist
	m.AddBlacklist("test_flag", "project-2")
	if m.GetBoolForProject("test_flag", "project-2") {
		t.Error("Expected flag to be false for blacklisted project-2")
	}

	// Test with percentage rollout
	m.SetPercentage("test_flag", 50) // 50% rollout
	// Project-1 should be whitelisted, so still true
	if !m.GetBoolForProject("test_flag", "project-1") {
		t.Error("Expected whitelisted project to bypass percentage")
	}
}

// TestSetPercentage tests percentage rollout configuration
func TestSetPercentage(t *testing.T) {
	m, _ := NewManager(Config{})

	// Valid percentages
	for _, pct := range []int{0, 25, 50, 75, 100} {
		if err := m.SetPercentage("worktree_prewarming", pct); err != nil {
			t.Errorf("SetPercentage(%d) failed: %v", pct, err)
		}
	}

	// Invalid percentages
	for _, pct := range []int{-1, 101, 200} {
		if err := m.SetPercentage("worktree_prewarming", pct); err == nil {
			t.Errorf("SetPercentage(%d) should have failed", pct)
		}
	}
}

// TestWatch tests flag change notifications
func TestWatch(t *testing.T) {
	m, _ := NewManager(Config{})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch := m.Watch(ctx)

	// Set a flag value
	go func() {
		time.Sleep(100 * time.Millisecond)
		m.Set("kill_switch_all_workers", true)
	}()

	// Wait for notification
	select {
	case update := <-ch:
		if update.FlagID != "kill_switch_all_workers" {
			t.Errorf("Expected flag ID 'kill_switch_all_workers', got %s", update.FlagID)
		}
		if update.NewValue != true {
			t.Errorf("Expected NewValue true, got %v", update.NewValue)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for flag update notification")
	}
}

// TestSaveAndLoad tests persistence
func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := tmpDir + "/flags.json"

	// Create and save flags
	m1, _ := NewManager(Config{})
	m1.Set("kill_switch_all_workers", true)
	m1.Set("max_concurrent_workers", 8)

	if err := m1.SaveToFile(configPath); err != nil {
		t.Fatalf("SaveToFile failed: %v", err)
	}

	// Load into new manager
	m2, _ := NewManager(Config{})
	if err := m2.LoadFromFile(configPath); err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	// Verify values
	if !m2.GetBool("kill_switch_all_workers") {
		t.Error("Expected loaded flag to be true")
	}

	if m2.GetInt("max_concurrent_workers") != 8 {
		t.Error("Expected loaded flag to be 8")
	}
}

// TestList tests listing all flags
func TestList(t *testing.T) {
	m, _ := NewManager(Config{})

	flags := m.List()
	if len(flags) != len(builtinFlags) {
		t.Errorf("Expected %d flags, got %d", len(builtinFlags), len(flags))
	}
}

// TestConsistentHashing tests that project IDs hash consistently
func TestConsistentHashing(t *testing.T) {
	projectID := "test-project-123"

	hash1 := simpleHash(projectID)
	hash2 := simpleHash(projectID)

	if hash1 != hash2 {
		t.Error("Hash should be consistent for same input")
	}

	// Different projects should (usually) have different hashes
	otherProject := "other-project-456"
	otherHash := simpleHash(otherProject)

	// Note: collisions are possible but unlikely for simple strings
	if hash1 == otherHash && hash1 != 0 {
		t.Log("Note: Hash collision detected (unlikely but possible)")
	}
}

// TestRolloutConfig tests rollout configuration CRUD
func TestRolloutConfig(t *testing.T) {
	m, _ := NewManager(Config{})

	// Get initial config
	config, err := m.GetRolloutConfig("worktree_prewarming")
	if err != nil {
		t.Fatalf("GetRolloutConfig failed: %v", err)
	}

	if config == nil {
		t.Error("Expected rollout config to exist")
	}

	// Modify and verify
	m.SetPercentage("worktree_prewarming", 75)
	config, _ = m.GetRolloutConfig("worktree_prewarming")

	if config.Percentage != 75 {
		t.Errorf("Expected percentage 75, got %d", config.Percentage)
	}

	// Test whitelist/blacklist
	m.AddWhitelist("worktree_prewarming", "project-a")
	m.AddBlacklist("worktree_prewarming", "project-b")

	config, _ = m.GetRolloutConfig("worktree_prewarming")

	hasWhitelist := false
	hasBlacklist := false
	for _, id := range config.Whitelist {
		if id == "project-a" {
			hasWhitelist = true
		}
	}
	for _, id := range config.Blacklist {
		if id == "project-b" {
			hasBlacklist = true
		}
	}

	if !hasWhitelist {
		t.Error("Expected project-a in whitelist")
	}
	if !hasBlacklist {
		t.Error("Expected project-b in blacklist")
	}
}
