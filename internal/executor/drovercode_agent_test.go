// Package executor_test provides tests for the executor package
package executor_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cloud-shuttle/drover/internal/executor"
	"github.com/cloud-shuttle/drover/pkg/types"
)

// createMockDroverCodeScript creates a shell script that simulates drover-code behavior
func createMockDroverCodeScript(t *testing.T, dir string, exitCode int, sleepMs int) string {
	t.Helper()
	scriptPath := filepath.Join(dir, "mock-drover-code")
	script := fmt.Sprintf(`#!/bin/bash
# Mock drover-code for testing
sleep %d

# Simulate writing a structured result JSON (used by drovercode_agent)
cat > .drover-result.json << EOF
{
  "success": %t,
  "output": "Mock drover-code completed task successfully",
  "error": "",
  "version": 1
}
EOF

exit %d
`, sleepMs/1000, exitCode == 0, exitCode)

	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("Failed to create mock drover-code script: %v", err)
	}
	return scriptPath
}

// TestDroverCodeAgent_Execute_Success verifies successful execution
func TestDroverCodeAgent_Execute_Success(t *testing.T) {
	tmpDir := t.TempDir()
	mockBinary := createMockDroverCodeScript(t, tmpDir, 0, 100)

	agent := executor.NewDroverCodeAgent(mockBinary, 5*time.Minute)
	agent.SetVerbose(true)

	task := &types.Task{
		ID:          "task-123",
		Title:       "Test Task",
		Description: "Test Description",
	}

	result := agent.ExecuteWithContext(context.Background(), tmpDir, task)
	if !result.Success {
		t.Errorf("Execute failed: %v", result.Error)
	}
	if result.Duration == 0 {
		t.Error("Expected non-zero duration")
	}
}

// TestDroverCodeAgent_ExecuteWithTimeout_Success verifies execution within timeout
func TestDroverCodeAgent_ExecuteWithTimeout_Success(t *testing.T) {
	tmpDir := t.TempDir()
	mockBinary := createMockDroverCodeScript(t, tmpDir, 0, 100)

	agent := executor.NewDroverCodeAgent(mockBinary, 5*time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	task := &types.Task{
		ID:          "task-123",
		Title:       "Test Task",
		Description: "Test Description",
	}

	result := agent.ExecuteWithContext(ctx, tmpDir, task)
	if !result.Success {
		t.Errorf("ExecuteWithContext failed: %v", result.Error)
	}
}

// TestDroverCodeAgent_ExecuteWithTimeout_Timeout verifies timeout handling
func TestDroverCodeAgent_ExecuteWithTimeout_Timeout(t *testing.T) {
	tmpDir := t.TempDir()
	mockBinary := createMockDroverCodeScript(t, tmpDir, 0, 5000) // Sleep 5s

	agent := executor.NewDroverCodeAgent(mockBinary, 5*time.Minute) 

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond) // Tight timeout
	defer cancel()

	task := &types.Task{
		ID:          "task-123",
		Title:       "Test Task",
		Description: "Test Description",
	}

	result := agent.ExecuteWithContext(ctx, tmpDir, task)
	if result.Success {
		t.Error("Expected timeout error, got success")
	}
	if result.Error != nil && !strings.Contains(result.Error.Error(), "timed out") {
		t.Errorf("Expected timeout in error message, got: %v", result.Error)
	}
}

// TestDroverCodeAgent_Execute_Failure verifies failure handling
func TestDroverCodeAgent_Execute_Failure(t *testing.T) {
	tmpDir := t.TempDir()
	mockBinary := createMockDroverCodeScript(t, tmpDir, 1, 100) // Exit code 1

	agent := executor.NewDroverCodeAgent(mockBinary, 5*time.Minute)

	task := &types.Task{
		ID:          "task-123",
		Title:       "Test Task",
		Description: "Test Description",
	}

	result := agent.ExecuteWithContext(context.Background(), tmpDir, task)
	if result.Success {
		t.Error("Expected failure, got success")
	}
}

// TestDroverCodeAgent_CheckInstalled verifies binary detection
func TestDroverCodeAgent_CheckInstalled(t *testing.T) {
	tmpDir := t.TempDir()
	mockBinary := createMockDroverCodeScript(t, tmpDir, 0, 0)

	agent := executor.NewDroverCodeAgent(mockBinary, 5*time.Minute)
	err := agent.CheckInstalled()
	if err != nil {
		t.Logf("CheckInstalled returned error (expected with simple mock): %v", err)
	}

	// Test non-existent binary
	nonExistent := executor.NewDroverCodeAgent("/nonexistent/drover-code", 5*time.Minute)
	err = nonExistent.CheckInstalled()
	if err == nil {
		t.Error("Expected error for non-existent binary")
	}
}

// TestDroverCodeAgent_PromptContent verifies prompt building
func TestDroverCodeAgent_PromptContent(t *testing.T) {
	tmpDir := t.TempDir()

	// Enhanced mock that captures the stdin prompt
	mockBinary := filepath.Join(tmpDir, "mock-drover-code")
	script := `#!/bin/bash
# Capture stdin (the prompt) to a file
cat > ` + filepath.Join(tmpDir, "captured_prompt.txt") + `

# Write success result
cat > .drover-result.json << EOF
{"success": true, "output": "Prompt received", "version": 1}
EOF

exit 0
`
	if err := os.WriteFile(mockBinary, []byte(script), 0755); err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}

	agent := executor.NewDroverCodeAgent(mockBinary, 5*time.Minute)
	agent.SetVerbose(true)

	task := &types.Task{
		ID:          "task-456",
		Title:       "Implement User Login",
		Description: "Add secure authentication with JWT",
		EpicID:      "epic-789",
	}

	result := agent.ExecuteWithContext(context.Background(), tmpDir, task)
	if !result.Success {
		t.Fatalf("Execute failed: %v", result.Error)
	}

	// Verify prompt content
	promptFile := filepath.Join(tmpDir, "captured_prompt.txt")
	promptBytes, err := os.ReadFile(promptFile)
	if err != nil {
		t.Fatalf("Failed to read captured prompt: %v", err)
	}
	prompt := string(promptBytes)

	expected := []string{
		"Implement User Login",
		"Add secure authentication with JWT",
		"epic-789",
	}

	for _, exp := range expected {
		if !strings.Contains(prompt, exp) {
			t.Errorf("Prompt missing expected content '%s':\n%s", exp, prompt)
		}
	}
}

