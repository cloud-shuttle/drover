package config

import "testing"

func TestResolveAgentPath(t *testing.T) {
	tests := []struct {
		agentType, path, want string
	}{
		{"claude", "claude", "claude"},
		{"claude", "", "claude"},
		{"codex", "claude", "codex"},
		{"amp", "claude", "amp"},
		{"opencode", "claude", "opencode"}, // previously ran `claude`
		{"opencode", "", "opencode"},
		{"drovercode", "claude", "drover-code"},
		{"drover-code", "claude", "drover-code"},
		{"Drover_Code", "", "drover-code"},
		{"opencode", "/opt/bin/opencode", "/opt/bin/opencode"}, // explicit path kept
		{"claude", "/opt/bin/claude", "/opt/bin/claude"},
		{"worker", "claude", "claude"},  // worker passes the claude path through
		{"unknown", "claude", "claude"}, // unknown types keep legacy behaviour
	}
	for _, tt := range tests {
		if got := ResolveAgentPath(tt.agentType, tt.path); got != tt.want {
			t.Errorf("ResolveAgentPath(%q, %q) = %q, want %q", tt.agentType, tt.path, got, tt.want)
		}
	}
}

func TestNormalizeAgentType(t *testing.T) {
	for in, want := range map[string]string{
		"drover-code": "drovercode", "drover_code": "drovercode", " DroverCode ": "drovercode",
		"OpenCode": "opencode", "claude": "claude",
	} {
		if got := NormalizeAgentType(in); got != want {
			t.Errorf("NormalizeAgentType(%q) = %q, want %q", in, got, want)
		}
	}
	if !IsKnownAgentType("drover-code") || IsKnownAgentType("worker") || IsKnownAgentType("bogus") {
		t.Error("IsKnownAgentType wrong")
	}
}

func TestLoad_ResolvesAgentPathPerType(t *testing.T) {
	tests := []struct {
		envType, envPath, wantType, wantPath string
	}{
		{"opencode", "", "opencode", "opencode"},
		{"drover-code", "", "drovercode", "drover-code"},
		{"codex", "", "codex", "codex"},
		{"opencode", "/custom/opencode", "opencode", "/custom/opencode"},
		{"", "", "claude", "claude"},
	}
	for _, tt := range tests {
		t.Run(tt.envType+"|"+tt.envPath, func(t *testing.T) {
			t.Setenv("DROVER_AGENT_TYPE", tt.envType)
			t.Setenv("DROVER_AGENT_PATH", tt.envPath)
			t.Setenv("DROVER_CLAUDE_PATH", "")
			cfg, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if cfg.AgentType != tt.wantType || cfg.AgentPath != tt.wantPath {
				t.Errorf("got type=%q path=%q, want type=%q path=%q", cfg.AgentType, cfg.AgentPath, tt.wantType, tt.wantPath)
			}
		})
	}
}
