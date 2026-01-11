package executor

import (
	"fmt"
	"testing"
	"time"

	"github.com/cloud-shuttle/drover/internal/config"
)

func TestNewAgentExecutor(t *testing.T) {
	tests := []struct {
		name     string
		cfg      config.Config
		wantType string
		wantErr  bool
	}{
		{
			name:     "default claude-code",
			cfg:      config.Config{},
			wantType: "*executor.Executor",
			wantErr:  false,
		},
		{
			name: "explicit claude-code",
			cfg: config.Config{
				AgentType:  config.AgentTypeClaudeCode,
				ClaudePath: "claude",
			},
			wantType: "*executor.Executor",
			wantErr:  false,
		},
		{
			name: "opencode with model",
			cfg: config.Config{
				AgentType:     config.AgentTypeOpenCode,
				OpenCodePath:  "opencode",
				OpenCodeModel: "anthropic/claude-sonnet-4-20250514",
				TaskTimeout:   60 * time.Minute,
			},
			wantType: "*executor.OpenCodeExecutor",
			wantErr:  false,
		},
		{
			name: "opencode with server URL",
			cfg: config.Config{
				AgentType:     config.AgentTypeOpenCode,
				OpenCodePath:  "opencode",
				OpenCodeModel: "anthropic/claude-sonnet-4-20250514",
				OpenCodeURL:   "http://localhost:4096",
				TaskTimeout:   60 * time.Minute,
			},
			wantType: "*executor.OpenCodeExecutor",
			wantErr:  false,
		},
		{
			name:     "invalid agent type",
			cfg:      config.Config{AgentType: config.AgentType("invalid")},
			wantType: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec, err := NewAgentExecutor(&tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewAgentExecutor() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil && tt.wantType != "" {
				gotType := fmt.Sprintf("%T", exec)
				if gotType != tt.wantType {
					t.Errorf("NewAgentExecutor() type = %v, want %v", gotType, tt.wantType)
				}
			}
		})
	}
}

func TestGetExecutorType(t *testing.T) {
	tests := []struct {
		agentType config.AgentType
		want      ExecutorType
	}{
		{
			agentType: config.AgentTypeClaudeCode,
			want:      ExecutorTypeClaudeCode,
		},
		{
			agentType: config.AgentTypeOpenCode,
			want:      ExecutorTypeOpenCode,
		},
		{
			agentType: config.AgentType("invalid"),
			want:      ExecutorTypeClaudeCode,
		},
		{
			agentType: config.AgentType(""),
			want:      ExecutorTypeClaudeCode,
		},
	}

	for _, tt := range tests {
		t.Run(string(tt.agentType), func(t *testing.T) {
			got := GetExecutorType(tt.agentType)
			if got != tt.want {
				t.Errorf("GetExecutorType(%v) = %v, want %v", tt.agentType, got, tt.want)
			}
		})
	}
}
