// Package backpressure provides quality validation implementations
package backpressure

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// runCommandCheck executes a shell command check
func (m *Manager) runCommandCheck(ctx context.Context, check *Check, input *ValidateInput, result *CheckResult) error {
	cmdStr, ok := check.Config["command"].(string)
	if !ok {
		result.Passed = false
		result.Message = "command not specified in config"
		return nil
	}

	argsInterface, ok := check.Config["args"]
	if !ok {
		result.Passed = false
		result.Message = "args not specified in config"
		return nil
	}

	argsSlice, ok := argsInterface.([]interface{})
	if !ok {
		// Try []string directly (for built-in checks)
		if stringSlice, ok := argsInterface.([]string); ok {
			argsSlice = make([]interface{}, len(stringSlice))
			for i, s := range stringSlice {
				argsSlice[i] = s
			}
		} else {
			result.Passed = false
			result.Message = "args must be an array"
			return nil
		}
	}

	args := make([]string, 0, len(argsSlice))
	for _, arg := range argsSlice {
		argStr, ok := arg.(string)
		if !ok {
			result.Passed = false
			result.Message = "all args must be strings"
			return nil
		}
		args = append(args, argStr)
	}

	// Create command
	cmd := exec.CommandContext(ctx, cmdStr, args...)

	// Set working directory to project dir if available
	if input.ProjectDir != "" {
		cmd.Dir = input.ProjectDir
	}

	// Run command
	output, err := cmd.CombinedOutput()
	if err != nil {
		result.Passed = false
		result.Message = fmt.Sprintf("command failed: %v\nOutput: %s", err, string(output))
		return nil
	}

	result.Passed = true
	result.Message = fmt.Sprintf("command succeeded: %s %s", cmdStr, strings.Join(args, " "))
	return nil
}

// runLLMCheck performs LLM-based validation
func (m *Manager) runLLMCheck(ctx context.Context, check *Check, input *ValidateInput, result *CheckResult) error {
	prompt, ok := check.Config["prompt"].(string)
	if !ok {
		result.Passed = false
		result.Message = "prompt not specified in config"
		return nil
	}

	// For now, this is a placeholder. In a real implementation, this would
	// call the LLM API to validate the output.
	// TODO: Integrate with actual LLM provider

	// Simple heuristic: if output contains "error", "panic", or "fatal", fail
	output := strings.ToLower(input.Output)
	failureIndicators := []string{"error:", "panic:", "fatal:", "failed"}
	for _, indicator := range failureIndicators {
		if strings.Contains(output, indicator) {
			result.Passed = false
			result.Message = fmt.Sprintf("Output contains failure indicator: %s", indicator)
			return nil
		}
	}

	result.Passed = true
	result.Message = "LLM validation check placeholder"
	result.Metadata = map[string]interface{}{
		"prompt": prompt,
	}
	return nil
}

// runFileCheck checks if files exist
func (m *Manager) runFileCheck(ctx context.Context, check *Check, input *ValidateInput, result *CheckResult) error {
	pathsInterface, ok := check.Config["paths"]
	if !ok {
		result.Passed = false
		result.Message = "paths not specified in config"
		return nil
	}

	pathsSlice, ok := pathsInterface.([]interface{})
	if !ok {
		// Try []string directly (for built-in checks)
		if stringSlice, ok := pathsInterface.([]string); ok {
			pathsSlice = make([]interface{}, len(stringSlice))
			for i, s := range stringSlice {
				pathsSlice[i] = s
			}
		} else {
			result.Passed = false
			result.Message = "paths must be an array"
			return nil
		}
	}

	paths := make([]string, 0, len(pathsSlice))
	for _, path := range pathsSlice {
		pathStr, ok := path.(string)
		if !ok {
			result.Passed = false
			result.Message = "all paths must be strings"
			return nil
		}
		paths = append(paths, pathStr)
	}

	baseDir := input.ProjectDir
	if baseDir == "" {
		// Try to use current directory
		var err error
		baseDir, err = os.Getwd()
		if err != nil {
			result.Passed = false
			result.Message = fmt.Sprintf("could not determine working directory: %v", err)
			return nil
		}
	}

	missingPaths := []string{}
	for _, path := range paths {
		fullPath := filepath.Join(baseDir, path)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			missingPaths = append(missingPaths, path)
		}
	}

	if len(missingPaths) > 0 {
		result.Passed = false
		result.Message = fmt.Sprintf("missing files: %s", strings.Join(missingPaths, ", "))
		return nil
	}

	result.Passed = true
	result.Message = fmt.Sprintf("all files exist: %s", strings.Join(paths, ", "))
	return nil
}

// runRegexCheck performs regex pattern matching
func (m *Manager) runRegexCheck(ctx context.Context, check *Check, input *ValidateInput, result *CheckResult) error {
	patternsInterface, ok := check.Config["patterns"]
	if !ok {
		result.Passed = false
		result.Message = "patterns not specified in config"
		return nil
	}

	patternsSlice, ok := patternsInterface.([]interface{})
	if !ok {
		// Try []string directly (for built-in checks)
		if stringSlice, ok := patternsInterface.([]string); ok {
			patternsSlice = make([]interface{}, len(stringSlice))
			for i, s := range stringSlice {
				patternsSlice[i] = s
			}
		} else {
			result.Passed = false
			result.Message = "patterns must be an array"
			return nil
		}
	}

	patterns := make([]string, 0, len(patternsSlice))
	for _, pattern := range patternsSlice {
		patternStr, ok := pattern.(string)
		if !ok {
			result.Passed = false
			result.Message = "all patterns must be strings"
			return nil
		}
		patterns = append(patterns, patternStr)
	}

	// Check if we should invert the result (fail if pattern matches)
	invert := false
	if invertVal, ok := check.Config["invert"].(bool); ok {
		invert = invertVal
	}

	// Check against output first, then description
	textToCheck := input.Output
	if textToCheck == "" {
		textToCheck = input.Description
	}
	if textToCheck == "" {
		textToCheck = input.Title
	}

	matches := []string{}
	for _, pattern := range patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			result.Passed = false
			result.Message = fmt.Sprintf("invalid regex pattern: %s", pattern)
			return nil
		}

		if re.MatchString(textToCheck) {
			matches = append(matches, pattern)
		}
	}

	// Determine pass/fail
	// Default (invert=false): pass if pattern is found (pattern is required)
	// Invert=true: fail if pattern is found (pattern is forbidden)
	passed := len(matches) > 0  // Default: pass if pattern found
	if invert {
		// Invert: fail if pattern is found (for "no TODO" type checks)
		passed = len(matches) == 0
	}

	if !passed {
		result.Passed = false
		if invert {
			result.Message = fmt.Sprintf("required pattern not found: %s", strings.Join(patterns, ", "))
		} else {
			result.Message = fmt.Sprintf("forbidden pattern found: %s", strings.Join(matches, ", "))
		}
		return nil
	}

	result.Passed = true
	if invert {
		result.Message = fmt.Sprintf("required patterns found: %s", strings.Join(matches, ", "))
	} else {
		result.Message = "no forbidden patterns found"
	}
	return nil
}
