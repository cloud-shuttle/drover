package executor

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cloud-shuttle/drover/internal/config"
	"github.com/cloud-shuttle/drover/pkg/types"
)

func createMockOpenCodeScript(t *testing.T, dir string, exitCode int, sleepMs int, output string) string {
	t.Helper()
	scriptPath := filepath.Join(dir, "mock-opencode.sh")
	script := fmt.Sprintf(`#!/bin/bash
sleep %d
echo '%s'
exit %d
`, sleepMs/1000, output, exitCode)

	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("Failed to create mock opencode script: %v", err)
	}
	return scriptPath
}

func TestOpenCodeExecutor_Execute_Success(t *testing.T) {
	tmpDir := t.TempDir()
	mockOpenCode := createMockOpenCodeScript(t, tmpDir, 0, 100, `{"type":"text","text":"Task completed successfully"}`)

	cfg := config.Config{
		AgentType:     config.AgentTypeOpenCode,
		OpenCodePath:  mockOpenCode,
		OpenCodeModel: "anthropic/claude-sonnet-4-20250514",
		TaskTimeout:   5 * time.Minute,
	}

	exec, err := NewOpenCodeExecutor(&cfg)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	task := &types.Task{
		ID:          "task-456",
		Title:       "OpenCode Test Task",
		Description: "Test Description",
	}

	result := exec.Execute(tmpDir, task)
	if !result.Success {
		t.Errorf("Execute failed: %v", result.Error)
	}
	if result.Duration == 0 {
		t.Errorf("Expected non-zero duration")
	}
}

func TestOpenCodeExecutor_Execute_Failure(t *testing.T) {
	tmpDir := t.TempDir()
	mockOpenCode := createMockOpenCodeScript(t, tmpDir, 1, 100, `{"type":"error","text":"Task failed"}`)

	cfg := config.Config{
		AgentType:     config.AgentTypeOpenCode,
		OpenCodePath:  mockOpenCode,
		OpenCodeModel: "anthropic/claude-sonnet-4-20250514",
		TaskTimeout:   5 * time.Minute,
	}

	exec, err := NewOpenCodeExecutor(&cfg)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	task := &types.Task{
		ID:    "task-789",
		Title: "Failing Task",
	}

	result := exec.Execute(tmpDir, task)
	if result.Success {
		t.Errorf("Expected execution to fail, but it succeeded")
	}
	if result.Error == nil {
		t.Errorf("Expected error, got nil")
	}
}

func TestOpenCodeExecutor_ExecuteWithTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	mockOpenCode := createMockOpenCodeScript(t, tmpDir, 0, 5000, `{"type":"text","text":"Slow task"}`) // Sleeps 5 seconds

	cfg := config.Config{
		AgentType:     config.AgentTypeOpenCode,
		OpenCodePath:  mockOpenCode,
		OpenCodeModel: "anthropic/claude-sonnet-4-20250514",
		TaskTimeout:   100 * time.Millisecond,
	}

	exec, err := NewAgentExecutor(&cfg)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	task := &types.Task{
		ID:    "task-timeout",
		Title: "Slow Task",
	}

	result := exec.Execute(tmpDir, task)
	if result.Success {
		t.Errorf("Expected timeout, but execution succeeded")
	}
	if result.Error == nil {
		t.Errorf("Expected timeout error, got nil")
	}
	if result.Error != nil && !containsString(result.Error.Error(), "timed out") {
		t.Errorf("Expected timeout error message, got: %v", result.Error)
	}
}

func TestOpenCodeExecutor_ModelValidation(t *testing.T) {
	tests := []struct {
		name        string
		model       string
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid anthropic model",
			model:   "anthropic/claude-sonnet-4-20250514",
			wantErr: false,
		},
		{
			name:    "valid openai model",
			model:   "openai/gpt-4o",
			wantErr: false,
		},
		{
			name:    "valid google model",
			model:   "google/gemini-2.5-pro",
			wantErr: false,
		},
		{
			name:        "missing provider",
			model:       "claude-sonnet-4-20250514",
			wantErr:     true,
			errContains: "invalid OpenCode model format",
		},
		{
			name:        "empty model",
			model:       "",
			wantErr:     true,
			errContains: "invalid OpenCode model format",
		},
		{
			name:        "too many slashes",
			model:       "provider/model/extra",
			wantErr:     true,
			errContains: "invalid OpenCode model format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Config{
				AgentType:     config.AgentTypeOpenCode,
				OpenCodePath:  "opencode",
				OpenCodeModel: tt.model,
				TaskTimeout:   5 * time.Minute,
			}

			_, err := NewOpenCodeExecutor(&cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewOpenCodeExecutor() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errContains != "" && err != nil {
				if !containsString(err.Error(), tt.errContains) {
					t.Errorf("error = %v, should contain %v", err, tt.errContains)
				}
			}
		})
	}
}

func TestCheckOpenCodeInstalled(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		shouldExist bool
	}{
		{
			name:        "non-existent path",
			path:        "/tmp/non-existent-opencode-12345",
			shouldExist: false,
		},
		{
			name:        "mock script success",
			path:        createMockOpenCodeScript(t, t.TempDir(), 0, 0, ""),
			shouldExist: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckOpenCodeInstalled(tt.path)
			if tt.shouldExist && err != nil {
				t.Errorf("CheckOpenCodeInstalled() expected success, got error: %v", err)
			}
			if !tt.shouldExist && err == nil {
				t.Errorf("CheckOpenCodeInstalled() expected error, got nil")
			}
		})
	}
}

func TestOpenCodeExecutor_BuildArgs(t *testing.T) {
	cfg := config.Config{
		AgentType:     config.AgentTypeOpenCode,
		OpenCodePath:  "opencode",
		OpenCodeModel: "anthropic/claude-sonnet-4-20250514",
		TaskTimeout:   5 * time.Minute,
	}

	exec, err := NewAgentExecutor(&cfg)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	opencodeExec := exec.(*OpenCodeExecutor)
	prompt := "Test prompt"

	args := opencodeExec.buildArgs(prompt)

	if len(args) < 3 {
		t.Errorf("Expected at least 3 args, got %d", len(args))
	}

	if args[0] != "run" {
		t.Errorf("First arg should be 'run', got %s", args[0])
	}

	if args[1] != "--model" {
		t.Errorf("Second arg should be '--model', got %s", args[1])
	}

	if args[2] != "anthropic/claude-sonnet-4-20250514" {
		t.Errorf("Model arg incorrect, got %s", args[2])
	}

	foundModel := false
	foundFormat := false
	for i, arg := range args {
		if arg == "--model" && i+1 < len(args) {
			foundModel = true
		}
		if arg == "--format" && i+1 < len(args) && args[i+1] == "json" {
			foundFormat = true
		}
	}

	if !foundModel {
		t.Error("Missing --model flag in args")
	}
	if !foundFormat {
		t.Error("Missing --format json flag in args")
	}
}

func TestOpenCodeExecutor_BuildArgs_WithServerURL(t *testing.T) {
	cfg := config.Config{
		AgentType:     config.AgentTypeOpenCode,
		OpenCodePath:  "opencode",
		OpenCodeModel: "anthropic/claude-sonnet-4-20250514",
		OpenCodeURL:   "http://localhost:4096",
		TaskTimeout:   5 * time.Minute,
	}

	exec, err := NewAgentExecutor(&cfg)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	opencodeExec := exec.(*OpenCodeExecutor)
	prompt := "Test prompt"

	args := opencodeExec.buildArgs(prompt)

	foundAttach := false
	for i, arg := range args {
		if arg == "--attach" && i+1 < len(args) && args[i+1] == "http://localhost:4096" {
			foundAttach = true
			break
		}
	}

	if !foundAttach {
		t.Error("Missing --attach flag with server URL in args")
	}
}

func TestOpenCodeExecutor_SetVerbose(t *testing.T) {
	cfg := config.Config{
		AgentType:     config.AgentTypeOpenCode,
		OpenCodePath:  "opencode",
		OpenCodeModel: "anthropic/claude-sonnet-4-20250514",
		TaskTimeout:   5 * time.Minute,
	}

	exec, err := NewOpenCodeExecutor(&cfg)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	exec.SetVerbose(true)
	exec.SetVerbose(false)
}

func TestOpenCodeExecutor_PromptContent(t *testing.T) {
	tmpDir := t.TempDir()
	mockOpenCode := createMockOpenCodeScript(t, tmpDir, 0, 100, `{"type":"text","text":"Done"}`)

	cfg := config.Config{
		AgentType:     config.AgentTypeOpenCode,
		OpenCodePath:  mockOpenCode,
		OpenCodeModel: "anthropic/claude-sonnet-4-20250514",
		TaskTimeout:   5 * time.Minute,
	}

	exec, err := NewOpenCodeExecutor(&cfg)
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	task := &types.Task{
		ID:          "task-epic",
		Title:       "Epic Feature Implementation",
		Description: "Implement the new feature for the epic",
		EpicID:      "epic-123",
	}

	result := exec.Execute(tmpDir, task)
	if !result.Success {
		t.Errorf("Execute failed: %v", result.Error)
	}
}

func containsString(haystack, needle string) bool {
	return len(needle) <= len(haystack) && containsStringHelper(needle, haystack)
}

func containsStringHelper(substr, str string) bool {
	for i := 0; i <= len(str)-len(substr); i++ {
		if str[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
