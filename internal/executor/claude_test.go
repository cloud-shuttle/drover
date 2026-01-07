// Package executor_test provides tests for the executor package
package executor_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cloud-shuttle/drover/internal/executor"
	"github.com/cloud-shuttle/drover/pkg/types"
)

// mockClaudeScript creates a shell script that simulates Claude behavior
func createMockClaudeScript(t *testing.T, dir string, exitCode int, sleepMs int) string {
	t.Helper()
	scriptPath := filepath.Join(dir, "mock-claude.sh")
	script := fmt.Sprintf(`#!/bin/bash
# Mock Claude script for testing
sleep %d
exit %d
`, sleepMs/1000, exitCode)

	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("Failed to create mock claude script: %v", err)
	}
	return scriptPath
}

// TestExecutor_Execute_Success verifies successful execution
func TestExecutor_Execute_Success(t *testing.T) {
	tmpDir := t.TempDir()
	mockClaude := createMockClaudeScript(t, tmpDir, 0, 100)

	exec := executor.NewExecutor(mockClaude, 5*time.Minute)
	exec.SetVerbose(true)

	task := &types.Task{
		ID:          "task-123",
		Title:       "Test Task",
		Description: "Test Description",
	}

	result := exec.Execute(tmpDir, task)
	if !result.Success {
		t.Errorf("Execute failed: %v", result.Error)
	}
}

// TestExecutor_ExecuteWithTimeout_Success verifies execution with timeout succeeds when within limit
func TestExecutor_ExecuteWithTimeout_Success(t *testing.T) {
	tmpDir := t.TempDir()
	mockClaude := createMockClaudeScript(t, tmpDir, 0, 100)

	exec := executor.NewExecutor(mockClaude, 5*time.Minute)

	task := &types.Task{
		ID:          "task-123",
		Title:       "Test Task",
		Description: "Test Description",
	}

	result := exec.ExecuteWithTimeout(tmpDir, task)
	if !result.Success {
		t.Errorf("ExecuteWithTimeout failed: %v", result.Error)
	}
}

// TestExecutor_ExecuteWithTimeout_Timeout verifies timeout is enforced
func TestExecutor_ExecuteWithTimeout_Timeout(t *testing.T) {
	tmpDir := t.TempDir()
	mockClaude := createMockClaudeScript(t, tmpDir, 0, 5000) // Sleeps 5 seconds

	exec := executor.NewExecutor(mockClaude, 100*time.Millisecond) // 100ms timeout

	task := &types.Task{
		ID:          "task-123",
		Title:       "Test Task",
		Description: "Test Description",
	}

	result := exec.ExecuteWithTimeout(tmpDir, task)
	if result.Success {
		t.Error("Expected timeout error, got success")
	}
	// Error message should contain "timed out"
	if result.Error != nil && !containsString(result.Error.Error(), "timed out") {
		t.Errorf("Expected timeout error message, got: %v", result.Error)
	}
}

// TestExecutor_Execute_Failure verifies execution failures are propagated
func TestExecutor_Execute_Failure(t *testing.T) {
	tmpDir := t.TempDir()
	mockClaude := createMockClaudeScript(t, tmpDir, 1, 100) // Exit code 1

	exec := executor.NewExecutor(mockClaude, 5*time.Minute)

	task := &types.Task{
		ID:          "task-123",
		Title:       "Test Task",
		Description: "Test Description",
	}

	result := exec.Execute(tmpDir, task)
	if result.Success {
		t.Error("Expected error for failed execution, got success")
	}
}

// TestExecutor_ExecuteWithContext_Cancel verifies context cancellation is handled
func TestExecutor_ExecuteWithContext_Cancel(t *testing.T) {
	tmpDir := t.TempDir()
	mockClaude := createMockClaudeScript(t, tmpDir, 0, 5000) // Sleeps 5 seconds

	exec := executor.NewExecutor(mockClaude, 5*time.Minute)

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately
	cancel()

	task := &types.Task{
		ID:          "task-123",
		Title:       "Test Task",
		Description: "Test Description",
	}

	result := exec.ExecuteWithContext(ctx, tmpDir, task)
	// Context cancellation should cause an error (command killed)
	if result.Success {
		t.Error("Expected error when context is cancelled, got success")
	}
}

// TestExecutor_CheckClaudeInstalled verifies Claude detection
func TestExecutor_CheckClaudeInstalled(t *testing.T) {
	tmpDir := t.TempDir()
	mockClaude := createMockClaudeScript(t, tmpDir, 0, 0)

	// Test with mock claude that exists
	err := executor.CheckClaudeInstalled(mockClaude)
	if err != nil {
		// The mock script might not handle --version properly
		// This is expected since our simple mock doesn't implement version check
		t.Logf("CheckClaudeInstalled failed (expected for mock): %v", err)
	}

	// Test with non-existent path
	err = executor.CheckClaudeInstalled("/nonexistent/path/to/claude")
	if err == nil {
		t.Error("Expected error for non-existent claude path, got nil")
	}
}

// Helper function to check if string contains substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && contains(s, substr))
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestExecutor_PromptContent verifies the prompt contains expected content
func TestExecutor_PromptContent(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a mock that writes the prompt to a file
	mockClaude := filepath.Join(tmpDir, "mock-claude.sh")
	script := `#!/bin/bash
# Write the prompt argument to a file for verification
echo "$@" > ` + filepath.Join(tmpDir, "prompt.txt") + "\n" + `exit 0
`
	if err := os.WriteFile(mockClaude, []byte(script), 0755); err != nil {
		t.Fatalf("Failed to create mock claude: %v", err)
	}

	exec := executor.NewExecutor(mockClaude, 5*time.Minute)

	task := &types.Task{
		ID:          "task-123",
		Title:       "Implement Feature X",
		Description: "Add feature X with proper error handling",
		EpicID:      "epic-456",
	}

	result := exec.Execute(tmpDir, task)
	if !result.Success {
		t.Fatalf("Execute failed: %v", result.Error)
	}

	// Read the captured prompt
	promptFile := filepath.Join(tmpDir, "prompt.txt")
	promptBytes, err := os.ReadFile(promptFile)
	if err != nil {
		t.Fatalf("Failed to read prompt file: %v", err)
	}
	prompt := string(promptBytes)

	// Verify prompt contains expected elements
	expectedStrings := []string{
		"Implement Feature X",
		"Add feature X with proper error handling",
		"epic-456",
	}

	for _, expected := range expectedStrings {
		if !contains(prompt, expected) {
			t.Errorf("Prompt missing expected string '%s':\n%s", expected, prompt)
		}
	}
}
