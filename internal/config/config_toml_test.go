package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfigWithTOML(t *testing.T) {
	// Create temporary directory for test
	tmpDir := t.TempDir()
	oldDir, _ := os.Getwd()
	defer os.Chdir(oldDir)
	os.Chdir(tmpDir)

	// Create .drover.toml
	tomlContent := `# Project configuration
agent = "codex"
max_workers = 8
task_timeout = "45m"
task_context_count = 5
max_description_size = "500KB"
max_diff_size = "300KB"
guidelines = """
This is a test project.
- Use Go idioms
- Write tests
"""
default_labels = ["test", "go"]

[agent_preferences]
model = "claude-sonnet-4"
temperature = 0.8
`

	tomlPath := filepath.Join(tmpDir, ".drover.toml")
	if err := os.WriteFile(tomlPath, []byte(tomlContent), 0644); err != nil {
		t.Fatalf("Failed to write test TOML: %v", err)
	}

	// Load config
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify TOML values were loaded
	if cfg.AgentType != "codex" {
		t.Errorf("Expected agent=codex, got %s", cfg.AgentType)
	}
	if cfg.Workers != 8 {
		t.Errorf("Expected max_workers=8, got %d", cfg.Workers)
	}
	if cfg.TaskTimeout != 45*time.Minute {
		t.Errorf("Expected task_timeout=45m, got %v", cfg.TaskTimeout)
	}
	if cfg.TaskContextCount != 5 {
		t.Errorf("Expected task_context_count=5, got %d", cfg.TaskContextCount)
	}
	if cfg.MaxDescriptionSize != 500*1024 {
		t.Errorf("Expected max_description_size=500KB, got %d", cfg.MaxDescriptionSize)
	}
	if cfg.MaxDiffSize != 300*1024 {
		t.Errorf("Expected max_diff_size=300KB, got %d", cfg.MaxDiffSize)
	}
	if cfg.Guidelines == "" {
		t.Error("Expected guidelines to be loaded")
	}
	if len(cfg.DefaultLabels) != 2 {
		t.Errorf("Expected 2 default labels, got %d", len(cfg.DefaultLabels))
	}
	if cfg.AgentPreferences["model"] != "claude-sonnet-4" {
		t.Errorf("Expected agent_preferences.model=claude-sonnet-4, got %v", cfg.AgentPreferences["model"])
	}
}

func TestConfigHierarchy(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()
	oldDir, _ := os.Getwd()
	defer os.Chdir(oldDir)
	os.Chdir(tmpDir)

	// Create global config
	homeDir := t.TempDir()
	globalDir := filepath.Join(homeDir, ".drover")
	os.MkdirAll(globalDir, 0755)
	globalPath := filepath.Join(globalDir, "config.toml")
	globalContent := `max_workers = 4
guidelines = "Global guidelines"
`
	os.WriteFile(globalPath, []byte(globalContent), 0644)

	// Set HOME to our temp dir
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", homeDir)
	defer os.Setenv("HOME", oldHome)

	// Create project config (should override global)
	projectContent := `max_workers = 6
guidelines = "Project guidelines"
`
	projectPath := filepath.Join(tmpDir, ".drover.toml")
	os.WriteFile(projectPath, []byte(projectContent), 0644)

	// Set environment variable (should override both)
	os.Setenv("DROVER_WORKERS", "8")
	defer os.Unsetenv("DROVER_WORKERS")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Environment should win
	if cfg.Workers != 8 {
		t.Errorf("Expected workers=8 (from env), got %d", cfg.Workers)
	}

	// Project guidelines should override global
	if cfg.Guidelines != "Project guidelines" {
		t.Errorf("Expected project guidelines, got %s", cfg.Guidelines)
	}
}

func TestParseByteSize(t *testing.T) {
	tests := []struct {
		input    string
		expected ByteSize
		wantErr  bool
	}{
		{"250KB", 250 * 1024, false},
		{"10MB", 10 * 1024 * 1024, false},
		{"1GB", 1 * 1024 * 1024 * 1024, false},
		{"1024B", 1024, false},
		{"1024", 1024, false},
		{"invalid", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseByteSize(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseByteSize(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("parseByteSize(%q) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: &Config{
				Workers:           4,
				TaskTimeout:       30 * time.Minute,
				TaskContextCount:  5,
				MaxDescriptionSize: 250 * 1024,
				MaxDiffSize:       250 * 1024,
				AgentType:         "claude",
			},
			wantErr: false,
		},
		{
			name: "invalid workers",
			cfg: &Config{
				Workers:   -1,
				AgentType: "claude",
			},
			wantErr: true,
		},
		{
			name: "invalid agent type",
			cfg: &Config{
				Workers:   4,
				AgentType: "invalid-agent",
			},
			wantErr: true,
		},
		{
			name: "negative context count",
			cfg: &Config{
				Workers:          4,
				TaskContextCount: -1,
				AgentType:        "claude",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
