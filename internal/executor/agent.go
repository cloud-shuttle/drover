// Package executor provides agent execution interfaces for different AI coding agents
package executor

import (
	"context"
	"strings"
	"time"

	"github.com/cloud-shuttle/drover/internal/config"
	"github.com/cloud-shuttle/drover/pkg/types"
	"go.opentelemetry.io/otel/trace"
)

// Agent is the interface that all AI coding agents must implement
type Agent interface {
	// ExecuteWithContext runs a task with a context and returns the execution result
	ExecuteWithContext(ctx context.Context, worktreePath string, task *types.Task, parentSpan ...trace.Span) *ExecutionResult

	// CheckInstalled verifies the agent is available and properly configured
	CheckInstalled() error

	// SetVerbose enables or disables verbose logging
	SetVerbose(bool)
}

// AgentConfig contains configuration for creating an agent
type AgentConfig struct {
	// Type is the agent type: "claude", "codex", "amp", or "opencode"
	Type string

	// Path is the path to the agent binary (for claude/codex/amp CLIs)
	Path string

	// Timeout is the maximum duration to wait for task completion
	Timeout time.Duration

	// Verbose enables detailed logging
	Verbose bool

	// Guidelines are project-specific instructions injected into agent prompts
	Guidelines string

	// DroverCode specific configuration
	DroverCode config.DroverCodeConfig
}

// NewAgent creates a new Agent based on the provided configuration
func NewAgent(cfg *AgentConfig) (Agent, error) {
	switch strings.ToLower(cfg.Type) {
	case "drovercode", "drover-code":
		agent := NewDroverCodeAgent(cfg.Path, cfg.Timeout)
		agent.SetVerbose(cfg.Verbose)
		agent.SetProjectGuidelines(cfg.Guidelines)
		agent.SetDroverCodeConfig(
			cfg.DroverCode.ResultJSONPath,
			cfg.DroverCode.PermissionPreset,
			cfg.DroverCode.CoordinatorMode,
			cfg.DroverCode.JSONL,
		)
		return agent, nil
	case "claude":
		agent := NewClaudeAgent(cfg.Path, cfg.Timeout)
		agent.SetGuidelines(cfg.Guidelines)
		return agent, nil
	case "codex":
		agent := NewCodexAgent(cfg.Path, cfg.Timeout)
		agent.SetGuidelines(cfg.Guidelines)
		return agent, nil
	case "amp":
		agent := NewAmpAgent(cfg.Path, cfg.Timeout)
		agent.SetGuidelines(cfg.Guidelines)
		return agent, nil
	case "opencode":
		agent := NewOpenCodeAgent(cfg.Path, cfg.Timeout)
		agent.SetGuidelines(cfg.Guidelines)
		return agent, nil
	default:
		// Default to Claude for backwards compatibility
		agent := NewClaudeAgent(cfg.Path, cfg.Timeout)
		agent.SetGuidelines(cfg.Guidelines)
		return agent, nil
	}
}
