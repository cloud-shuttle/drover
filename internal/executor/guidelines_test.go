package executor

import (
	"testing"
	"time"

	"github.com/cloud-shuttle/drover/pkg/types"
)

func TestExpandGuidelineTemplates_Project(t *testing.T) {
	task := &types.Task{
		ID:    "task-1",
		Title: "Test Task",
		EpicID: "my-epic",
	}
	result := expandGuidelineTemplates("Working on {{project}}", task)
	if result != "Working on my-epic" {
		t.Errorf("expected 'Working on my-epic', got %q", result)
	}
}

func TestExpandGuidelineTemplates_ProjectFallback(t *testing.T) {
	task := &types.Task{
		ID:    "task-1",
		Title: "Test Task",
		EpicID: "",
	}
	result := expandGuidelineTemplates("Working on {{project}}", task)
	if result != "Working on this project" {
		t.Errorf("expected 'Working on this project', got %q", result)
	}
}

func TestExpandGuidelineTemplates_TaskType(t *testing.T) {
	tests := []struct {
		title       string
		desc        string
		expectedType string
	}{
		{"Test the auth module", "", "testing"},
		{"Write tests", "", "testing"},
		{"Fix the crash bug", "", "bug fix"},
		{"Bug in login flow", "", "bug fix"},
		{"Refactor the DB layer", "", "refactoring"},
		{"Document the API", "", "documentation"},
		{"Implement new feature", "", "implementation"},
		{"Add search functionality", "", "implementation"},
	}

	for _, tt := range tests {
		task := &types.Task{Title: tt.title, Description: tt.desc}
		result := expandGuidelineTemplates("Type: {{task_type}}", task)
		expected := "Type: " + tt.expectedType
		if result != expected {
			t.Errorf("for title=%q: expected %q, got %q", tt.title, expected, result)
		}
	}
}

func TestExpandGuidelineTemplates_TaskTypeFromDescription(t *testing.T) {
	task := &types.Task{
		Title:       "Update module",
		Description: "Fix the broken validation logic",
	}
	result := expandGuidelineTemplates("{{task_type}}", task)
	if result != "bug fix" {
		t.Errorf("expected 'bug fix', got %q", result)
	}
}

func TestExpandGuidelineTemplates_Labels(t *testing.T) {
	task := &types.Task{Title: "Test", Description: "test"}
	result := expandGuidelineTemplates("Labels: {{labels}}", task)
	if result != "Labels: none" {
		t.Errorf("expected 'Labels: none', got %q", result)
	}
}

func TestExpandGuidelineTemplates_MultipleVariables(t *testing.T) {
	task := &types.Task{
		Title:  "Test the auth module",
		EpicID: "auth-epic",
	}
	template := "Project: {{project}}, Type: {{task_type}}, Labels: {{labels}}"
	result := expandGuidelineTemplates(template, task)
	expected := "Project: auth-epic, Type: testing, Labels: none"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestExpandGuidelineTemplates_NoVariables(t *testing.T) {
	task := &types.Task{Title: "Test"}
	input := "Plain guidelines with no variables"
	result := expandGuidelineTemplates(input, task)
	if result != input {
		t.Errorf("expected unchanged string, got %q", result)
	}
}

func TestNewAgent_AllTypes(t *testing.T) {
	tests := []struct {
		agentType string
	}{
		{"claude"},
		{"codex"},
		{"amp"},
		{"opencode"},
		{"drovercode"},
		{"drover-code"},
		{"unknown-defaults-to-claude"},
	}

	for _, tt := range tests {
		cfg := &AgentConfig{
			Type:       tt.agentType,
			Path:       "/usr/local/bin/fake-agent",
			Timeout:    5 * time.Minute,
			Verbose:    true,
			ProjectGuidelines: "test guidelines",
		}
		agent, err := NewAgent(cfg)
		if err != nil {
			t.Errorf("NewAgent(%q) returned error: %v", tt.agentType, err)
		}
		if agent == nil {
			t.Errorf("NewAgent(%q) returned nil agent", tt.agentType)
		}
	}
}
