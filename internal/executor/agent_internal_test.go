package executor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cloud-shuttle/drover/pkg/types"
)

// ============================================================================
// truncateString
// ============================================================================

func TestTruncateString_Short(t *testing.T) {
	result := truncateString("hello", 10)
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestTruncateString_ExactLength(t *testing.T) {
	result := truncateString("hello", 5)
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestTruncateString_Long(t *testing.T) {
	result := truncateString("hello world", 5)
	if result != "hello..." {
		t.Errorf("expected 'hello...', got %q", result)
	}
}

func TestTruncateString_Empty(t *testing.T) {
	result := truncateString("", 5)
	if result != "" {
		t.Errorf("expected empty, got %q", result)
	}
}

// ============================================================================
// AgentResult JSON
// ============================================================================

func TestAgentResult_JSON(t *testing.T) {
	r := AgentResult{Success: true, Output: "done", Version: 1}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var decoded AgentResult
	json.Unmarshal(data, &decoded)
	if decoded.Success != true || decoded.Output != "done" || decoded.Version != 1 {
		t.Errorf("roundtrip mismatch: %+v", decoded)
	}
}

func TestAgentResult_Error(t *testing.T) {
	r := AgentResult{Success: false, Error: "failed", Version: 1}
	data, _ := json.Marshal(r)
	var decoded AgentResult
	json.Unmarshal(data, &decoded)
	if decoded.Error != "failed" {
		t.Errorf("expected error 'failed', got %q", decoded.Error)
	}
}

// ============================================================================
// DroverCodeAgent readResultJSON
// ============================================================================

func TestReadResultJSON_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "result.json")
	os.WriteFile(path, []byte(`{"success": true, "output": "done", "version": 1}`), 0644)

	agent := NewDroverCodeAgent("fake", 5*time.Minute)
	result, err := agent.readResultJSON(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success || result.Output != "done" {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestReadResultJSON_FileNotFound(t *testing.T) {
	agent := NewDroverCodeAgent("fake", 5*time.Minute)
	_, err := agent.readResultJSON("/nonexistent/file.json")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestReadResultJSON_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	os.WriteFile(path, []byte(`not json`), 0644)

	agent := NewDroverCodeAgent("fake", 5*time.Minute)
	_, err := agent.readResultJSON(path)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// ============================================================================
// ClaudeAgent buildPrompt
// ============================================================================

func TestClaudeAgent_BuildPrompt_Minimal(t *testing.T) {
	agent := NewClaudeAgent("fake", 5*time.Minute)
	task := &types.Task{ID: "t1", Title: "Fix bug"}
	prompt := agent.buildPrompt(task)

	if !strings.Contains(prompt, "Fix bug") {
		t.Error("prompt missing title")
	}
	if !strings.Contains(prompt, "Please implement") {
		t.Error("prompt missing instruction")
	}
}

func TestClaudeAgent_BuildPrompt_WithDescription(t *testing.T) {
	agent := NewClaudeAgent("fake", 5*time.Minute)
	task := &types.Task{ID: "t1", Title: "Fix bug", Description: "Null pointer in handler"}
	prompt := agent.buildPrompt(task)

	if !strings.Contains(prompt, "Null pointer in handler") {
		t.Error("prompt missing description")
	}
}

func TestClaudeAgent_BuildPrompt_WithEpic(t *testing.T) {
	agent := NewClaudeAgent("fake", 5*time.Minute)
	task := &types.Task{ID: "t1", Title: "Fix bug", EpicID: "epic-1"}
	prompt := agent.buildPrompt(task)

	if !strings.Contains(prompt, "epic-1") {
		t.Error("prompt missing epic ID")
	}
}

func TestClaudeAgent_BuildPrompt_WithGuidelines(t *testing.T) {
	agent := NewClaudeAgent("fake", 5*time.Minute)
	agent.SetGuidelines("Always use Go 1.22+")
	task := &types.Task{ID: "t1", Title: "Fix bug"}
	prompt := agent.buildPrompt(task)

	if !strings.Contains(prompt, "Always use Go 1.22+") {
		t.Error("prompt missing guidelines")
	}
	if !strings.Contains(prompt, "Project Guidelines") {
		t.Error("prompt missing guidelines header")
	}
}

func TestClaudeAgent_SetVerbose(t *testing.T) {
	agent := NewClaudeAgent("fake", 5*time.Minute)
	agent.SetVerbose(true)
	if !agent.verbose {
		t.Error("expected verbose=true")
	}
	agent.SetVerbose(false)
	if agent.verbose {
		t.Error("expected verbose=false")
	}
}

func TestClaudeAgent_CheckInstalled_NonExistent(t *testing.T) {
	agent := NewClaudeAgent("/nonexistent/claude", 5*time.Minute)
	err := agent.CheckInstalled()
	if err == nil {
		t.Error("expected error for nonexistent binary")
	}
	if !strings.Contains(err.Error(), "claude not found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// ============================================================================
// AmpAgent buildPrompt
// ============================================================================

func TestAmpAgent_BuildPrompt_Minimal(t *testing.T) {
	agent := NewAmpAgent("fake", 5*time.Minute)
	task := &types.Task{ID: "t1", Title: "Add API"}
	prompt := agent.buildPrompt(task)

	if !strings.Contains(prompt, "Add API") {
		t.Error("prompt missing title")
	}
}

func TestAmpAgent_BuildPrompt_WithGuidelines(t *testing.T) {
	agent := NewAmpAgent("fake", 5*time.Minute)
	agent.SetGuidelines("Follow REST conventions")
	task := &types.Task{ID: "t1", Title: "Add API", EpicID: "api-epic"}
	prompt := agent.buildPrompt(task)

	if !strings.Contains(prompt, "Follow REST conventions") {
		t.Error("prompt missing guidelines")
	}
	if !strings.Contains(prompt, "api-epic") {
		t.Error("prompt missing epic")
	}
}

func TestAmpAgent_SetVerbose(t *testing.T) {
	agent := NewAmpAgent("fake", 5*time.Minute)
	agent.SetVerbose(true)
	if !agent.verbose {
		t.Error("expected verbose=true")
	}
}

func TestAmpAgent_CheckInstalled_NonExistent(t *testing.T) {
	agent := NewAmpAgent("/nonexistent/amp", 5*time.Minute)
	err := agent.CheckInstalled()
	if err == nil {
		t.Error("expected error for nonexistent binary")
	}
	if !strings.Contains(err.Error(), "amp not found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// ============================================================================
// CodexAgent buildPrompt
// ============================================================================

func TestCodexAgent_BuildPrompt_Minimal(t *testing.T) {
	agent := NewCodexAgent("fake", 5*time.Minute)
	task := &types.Task{ID: "t1", Title: "Refactor DB"}
	prompt := agent.buildPrompt(task)

	if !strings.Contains(prompt, "Refactor DB") {
		t.Error("prompt missing title")
	}
}

func TestCodexAgent_BuildPrompt_Full(t *testing.T) {
	agent := NewCodexAgent("fake", 5*time.Minute)
	agent.SetGuidelines("Use connection pooling")
	task := &types.Task{
		ID:          "t1",
		Title:       "Refactor DB",
		Description: "Migrate to pgx",
		EpicID:      "db-epic",
	}
	prompt := agent.buildPrompt(task)

	for _, expected := range []string{"Use connection pooling", "Refactor DB", "Migrate to pgx", "db-epic"} {
		if !strings.Contains(prompt, expected) {
			t.Errorf("prompt missing %q", expected)
		}
	}
}

func TestCodexAgent_SetVerbose(t *testing.T) {
	agent := NewCodexAgent("fake", 5*time.Minute)
	agent.SetVerbose(true)
	if !agent.verbose {
		t.Error("expected verbose=true")
	}
}

func TestCodexAgent_CheckInstalled_NonExistent(t *testing.T) {
	agent := NewCodexAgent("/nonexistent/codex", 5*time.Minute)
	err := agent.CheckInstalled()
	if err == nil {
		t.Error("expected error for nonexistent binary")
	}
	if !strings.Contains(err.Error(), "codex not found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// ============================================================================
// OpenCodeAgent buildPrompt
// ============================================================================

func TestOpenCodeAgent_BuildPrompt_Minimal(t *testing.T) {
	agent := NewOpenCodeAgent("fake", 5*time.Minute)
	task := &types.Task{ID: "t1", Title: "Add logging"}
	prompt := agent.buildPrompt(task)

	if !strings.Contains(prompt, "Add logging") {
		t.Error("prompt missing title")
	}
}

func TestOpenCodeAgent_BuildPrompt_WithEverything(t *testing.T) {
	agent := NewOpenCodeAgent("fake", 5*time.Minute)
	agent.SetGuidelines("Use structured logging")
	task := &types.Task{
		ID:          "t1",
		Title:       "Add logging",
		Description: "Add slog everywhere",
		EpicID:      "observability",
	}
	prompt := agent.buildPrompt(task)

	for _, expected := range []string{"Use structured logging", "Add logging", "Add slog everywhere", "observability"} {
		if !strings.Contains(prompt, expected) {
			t.Errorf("prompt missing %q", expected)
		}
	}
}

func TestOpenCodeAgent_SetVerbose(t *testing.T) {
	agent := NewOpenCodeAgent("fake", 5*time.Minute)
	agent.SetVerbose(true)
	if !agent.verbose {
		t.Error("expected verbose=true")
	}
}

func TestOpenCodeAgent_CheckInstalled_NonExistent(t *testing.T) {
	agent := NewOpenCodeAgent("/nonexistent/opencode", 5*time.Minute)
	err := agent.CheckInstalled()
	if err == nil {
		t.Error("expected error for nonexistent binary")
	}
	if !strings.Contains(err.Error(), "opencode not found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// ============================================================================
// DroverCodeAgent buildPrompt
// ============================================================================

func TestDroverCodeAgent_BuildPrompt_Minimal(t *testing.T) {
	agent := NewDroverCodeAgent("fake", 5*time.Minute)
	task := &types.Task{ID: "t1", Title: "Setup CI"}
	prompt := agent.buildPrompt(task)

	if !strings.Contains(prompt, "Setup CI") {
		t.Error("prompt missing title")
	}
}

func TestDroverCodeAgent_BuildPrompt_Full(t *testing.T) {
	agent := NewDroverCodeAgent("fake", 5*time.Minute)
	agent.SetProjectGuidelines("Use GitHub Actions")
	task := &types.Task{
		ID:          "t1",
		Title:       "Setup CI",
		Description: "Add CI pipeline",
		EpicID:      "devops",
	}
	prompt := agent.buildPrompt(task)

	for _, expected := range []string{"Use GitHub Actions", "Setup CI", "Add CI pipeline", "devops"} {
		if !strings.Contains(prompt, expected) {
			t.Errorf("prompt missing %q", expected)
		}
	}
}

func TestDroverCodeAgent_SetDroverCodeConfig(t *testing.T) {
	agent := NewDroverCodeAgent("fake", 5*time.Minute)
	agent.SetDroverCodeConfig("/tmp/result.json", "safe", true, true)

	if agent.resultJSONPath != "/tmp/result.json" {
		t.Errorf("expected resultJSONPath, got %q", agent.resultJSONPath)
	}
	if agent.permissionPreset != "safe" {
		t.Errorf("expected permissionPreset 'safe', got %q", agent.permissionPreset)
	}
	if !agent.coordinatorMode {
		t.Error("expected coordinatorMode=true")
	}
	if !agent.jsonl {
		t.Error("expected jsonl=true")
	}
}

func TestDroverCodeAgent_DefaultBinaryPath(t *testing.T) {
	agent := NewDroverCodeAgent("", 5*time.Minute)
	if agent.binaryPath != "drover-code" {
		t.Errorf("expected default 'drover-code', got %q", agent.binaryPath)
	}
}

// ============================================================================
// ExecutionResult
// ============================================================================

func TestExecutionResult_Fields(t *testing.T) {
	r := &ExecutionResult{
		Success:  true,
		Output:   "all good",
		Duration: 2 * time.Second,
	}
	if !r.Success || r.Output != "all good" || r.Duration != 2*time.Second {
		t.Errorf("unexpected fields: %+v", r)
	}
}

func TestExecutionResult_Error(t *testing.T) {
	r := &ExecutionResult{
		Success: false,
		Error:   os.ErrNotExist,
	}
	if r.Success {
		t.Error("expected failure")
	}
	if r.Error == nil {
		t.Error("expected non-nil error")
	}
}
