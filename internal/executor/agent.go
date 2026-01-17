// Package executor provides agent execution interfaces for different AI coding agents
package executor

import (
	"context"
	"time"

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
	// Type is the agent type: "claude", "codex", "amp", or "copilot"
	Type string

	// Path is the path to the agent binary (for claude/codex/amp CLIs)
	Path string

	// Timeout is the maximum duration to wait for task completion
	Timeout time.Duration

	// Verbose enables detailed logging
	Verbose bool
}

// NewAgent creates a new Agent based on the provided configuration
func NewAgent(cfg *AgentConfig) (Agent, error) {
	switch cfg.Type {
	case "claude":
		return NewClaudeAgent(cfg.Path, cfg.Timeout), nil
	case "codex":
		return NewCodexAgent(cfg.Path, cfg.Timeout), nil
	case "amp":
		return NewAmpAgent(cfg.Path, cfg.Timeout), nil
	case "copilot":
		return NewCopilotAgent(cfg.Path, cfg.Timeout), nil
	case "opencode":
		return NewOpenCodeAgent(cfg.Path, cfg.Timeout), nil
	default:
		// Default to Claude for backwards compatibility
		return NewClaudeAgent(cfg.Path, cfg.Timeout), nil
	}
}
