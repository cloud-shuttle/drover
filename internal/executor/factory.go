package executor

import (
	"context"
	"fmt"

	"github.com/cloud-shuttle/drover/internal/config"
	"github.com/cloud-shuttle/drover/pkg/types"
	"go.opentelemetry.io/otel/trace"
)

// AgentExecutor defines the interface for executing AI agent tasks
type AgentExecutor interface {
	Execute(worktreePath string, task *types.Task) *ExecutionResult
	ExecuteWithTimeout(parentCtx context.Context, worktreePath string, task *types.Task, parentSpan ...trace.Span) *ExecutionResult
	ExecuteWithContext(ctx context.Context, worktreePath string, task *types.Task, parentSpan ...trace.Span) *ExecutionResult
}

// ExecutorType represents the type of executor
type ExecutorType string

const (
	ExecutorTypeClaudeCode ExecutorType = "claude-code"
	ExecutorTypeOpenCode   ExecutorType = "opencode"
)

// NewAgentExecutor creates an executor based on configuration
func NewAgentExecutor(cfg *config.Config) (AgentExecutor, error) {
	agentType := cfg.AgentType

	// Default to Claude Code if not specified
	if agentType == "" {
		agentType = config.AgentTypeClaudeCode
	}

	switch agentType {
	case config.AgentTypeOpenCode:
		return NewOpenCodeExecutor(cfg)
	case config.AgentTypeClaudeCode:
		return NewClaudeExecutorFromConfig(cfg), nil
	default:
		return nil, fmt.Errorf("unknown agent type: %s (expected 'claude-code' or 'opencode')", cfg.AgentType)
	}
}

// GetExecutorType returns the executor type for a given agent type
func GetExecutorType(agentType config.AgentType) ExecutorType {
	switch agentType {
	case config.AgentTypeOpenCode:
		return ExecutorTypeOpenCode
	case config.AgentTypeClaudeCode:
		return ExecutorTypeClaudeCode
	default:
		return ExecutorTypeClaudeCode
	}
}

// NewClaudeExecutorFromConfig creates a Claude executor from Config
func NewClaudeExecutorFromConfig(cfg *config.Config) *Executor {
	return NewExecutor(cfg.ClaudePath, cfg.TaskTimeout)
}
