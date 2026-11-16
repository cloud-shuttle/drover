package config

import "strings"

// Canonical agent type names understood by the executor.
const (
	AgentClaude     = "claude"
	AgentCodex      = "codex"
	AgentAmp        = "amp"
	AgentOpenCode   = "opencode"
	AgentDroverCode = "drovercode"
	AgentWorker     = "worker"
)

// defaultAgentBinaries maps each CLI agent type to the binary drover runs
// when no explicit agent path is configured.
var defaultAgentBinaries = map[string]string{
	AgentClaude:     "claude",
	AgentCodex:      "codex",
	AgentAmp:        "amp",
	AgentOpenCode:   "opencode",
	AgentDroverCode: "drover-code",
}

// KnownAgentTypes lists the agent types users can select in .drover.toml or
// DROVER_AGENT_TYPE ("worker" is selected via DROVER_USE_WORKER_SUBPROCESS).
func KnownAgentTypes() []string {
	return []string{AgentClaude, AgentCodex, AgentAmp, AgentOpenCode, AgentDroverCode}
}

// NormalizeAgentType lower-cases and trims an agent type and maps aliases
// ("drover-code", "drover_code") to their canonical name.
func NormalizeAgentType(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	switch t {
	case "drover-code", "drover_code":
		return AgentDroverCode
	}
	return t
}

// IsKnownAgentType reports whether t (after normalisation) is a selectable agent.
func IsKnownAgentType(t string) bool {
	_, ok := defaultAgentBinaries[NormalizeAgentType(t)]
	return ok
}

// DefaultAgentBinary returns the default binary for an agent type, or "" if
// the type has no CLI binary of its own (e.g. "worker" or unknown types).
func DefaultAgentBinary(agentType string) string {
	return defaultAgentBinaries[NormalizeAgentType(agentType)]
}

// ResolveAgentPath returns the binary to run for agentType.
//
// An empty path, or the built-in "claude" default left in place while a
// different agent type is selected, resolves to that type's default binary.
// Any other path is treated as an explicit user choice and returned as is.
func ResolveAgentPath(agentType, path string) string {
	def := DefaultAgentBinary(agentType)
	if def == "" {
		return path
	}
	if path == "" || (path == defaultAgentBinaries[AgentClaude] && NormalizeAgentType(agentType) != AgentClaude) {
		return def
	}
	return path
}
