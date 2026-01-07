// Package template provides structured task templates and validation
package template

import (
	"fmt"
	"regexp"
	"strings"
)

// TaskTemplate defines the structure for high-quality tasks
type TaskTemplate struct {
	Title          string            `json:"title"`
	Description    string            `json:"description"`
	TargetFiles    []string          `json:"target_files,omitempty"`      // Specific files to modify
	Components     []string          `json:"components,omitempty"`        // Specific components
	Action         string            `json:"action"`                      // create, fix, update, refactor, test
	AcceptanceCriteria []string      `json:"acceptance_criteria,omitempty"`
	Context        string            `json:"context,omitempty"`           // Additional context
}

// ValidationError represents a validation error with suggestions
type ValidationError struct {
	Field   string   `json:"field"`
	Message string   `json:"message"`
	Suggestions []string `json:"suggestions,omitempty"`
}

// Validate checks if a task meets quality standards
func Validate(title, description string) []ValidationError {
	var errors []ValidationError

	// Check title quality
	if len(title) < 10 {
		errors = append(errors, ValidationError{
			Field:   "title",
			Message: "Title is too short (min 10 chars)",
			Suggestions: []string{
				"Add specific component/feature name",
				"Include the action verb (create, fix, update)",
				"Example: 'Fix Button component New York variant styling'",
			},
		})
	}

	// Check for action verb in title or description
	actionVerbs := []string{"create", "add", "fix", "update", "implement", "refactor", "test", "remove", "optimize"}
	hasActionVerb := false
	lowerDesc := strings.ToLower(title + " " + description)
	for _, verb := range actionVerbs {
		if strings.Contains(lowerDesc, verb) {
			hasActionVerb = true
			break
		}
	}
	if !hasActionVerb {
		errors = append(errors, ValidationError{
			Field:   "description",
			Message: "Missing clear action verb",
			Suggestions: []string{
				"Start with: Create, Add, Fix, Update, Implement, Refactor, Test",
			},
		})
	}

	// Check description quality
	if len(description) < 30 {
		errors = append(errors, ValidationError{
			Field:   "description",
			Message: "Description is too vague (min 30 chars)",
			Suggestions: []string{
				"Specify which files/components to modify",
				"Include technical details (file paths, function names)",
				"Define acceptance criteria",
			},
		})
	}

	// Check for file paths or component names
	hasFileRef := regexp.MustCompile(`[\w/]+\.rs|[\w/]+\.go|packages/\w+|components?/\w+|\w+ component`).MatchString(description)
	if !hasFileRef {
		errors = append(errors, ValidationError{
			Field:   "description",
			Message: "Missing specific file or component references",
			Suggestions: []string{
				"Add file paths like 'packages/components/src/button/'",
				"Reference components like 'Button, Input, Select components'",
				"Specify modules like 'error-boundary component'",
			},
		})
	}

	// Check for vague phrases
	vaguePhrases := []string{
		"various improvements",
		"make it better",
		"optimize it",
		"fix issues",
		"update things",
		"improve performance",
		"add features",
		"handle errors",
	}
	lowerDesc = strings.ToLower(description)
	for _, phrase := range vaguePhrases {
		if strings.Contains(lowerDesc, phrase) {
			errors = append(errors, ValidationError{
				Field:   "description",
				Message: fmt.Sprintf("Vague phrase detected: '%s'", phrase),
				Suggestions: []string{
					"Be specific: which components? what improvements?",
					"Add metrics: 'reduce bundle size by 30%'",
					"List specific files or functions to modify",
				},
			})
			break
		}
	}

	return errors
}

// ImproveDescription generates an improved version of a vague task description
func ImproveDescription(title, description string) string {
	// This is a simple heuristic-based improver
	// In production, you'd use Claude to generate better descriptions

	lowerTitle := strings.ToLower(title)
	lowerDesc := strings.ToLower(description)

	var improvements []string

	// Detect component mentions
	componentRegex := regexp.MustCompile(`(\w+)\s+component`)
	if matches := componentRegex.FindAllStringSubmatch(description, -1); len(matches) > 0 {
		for _, match := range matches {
			if len(match) > 1 {
				improvements = append(improvements, fmt.Sprintf("- Target: %s component", match[1]))
			}
		}
	}

	// Detect action
	if strings.Contains(lowerDesc, "test") || strings.Contains(lowerTitle, "test") {
		improvements = append(improvements, "- Action: Create comprehensive tests")
	}
	if strings.Contains(lowerDesc, "fix") || strings.Contains(lowerTitle, "fix") {
		improvements = append(improvements, "- Action: Fix identified issues")
	}
	if strings.Contains(lowerDesc, "add") || strings.Contains(lowerDesc, "implement") {
		improvements = append(improvements, "- Action: Implement new functionality")
	}

	// If no improvements detected, return a template
	if len(improvements) == 0 {
		return fmt.Sprintf(`%s

Specific Requirements:
- Target files/packages: [specify which files]
- Action: [create/update/fix/test]
- Acceptance criteria: [how to verify it works]
- Context: [any relevant issue numbers or related work]`, title)
	}

	return strings.Join(improvements, "\n")
}
