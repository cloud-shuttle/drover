package modes

import (
	"strings"
	"testing"
)

func TestDefaultPlanningPrompt(t *testing.T) {
	prompt := DefaultPlanningPrompt()

	if prompt == "" {
		t.Fatal("DefaultPlanningPrompt() returned empty string")
	}

	// Verify prompt contains key sections
	requiredSections := []string{
		"PLANNING MODE",
		"create a detailed implementation plan",
		"DO NOT make any code changes",
		"DO NOT use file editing tools",
		"Steps",
		"Files to Create",
		"Files to Modify",
		"Risk Factors",
		"Complexity",
	}

	for _, section := range requiredSections {
		if !strings.Contains(prompt, section) {
			t.Errorf("DefaultPlanningPrompt() should contain '%s'", section)
		}
	}

	// Verify template variables are present
	templateVars := []string{
		"{{.TaskTitle}}",
		"{{.TaskDescription}}",
		"{{.MaxSteps}}",
		"{{.EpicID}}",
	}

	for _, variable := range templateVars {
		if !strings.Contains(prompt, variable) {
			t.Errorf("DefaultPlanningPrompt() should contain template variable '%s'", variable)
		}
	}
}

func TestDefaultBuildingPrompt(t *testing.T) {
	prompt := DefaultBuildingPrompt()

	if prompt == "" {
		t.Fatal("DefaultBuildingPrompt() returned empty string")
	}

	// Verify prompt contains key sections
	requiredSections := []string{
		"BUILDING MODE",
		"execute an approved implementation plan",
		"Follow the Plan",
		"Make Changes",
		"Verify Each Step",
		"Report Progress",
		"ONLY make changes specified in the approved plan",
	}

	for _, section := range requiredSections {
		if !strings.Contains(prompt, section) {
			t.Errorf("DefaultBuildingPrompt() should contain '%s'", section)
		}
	}

	// Verify template variables are present
	templateVars := []string{
		"{{.TaskTitle}}",
		"{{.TaskDescription}}",
		"{{.PlanOverview}}",
		"{{.PlanSteps}}",
	}

	for _, variable := range templateVars {
		if !strings.Contains(prompt, variable) {
			t.Errorf("DefaultBuildingPrompt() should contain template variable '%s'", variable)
		}
	}
}

func TestDefaultRefinementPrompt(t *testing.T) {
	prompt := DefaultRefinementPrompt()

	if prompt == "" {
		t.Fatal("DefaultRefinementPrompt() returned empty string")
	}

	// Verify prompt contains key sections
	requiredSections := []string{
		"PLAN REFINEMENT MODE",
		"improve a failed or rejected implementation plan",
		"Root Cause Analysis",
		"Alternative Approaches",
		"Missing Steps",
		"Risk Mitigation",
		"Dependency Issues",
		"What Changed",
		"Root Cause of Previous Failure",
		"Revised Steps",
	}

	for _, section := range requiredSections {
		if !strings.Contains(prompt, section) {
			t.Errorf("DefaultRefinementPrompt() should contain '%s'", section)
		}
	}

	// Verify template variables are present
	templateVars := []string{
		"{{.PlanOverview}}",
		"{{.PlanSteps}}",
		"{{.FailureReason}}",
	}

	for _, variable := range templateVars {
		if !strings.Contains(prompt, variable) {
			t.Errorf("DefaultRefinementPrompt() should contain template variable '%s'", variable)
		}
	}
}

func TestPlanningPromptStructure(t *testing.T) {
	prompt := DefaultPlanningPrompt()

	// Verify proper structure with headers
	expectedHeaders := []string{
		"## Your Objective",
		"## Requirements for Your Plan",
		"## Output Format",
		"## Important Constraints",
	}

	for _, header := range expectedHeaders {
		if !strings.Contains(prompt, header) {
			t.Errorf("DefaultPlanningPrompt() should contain header '%s'", header)
		}
	}
}

func TestBuildingPromptStructure(t *testing.T) {
	prompt := DefaultBuildingPrompt()

	// Verify proper structure with headers
	expectedHeaders := []string{
		"## Your Objective",
		"## Approved Plan",
		"## Execution Guidelines",
		"## How to Report Progress",
		"## Important Constraints",
	}

	for _, header := range expectedHeaders {
		if !strings.Contains(prompt, header) {
			t.Errorf("DefaultBuildingPrompt() should contain header '%s'", header)
		}
	}
}

func TestRefinementPromptStructure(t *testing.T) {
	prompt := DefaultRefinementPrompt()

	// Verify proper structure with headers
	expectedHeaders := []string{
		"## Original Plan (Failed/Rejected)",
		"## Failure/Rejection Reason",
		"## Your Task",
		"## Output Format",
	}

	for _, header := range expectedHeaders {
		if !strings.Contains(prompt, header) {
			t.Errorf("DefaultRefinementPrompt() should contain header '%s'", header)
		}
	}
}

func TestPlanningPromptConstraints(t *testing.T) {
	prompt := DefaultPlanningPrompt()

	// Verify constraints are emphasized
	constraints := []string{
		"DO NOT make any code changes",
		"DO NOT use file editing tools",
		"DO NOT run build or test commands",
		"Focus on thorough analysis",
		"Consider edge cases",
	}

	for _, constraint := range constraints {
		if !strings.Contains(prompt, constraint) {
			t.Errorf("DefaultPlanningPrompt() should emphasize constraint: '%s'", constraint)
		}
	}
}

func TestBuildingPromptConstraints(t *testing.T) {
	prompt := DefaultBuildingPrompt()

	// Verify constraints for building
	constraints := []string{
		"ONLY make changes specified in the approved plan",
		"If you encounter issues not covered in the plan, STOP and report",
		"Use appropriate file editing tools",
		"Run tests if specified in verification steps",
		"Commit frequently with clear messages",
	}

	for _, constraint := range constraints {
		if !strings.Contains(prompt, constraint) {
			t.Errorf("DefaultBuildingPrompt() should include constraint: '%s'", constraint)
		}
	}
}

func TestPromptsNotEmpty(t *testing.T) {
	tests := []struct {
		name   string
		prompt func() string
	}{
		{"planning prompt", DefaultPlanningPrompt},
		{"building prompt", DefaultBuildingPrompt},
		{"refinement prompt", DefaultRefinementPrompt},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt := tt.prompt()
			if prompt == "" {
				t.Errorf("%s() should not return empty string", tt.name)
			}

			// Check minimum length (prompts should be substantial)
			if len(prompt) < 500 {
				t.Errorf("%s() returned too short prompt (%d chars), want at least 500", tt.name, len(prompt))
			}
		})
	}
}

func TestPromptsContainStepPlaceholders(t *testing.T) {
	// Planning prompt should guide output format
	planningPrompt := DefaultPlanningPrompt()
	stepFormatIndicators := []string{
		"1. [Step Title]",
		"- Description:",
		"- Files:",
		"- Dependencies:",
		"- Verification:",
	}

	for _, indicator := range stepFormatIndicators {
		if !strings.Contains(planningPrompt, indicator) {
			t.Errorf("DefaultPlanningPrompt() should show step format with '%s'", indicator)
		}
	}
}

func TestBuildingPromptProgressFormat(t *testing.T) {
	buildingPrompt := DefaultBuildingPrompt()

	// Verify progress reporting format
	progressIndicators := []string{
		"Step [N]/[Total]",
		"Completed / Failed / In Progress",
		"[Details about what you did]",
	}

	for _, indicator := range progressIndicators {
		if !strings.Contains(buildingPrompt, indicator) {
			t.Errorf("DefaultBuildingPrompt() should include progress format '%s'", indicator)
		}
	}
}
