// Package executor provides agent execution interfaces for different AI coding agents
package executor

import (
	"context"
	"time"

	"github.com/cloud-shuttle/drover/internal/config"
	ctxmngr "github.com/cloud-shuttle/drover/internal/context"
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

	// SetProjectGuidelines sets project-specific guidelines for the agent
	SetProjectGuidelines(guidelines string)

	// SetContextManager sets the context window manager for the agent
	SetContextManager(manager *ctxmngr.Manager)

	// SetTaskContext sets recent completed tasks for context carrying
	SetTaskContext(recentTasks []*types.Task, taskContextCount int)
}

// AgentConfig contains configuration for creating an agent
type AgentConfig struct {
	// Type is the agent type: "claude", "codex", "amp", "opencode", "drovercode"
	// (alias "drover-code"), or "worker"
	Type string

	// Path is the path to the agent binary. Empty (or the "claude" default with
	// another type selected) resolves to the type's default binary.
	Path string

	// Timeout is the maximum duration to wait for task completion
	Timeout time.Duration

	// Verbose enables detailed logging
	Verbose bool

	// ProjectGuidelines contains project-specific guidelines to include in prompts
	ProjectGuidelines string

	// ContextThresholds defines size limits for content types
	ContextThresholds *ctxmngr.ContentThresholds

	// WorkerBinary is the path to drover-worker binary (for type="worker")
	WorkerBinary string

	// WorkerMemoryLimit is the memory limit for worker processes (for type="worker")
	WorkerMemoryLimit string
}

// NewAgent creates a new Agent based on the provided configuration
func NewAgent(cfg *AgentConfig) (Agent, error) {
	var agent Agent

	agentType := config.NormalizeAgentType(cfg.Type)
	// Resolve the binary for the selected type so a leftover "claude"
	// default path never runs Claude for another agent type.
	path := config.ResolveAgentPath(agentType, cfg.Path)

	switch agentType {
	case "worker":
		// Use drover-worker subprocess for process isolation
		workerPath := cfg.WorkerBinary
		if workerPath == "" {
			workerPath = "drover-worker"
		}
		agent = NewWorkerAgent(workerPath, cfg.Path, cfg.Timeout)
		// Set memory limit if provided
		if wa, ok := agent.(*WorkerAgent); ok && cfg.WorkerMemoryLimit != "" {
			wa.SetMemoryLimit(cfg.WorkerMemoryLimit)
		}
	case config.AgentClaude:
		agent = NewClaudeAgent(path, cfg.Timeout)
	case config.AgentCodex:
		agent = NewCodexAgent(path, cfg.Timeout)
	case config.AgentAmp:
		agent = NewAmpAgent(path, cfg.Timeout)
	case config.AgentOpenCode:
		agent = NewOpenCodeAgent(path, cfg.Timeout)
	case config.AgentDroverCode:
		agent = NewDroverCodeAgent(path, cfg.Timeout)
	default:
		// Default to Claude for backwards compatibility
		agent = NewClaudeAgent(cfg.Path, cfg.Timeout)
	}

	// Set project guidelines if provided
	if cfg.ProjectGuidelines != "" {
		agent.SetProjectGuidelines(cfg.ProjectGuidelines)
	}

	// Set context manager if thresholds are provided
	if cfg.ContextThresholds != nil {
		ctxManager := ctxmngr.NewManagerWithThresholds(cfg.ContextThresholds)
		agent.SetContextManager(ctxManager)
	}

	// Set verbose mode
	if cfg.Verbose {
		agent.SetVerbose(true)
	}

	return agent, nil
}
