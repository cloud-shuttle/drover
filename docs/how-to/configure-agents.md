---
title: Configure orchestrator agents
description: Select and verify Claude Code, Codex, Amp, OpenCode, or isolated worker subprocesses for drover run.
product: drover-orchestrator
audience: platform-operator
doc_type: how-to
topics:
  - agent-jobs
surface: repo-docs
---

# Configure orchestrator agents

**Drover Orchestrator** invokes a pluggable **orchestrator agent** inside each Git worktree. Configuration is per machine (environment variables) or per run (flags that override worker count and mode).

This path is **self-hosted** — not [Drover Cloud hosted agent jobs](../../../drover-cloud/docs/explanation/agent-jobs-vs-orchestrator.md).

## Supported agent types

| `DROVER_AGENT_TYPE` | Binary (`DROVER_AGENT_PATH`) | Notes |
|---------------------|--------------------------------|-------|
| `claude` (default) | `claude` | Anthropic Claude Code CLI |
| `codex` | `codex` | OpenAI Codex agent |
| `amp` | `amp` | Amp agent |
| `opencode` | `opencode` | OpenCode CLI |
| `worker` | inner agent via `DROVER_AGENT_PATH` | Runs the real agent inside a **`drover-worker`** subprocess for OOM isolation |

The factory in `internal/executor/agent.go` wires these types. A **`drover-code`** executor exists in the repo but is **not** selected by `DROVER_AGENT_TYPE` today — use Cloud [hosted agent jobs](../../../drover-cloud/docs/how-to/configure-gateway-hosted-agents.md) for headless Drover Code, or contribute wiring for local `drover-code`.

## Quick setup (Claude Code)

```bash
# Verify the CLI is on PATH
claude --version

export DROVER_AGENT_TYPE=claude
export DROVER_AGENT_PATH=claude   # optional if binary name is claude

cd my-repo
drover init
drover add "Smoke test task" --skip-validation
drover run --workers 1 --verbose
```

## Switch agent type

```bash
# Codex
export DROVER_AGENT_TYPE=codex
export DROVER_AGENT_PATH=codex

# Amp with custom install location
export DROVER_AGENT_TYPE=amp
export DROVER_AGENT_PATH=/usr/local/bin/amp

# OpenCode
export DROVER_AGENT_TYPE=opencode
export DROVER_AGENT_PATH=opencode

drover run --workers 4
```

`DROVER_CLAUDE_PATH` still works as an alias for `DROVER_AGENT_PATH` (deprecated).

## Process-isolated workers (OOM safety)

When many agents run in parallel, use the **`worker`** type so each task runs in a separate `drover-worker` process:

```bash
export DROVER_AGENT_TYPE=worker
export DROVER_AGENT_PATH=claude          # agent the worker invokes
export DROVER_USE_WORKER_SUBPROCESS=true # optional explicit toggle
export DROVER_WORKER_BINARY=drover-worker
export DROVER_WORKER_MEMORY_LIMIT=2G     # optional per-worker RSS cap

drover run --workers 8
```

Build `drover-worker` from this repo: `go build -o drover-worker ./cmd/drover-worker`.

## Project guidelines

Agents receive optional instructions from `.drover/task_template.yaml` (created by `drover init`) and per-task descriptions. Guidelines support template variables (`{{project}}`, `{{task_type}}`, `{{labels}}`) expanded in `internal/executor/guidelines.go`.

## Local testing without live LLM calls

For CI or scheduler testing, use mock-friendly settings documented in [internal testing checklists](../internal/testing/TESTING_CHECKLIST.md) (`DROVER_AGENT_TYPE=mock` where your branch supports it).

## Route LLM traffic through Gateway

To centralize provider keys, failover, and spend logging for self-hosted runs, see [Route LLM traffic through Drover Gateway](route-llm-through-gateway.md).

## Related

- [CLI reference](../reference/cli.md) — `drover run` flags
- [First parallel epic tutorial](../tutorials/first-parallel-epic.md)
- [Planning vs building mode](planning-vs-building-mode.md)
