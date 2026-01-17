// Package db provides database utilities for Drover
package db

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// hierarchicalIDPattern matches hierarchical task IDs like:
// - task-123 (base task)
// - task-123.1 (first-level sub-task)
// - task-123.1.2 (second-level sub-task)
var hierarchicalIDPattern = regexp.MustCompile(`^([a-z]+-\d+)(?:\.(\d+)(?:\.(\d+))?)?$`)

// ParseHierarchicalID extracts components from a hierarchical ID
// Returns: (baseID, level1Seq, level2Seq, error)
//
// Examples:
//
//	"task-123"           -> ("task-123", 0, 0, nil)
//	"task-123.1"         -> ("task-123", 1, 0, nil)
//	"task-123.1.2"       -> ("task-123", 1, 2, nil)
//	"task-123.5.10"      -> ("task-123", 5, 10, nil)
//	"invalid"            -> ("", 0, 0, error)
func ParseHierarchicalID(id string) (string, int, int, error) {
	matches := hierarchicalIDPattern.FindStringSubmatch(id)
	if matches == nil {
		return "", 0, 0, fmt.Errorf("invalid hierarchical ID format: %s", id)
	}

	baseID := matches[1]
	level1 := 0
	level2 := 0

	if matches[2] != "" {
		level1, _ = strconv.Atoi(matches[2])
	}
	if matches[3] != "" {
		level2, _ = strconv.Atoi(matches[3])
	}

	return baseID, level1, level2, nil
}

// GetIDDepth returns the depth level of a hierarchical ID
// Returns:
//
//	0 = base task (no dots)
//	1 = first-level sub-task
//	2 = second-level sub-task
func GetIDDepth(id string) int {
	_, level1, level2, err := ParseHierarchicalID(id)
	if err != nil {
		return 0
	}

	if level2 > 0 {
		return 2
	}
	if level1 > 0 {
		return 1
	}
	return 0
}

// GenerateHierarchicalID creates a new hierarchical ID
// baseID: parent task ID (e.g., "task-123")
// level: depth level (1 or 2)
// sequence: position among siblings (1-indexed)
//
// Examples:
//
//	GenerateHierarchicalID("task-123", 1, 1) -> "task-123.1"
//	GenerateHierarchicalID("task-123.1", 2, 2) -> "task-123.1.2"
func GenerateHierarchicalID(baseID string, level int, sequence int) string {
	return fmt.Sprintf("%s.%d", baseID, sequence)
}

// ValidateHierarchicalID checks if an ID is valid and within depth limit
// maxDepth: maximum allowed depth (typically 2)
func ValidateHierarchicalID(id string, maxDepth int) error {
	depth := GetIDDepth(id)
	if depth > maxDepth {
		return fmt.Errorf("task depth %d exceeds maximum allowed depth %d", depth, maxDepth)
	}

	// Validate format
	if !hierarchicalIDPattern.MatchString(id) {
		return fmt.Errorf("invalid task ID format: %s", id)
	}

	return nil
}

// ExtractBaseID returns the base ID without sequence numbers
//
// Examples:
//
//	"task-123.1.2" -> "task-123"
//	"task-123.1"   -> "task-123"
//	"task-123"     -> "task-123"
func ExtractBaseID(id string) string {
	baseID, _, _, _ := ParseHierarchicalID(id)
	return baseID
}

// IsSubTask returns true if the ID represents a sub-task (has a dot separator)
func IsSubTask(id string) bool {
	return strings.Contains(id, ".")
}

// GetParentIDFromHierarchicalID returns the parent task ID from a hierarchical ID
// Returns empty string if the ID is already a base task (no parent)
//
// Examples:
//
//	"task-123.1"   -> "task-123"
//	"task-123.1.2" -> "task-123.1"
//	"task-123"     -> ""
func GetParentIDFromHierarchicalID(id string) string {
	lastDot := strings.LastIndex(id, ".")
	if lastDot == -1 {
		return ""
	}
	return id[:lastDot]
}
