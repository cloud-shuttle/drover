package executor

import (
	"strings"

	"github.com/cloud-shuttle/drover/pkg/types"
)

// expandGuidelineTemplates expands template variables in guidelines string
// Supported variables: {{project}}, {{task_type}}, {{labels}}
// This is used by all agent implementations to expand guidelines before including in prompts
func expandGuidelineTemplates(guidelines string, task *types.Task) string {
	result := guidelines

	// {{project}} - project name (extracted from ProjectDir or task context)
	projectName := "this project"
	if task.EpicID != "" {
		// Use epic ID as project identifier
		projectName = task.EpicID
	}
	result = strings.ReplaceAll(result, "{{project}}", projectName)

	// {{task_type}} - inferred from task title/description
	taskType := "implementation"
	titleLower := strings.ToLower(task.Title)
	descLower := strings.ToLower(task.Description)
	if strings.Contains(titleLower, "test") || strings.Contains(descLower, "test") {
		taskType = "testing"
	} else if strings.Contains(titleLower, "fix") || strings.Contains(titleLower, "bug") ||
		strings.Contains(descLower, "fix") || strings.Contains(descLower, "bug") {
		taskType = "bug fix"
	} else if strings.Contains(titleLower, "refactor") || strings.Contains(descLower, "refactor") {
		taskType = "refactoring"
	} else if strings.Contains(titleLower, "document") || strings.Contains(descLower, "document") {
		taskType = "documentation"
	}
	result = strings.ReplaceAll(result, "{{task_type}}", taskType)

	// {{labels}} - task labels (if available in future)
	labels := "none"
	// TODO: When labels are added to Task struct, use them here
	// For now, we could infer from epic or other task properties
	result = strings.ReplaceAll(result, "{{labels}}", labels)

	return result
}
