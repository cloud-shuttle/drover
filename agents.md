# Agent Context for Drover Orchestrator (`drover`)

Welcome, AI Agent. This document provides context, architecture, and instructions for working within the `drover` repository.

## Ecosystem Role

> **Part of the Drover Ecosystem**: `drover` is **Drover Orchestrator**—parallel **task** dispatch in Git worktrees via DBOS. It invokes **Drover Code** (and other agents) locally per **worktree run**. Milestone A hosted path: **Drover Cloud** runs single **agent jobs** on workers; Orchestrator is self-hosted and out of scope until a future platform integration.

**Glossary:** [`CONTEXT.md`](CONTEXT.md). **Org index:** [`../AGENTS.md`](../AGENTS.md).

## Project Overview

Drover is a durable workflow orchestrator designed to parallelize AI coding tasks. Instead of running a single agent linearly, Drover breaks down a large epic into dependencies, respects blockers, and executes tasks using multiple agents concurrently inside isolated Git worktrees.

### Core Technologies

- **Language**: Go 1.26.3
- **Workflow Engine**: DBOS — `dbos.Go` for concurrent steps, `dbos.Sleep` for durable delays
- **Database**: PostgreSQL (production) / SQLite (local)
- **CLI**: Cobra
- **Observability**: OpenTelemetry
- **Agent interface**: Pluggable — Claude Code, Codex, Amp, OpenCode, Drover Code

## Architecture

### Package Map

| Package | Purpose |
|---------|---------|
| `internal/workflow` | Core orchestration (Orchestrator, DBOSOrchestrator, step functions) |
| `internal/db` | SQLite/PostgreSQL persistence |
| `internal/executor` | Pluggable agent interface |
| `internal/git` | Git worktree lifecycle |
| `internal/config` | Configuration loading |
| `internal/analytics` | Task execution analytics |
| `internal/backpressure` | Adaptive concurrency (AIMD) |
| `internal/beads` | Hierarchical task ID parser |
| `internal/modes` | Execution modes (parallel, sequential, queue) |
| `pkg/telemetry` | OpenTelemetry |

### Key Interfaces

- **`workflow.GitManager`** — worktree operations; do not use `*git.WorktreeManager` directly in orchestrator
- **`executor.Agent`** — pluggable agent execution
- **`clock.Clock`** — injectable time for tests

## Key Concepts

- **Durable workflows**: checkpointed by DBOS; resume after crash
- **Git worktrees**: isolated parallel agent workspaces
- **Hierarchical tasks**: `task-123.1` sub-tasks (Beads-style)
- Use `dbos.Go` for fire-and-forget side effects; `dbos.RunAsStep` when results are needed
- Use `dbos.Sleep` not `time.Sleep` inside workflows
- Use `clock.Clock` not raw `time.Now()` in testable workflow paths

## Build and Test

```bash
go build -o drover ./cmd/drover
go test ./...
go test -race ./...
```

DBOS integration tests require `DBOS_SYSTEM_DATABASE_URL`.

## Known Guidelines

- No non-deterministic work inside workflow functions without a DBOS step
- Respect OpenTelemetry spans on new operations
- **Provisioning work** never runs through Orchestrator — that is **Drover Cloud** only
