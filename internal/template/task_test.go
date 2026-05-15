package template

import (
	"strings"
	"testing"
)

func TestValidate_ShortTitle(t *testing.T) {
	errors := Validate("Short", "")
	found := false
	for _, e := range errors {
		if e.Field == "title" && strings.Contains(e.Message, "too short") {
			found = true
		}
	}
	if !found {
		t.Error("expected title-too-short validation error")
	}
}

func TestValidate_GoodTitle(t *testing.T) {
	errors := Validate("Implement the new button component in packages/components/src/button/", "Create a new button component in packages/components/src/button/ with hover states")
	for _, e := range errors {
		if e.Field == "title" && strings.Contains(e.Message, "too short") {
			t.Error("did not expect title-too-short error for a long title")
		}
	}
}

func TestValidate_MissingActionVerb(t *testing.T) {
	errors := Validate("The logging module needs changes", "Something about the logging module")
	found := false
	for _, e := range errors {
		if strings.Contains(e.Message, "action verb") {
			found = true
		}
	}
	if !found {
		t.Error("expected missing-action-verb validation error")
	}
}

func TestValidate_HasActionVerb(t *testing.T) {
	for _, verb := range []string{"create", "add", "fix", "update", "implement", "refactor", "test", "remove", "optimize"} {
		title := verb + " the new feature for the dashboard"
		errors := Validate(title, "Detailed description with packages/components reference for "+verb)
		for _, e := range errors {
			if strings.Contains(e.Message, "action verb") {
				t.Errorf("did not expect missing-action-verb error for verb %q", verb)
			}
		}
	}
}

func TestValidate_ShortDescription(t *testing.T) {
	errors := Validate("Add the new feature module", "short")
	found := false
	for _, e := range errors {
		if e.Field == "description" && strings.Contains(e.Message, "too vague") {
			found = true
		}
	}
	if !found {
		t.Error("expected description-too-vague validation error")
	}
}

func TestValidate_MissingFileReference(t *testing.T) {
	errors := Validate("Add a new feature for users", "Create a comprehensive user management system with validation and error handling for all edge cases")
	found := false
	for _, e := range errors {
		if strings.Contains(e.Message, "file or component") {
			found = true
		}
	}
	if !found {
		t.Error("expected missing file/component reference error")
	}
}

func TestValidate_WithFileReference(t *testing.T) {
	tests := []string{
		"Update internal/db/db.go to add new method",
		"Fix the button component rendering",
		"Modify packages/core to support new API",
		"Update components/sidebar module",
	}
	for _, desc := range tests {
		errors := Validate("Implement the new feature module", desc)
		for _, e := range errors {
			if strings.Contains(e.Message, "file or component") {
				t.Errorf("did not expect missing file/component error for description: %q", desc)
			}
		}
	}
}

func TestValidate_VaguePhrases(t *testing.T) {
	vague := []string{
		"various improvements to the codebase",
		"make it better for users",
		"optimize it for performance",
		"fix issues in the module",
		"update things across the board",
		"improve performance of the system",
		"add features to the dashboard",
		"handle errors in the pipeline",
	}
	for _, phrase := range vague {
		errors := Validate("Implement the new feature module", phrase)
		found := false
		for _, e := range errors {
			if strings.Contains(e.Message, "Vague phrase") {
				found = true
			}
		}
		if !found {
			t.Errorf("expected vague-phrase error for %q", phrase)
		}
	}
}

func TestValidate_CleanPass(t *testing.T) {
	title := "Create user authentication middleware"
	desc := "Implement JWT-based auth middleware in packages/auth/middleware.go with proper token validation, refresh handling, and role-based access control."
	errors := Validate(title, desc)
	if len(errors) != 0 {
		t.Errorf("expected clean pass with 0 errors, got %d: %+v", len(errors), errors)
	}
}

func TestImproveDescription_DetectsComponent(t *testing.T) {
	result := ImproveDescription("Update the UI", "Fix the button component styling")
	if !strings.Contains(result, "button") {
		t.Errorf("expected improved description to mention 'button', got: %s", result)
	}
}

func TestImproveDescription_DetectsTestAction(t *testing.T) {
	result := ImproveDescription("Test the module", "Write tests for the auth module")
	if !strings.Contains(result, "test") && !strings.Contains(result, "Test") {
		t.Errorf("expected improved description to mention testing, got: %s", result)
	}
}

func TestImproveDescription_DetectsFixAction(t *testing.T) {
	result := ImproveDescription("Fix the bug", "Fix the rendering issue in sidebar")
	if !strings.Contains(result, "Fix") && !strings.Contains(result, "fix") {
		t.Errorf("expected improved description to mention fix, got: %s", result)
	}
}

func TestImproveDescription_DetectsAddAction(t *testing.T) {
	result := ImproveDescription("New feature", "Add dark mode support to the settings page")
	if !strings.Contains(result, "Implement") {
		t.Errorf("expected improved description to mention implement, got: %s", result)
	}
}

func TestImproveDescription_FallbackTemplate(t *testing.T) {
	result := ImproveDescription("Something vague", "No real details here")
	if !strings.Contains(result, "Specific Requirements") {
		t.Errorf("expected fallback template, got: %s", result)
	}
}
