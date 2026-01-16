// Package db tests for hierarchical ID utilities
package db

import (
	"testing"
)

func TestParseHierarchicalID(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantBase   string
		wantLevel1 int
		wantLevel2 int
		wantErr    bool
	}{
		{"base task", "task-123", "task-123", 0, 0, false},
		{"first level sub-task", "task-123.1", "task-123", 1, 0, false},
		{"first level sub-task with large number", "task-123.42", "task-123", 42, 0, false},
		{"second level sub-task", "task-123.1.2", "task-123", 1, 2, false},
		{"second level with large numbers", "task-123.5.10", "task-123", 5, 10, false},
		{"invalid format", "invalid", "", 0, 0, true},
		{"invalid no prefix", "123.1", "", 0, 0, true},
		{"empty string", "", "", 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotBase, gotLevel1, gotLevel2, err := ParseHierarchicalID(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseHierarchicalID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if gotBase != tt.wantBase {
					t.Errorf("ParseHierarchicalID() base = %v, want %v", gotBase, tt.wantBase)
				}
				if gotLevel1 != tt.wantLevel1 {
					t.Errorf("ParseHierarchicalID() level1 = %v, want %v", gotLevel1, tt.wantLevel1)
				}
				if gotLevel2 != tt.wantLevel2 {
					t.Errorf("ParseHierarchicalID() level2 = %v, want %v", gotLevel2, tt.wantLevel2)
				}
			}
		})
	}
}

func TestGetIDDepth(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want int
	}{
		{"base task", "task-123", 0},
		{"first level", "task-123.1", 1},
		{"second level", "task-123.1.2", 2},
		{"invalid", "invalid", 0},
		{"empty", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetIDDepth(tt.id); got != tt.want {
				t.Errorf("GetIDDepth() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateHierarchicalID(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		maxDepth int
		wantErr  bool
	}{
		{"valid base", "task-123", 2, false},
		{"valid level 1", "task-123.1", 2, false},
		{"valid level 2", "task-123.1.2", 2, false},
		{"too deep", "task-123.1.2.3", 2, true},
		{"invalid format", "invalid", 2, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHierarchicalID(tt.id, tt.maxDepth)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateHierarchicalID() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestExtractBaseID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want string
	}{
		{"base task", "task-123", "task-123"},
		{"level 1", "task-123.1", "task-123"},
		{"level 2", "task-123.1.2", "task-123"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExtractBaseID(tt.id); got != tt.want {
				t.Errorf("ExtractBaseID() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsSubTask(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want bool
	}{
		{"base task", "task-123", false},
		{"sub task", "task-123.1", true},
		{"nested sub task", "task-123.1.2", true},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSubTask(tt.id); got != tt.want {
				t.Errorf("IsSubTask() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetParentIDFromHierarchicalID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want string
	}{
		{"base task", "task-123", ""},
		{"level 1", "task-123.1", "task-123"},
		{"level 2", "task-123.1.2", "task-123.1"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetParentIDFromHierarchicalID(tt.id); got != tt.want {
				t.Errorf("GetParentIDFromHierarchicalID() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGenerateHierarchicalID(t *testing.T) {
	tests := []struct {
		name     string
		baseID   string
		level    int
		sequence int
		want     string
	}{
		{"level 1 sequence 1", "task-123", 1, 1, "task-123.1"},
		{"level 1 sequence 5", "task-123", 1, 5, "task-123.5"},
		{"level 2 sequence 2", "task-123.1", 2, 2, "task-123.1.2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GenerateHierarchicalID(tt.baseID, tt.level, tt.sequence); got != tt.want {
				t.Errorf("GenerateHierarchicalID() = %v, want %v", got, tt.want)
			}
		})
	}
}
