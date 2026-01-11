package config

import (
	"os"
	"testing"
	"time"
)

func TestParseIntOrDefault(t *testing.T) {
	tests := []struct {
		input    string
		def      int
		expected int
	}{
		{"5", 10, 5},
		{"100", 0, 100},
		{"-3", 10, -3},
		{"abc", 10, 10}, // invalid returns default
		{"", 10, 10},    // empty returns default
		{"3.14", 10, 3}, // parses integer prefix (3)
		{"7xyz", 10, 7}, // parses prefix
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseIntOrDefault(tt.input, tt.def)
			if result != tt.expected {
				t.Errorf("parseIntOrDefault(%q, %d) = %d; want %d", tt.input, tt.def, result, tt.expected)
			}
		})
	}
}

func TestParseDurationOrDefault(t *testing.T) {
	tests := []struct {
		input    string
		def      time.Duration
		expected time.Duration
	}{
		{"60m", 10 * time.Minute, 60 * time.Minute},
		{"2h", 10 * time.Minute, 2 * time.Hour},
		{"90s", 10 * time.Minute, 90 * time.Second},
		{"1h30m", 10 * time.Minute, 90 * time.Minute},
		{"invalid", 10 * time.Minute, 10 * time.Minute}, // invalid returns default
		{"", 10 * time.Minute, 10 * time.Minute},        // empty returns default
		{"500ms", time.Second, 500 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseDurationOrDefault(tt.input, tt.def)
			if result != tt.expected {
				t.Errorf("parseDurationOrDefault(%q, %v) = %v; want %v", tt.input, tt.def, result, tt.expected)
			}
		})
	}
}

func TestValidateOpenCodeModel(t *testing.T) {
	tests := []struct {
		model       string
		wantErr     bool
		errContains string
	}{
		{"anthropic/claude-sonnet-4-20250514", false, ""},
		{"openai/gpt-4o", false, ""},
		{"google/gemini-2.5-pro", false, ""},
		{"opencode/grok-code", false, ""},
		{"claude-sonnet-4-20250514", true, "invalid OpenCode model format"},
		{"", true, "invalid OpenCode model format"},
		{"/model", true, "invalid OpenCode model format"},
		{"provider/", true, "invalid OpenCode model format"},
		{"provider/model/extra", true, "invalid OpenCode model format"},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			err := ValidateOpenCodeModel(tt.model)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateOpenCodeModel(%q) error = %v, wantErr %v", tt.model, err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errContains != "" && err != nil {
				if !containsString(err.Error(), tt.errContains) {
					t.Errorf("error = %v, should contain %v", err, tt.errContains)
				}
			}
		})
	}
}

func containsString(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	for i := 0; i <= len(haystack)-len(needle); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

func TestConfig_OpenCodeEnvVars(t *testing.T) {
	envKeys := []string{"DROVER_AGENT_TYPE", "DROVER_OPENCODE_MODEL", "DROVER_OPENCODE_PATH", "DROVER_OPENCODE_URL"}
	originalEnv := make(map[string]string)
	for _, key := range envKeys {
		originalEnv[key] = os.Getenv(key)
	}
	defer func() {
		for key, value := range originalEnv {
			os.Setenv(key, value)
		}
	}()

	tests := []struct {
		name      string
		envVars   map[string]string
		wantType  AgentType
		wantModel string
		wantPath  string
		wantURL   string
	}{
		{
			name:      "default values",
			envVars:   map[string]string{},
			wantType:  AgentTypeClaudeCode,
			wantModel: "anthropic/claude-sonnet-4-20250514",
			wantPath:  "opencode",
		},
		{
			name: "opencode agent type",
			envVars: map[string]string{
				"DROVER_AGENT_TYPE": "opencode",
			},
			wantType:  AgentTypeOpenCode,
			wantModel: "anthropic/claude-sonnet-4-20250514",
		},
		{
			name: "opencode with custom model",
			envVars: map[string]string{
				"DROVER_AGENT_TYPE":     "opencode",
				"DROVER_OPENCODE_MODEL": "openai/gpt-4o",
			},
			wantType:  AgentTypeOpenCode,
			wantModel: "openai/gpt-4o",
		},
		{
			name: "opencode with custom path",
			envVars: map[string]string{
				"DROVER_AGENT_TYPE":    "opencode",
				"DROVER_OPENCODE_PATH": "/usr/local/bin/opencode",
			},
			wantType: AgentTypeOpenCode,
			wantPath: "/usr/local/bin/opencode",
		},
		{
			name: "opencode with server URL",
			envVars: map[string]string{
				"DROVER_AGENT_TYPE":   "opencode",
				"DROVER_OPENCODE_URL": "http://localhost:4096",
			},
			wantType: AgentTypeOpenCode,
			wantURL:  "http://localhost:4096",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, key := range envKeys {
				os.Setenv(key, "")
			}
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}

			if cfg.AgentType != tt.wantType {
				t.Errorf("AgentType = %v, want %v", cfg.AgentType, tt.wantType)
			}
			if tt.wantModel != "" && cfg.OpenCodeModel != tt.wantModel {
				t.Errorf("OpenCodeModel = %v, want %v", cfg.OpenCodeModel, tt.wantModel)
			}
			if tt.wantPath != "" && cfg.OpenCodePath != tt.wantPath {
				t.Errorf("OpenCodePath = %v, want %v", cfg.OpenCodePath, tt.wantPath)
			}
			if tt.wantURL != "" && cfg.OpenCodeURL != tt.wantURL {
				t.Errorf("OpenCodeURL = %v, want %v", cfg.OpenCodeURL, tt.wantURL)
			}
		})
	}
}

func TestConfig_GetAgentExecutorPath(t *testing.T) {
	tests := []struct {
		name       string
		agentType  AgentType
		claudePath string
		opencode   string
		wantPath   string
	}{
		{
			name:       "claude-code agent",
			agentType:  AgentTypeClaudeCode,
			claudePath: "/usr/bin/claude",
			opencode:   "/usr/bin/opencode",
			wantPath:   "/usr/bin/claude",
		},
		{
			name:       "opencode agent",
			agentType:  AgentTypeOpenCode,
			claudePath: "/usr/bin/claude",
			opencode:   "/usr/bin/opencode",
			wantPath:   "/usr/bin/opencode",
		},
		{
			name:       "default (empty) uses claude",
			agentType:  AgentType(""),
			claudePath: "/usr/bin/claude",
			opencode:   "/usr/bin/opencode",
			wantPath:   "/usr/bin/claude",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				AgentType:    tt.agentType,
				ClaudePath:   tt.claudePath,
				OpenCodePath: tt.opencode,
			}
			gotPath := cfg.GetAgentExecutorPath()
			if gotPath != tt.wantPath {
				t.Errorf("GetAgentExecutorPath() = %v, want %v", gotPath, tt.wantPath)
			}
		})
	}
}
