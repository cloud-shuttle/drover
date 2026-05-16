# Agent Context for Drover

Welcome, AI Agent. This document provides context, architecture, and instructions for working within the `drover` repository.

## Ecosystem Role

> **Part of the Drover Ecosystem**: `drover` acts as the **Parallel Orchestrator**. It manages workflows, breaks down tasks, and parallelizes execution by spanning multiple instances of the core engine (`drover-code`) inside the execution sandbox (`drover-cloud`).

## Project Overview

Drover is a durable workflow orchestrator designed to parallelize AI coding tasks. Instead of running a single agent (like Claude Code or Cursor) linearly, Drover breaks down a large epic into dependencies, respects blockers, and executes tasks using multiple agents concurrently inside isolated Git worktrees.

### Core Technologies
- **Language**: Go 1.26.3
- **Workflow Engine**: DBOS v0.14.0 (Durable Operating System) — using `dbos.Go` for concurrent steps, `dbos.Sleep` for durable delays
- **Database**: PostgreSQL (Production) / SQLite (Local Development)
- **CLI Framework**: Cobra v1.10
- **Observability**: OpenTelemetry v1.43.0
- **Agent Interface**: Pluggable — supports Claude Code, Codex, Amp, OpenCode, Drover Code

## Architecture

### Package Map

| Package | Purpose | Coverage |
|---------|---------|----------|
| `internal/workflow` | Core orchestration (Orchestrator, DBOSOrchestrator, step functions) | 44.1% |
| `internal/db` | SQLite/PostgreSQL persistence layer | 38.0% |
| `internal/executor` | Pluggable agent interface (Claude, Codex, Amp, OpenCode, Drover Code) | 47.8% |
| `internal/git` | Git worktree lifecycle (create, merge, prune, cleanup) | 69.5% |
| `internal/config` | Configuration loading (env vars, flags, defaults) | 4.8% |
| `internal/analytics` | Task execution analytics and reporting | 91.4% |
| `internal/backpressure` | Adaptive concurrency control (AIMD algorithm) | 77.8% |
| `internal/beads` | Hierarchical task ID parser (e.g., `task-123.1.2`) | 72.8% |
| `internal/clock` | Deterministic time injection for testing | — |
| `internal/context` | Context window management for LLM prompts | — |
| `internal/taskcontext` | Recent-task context carrying between agents | 98.6% |
| `internal/llmproxy` | LLM proxy server for rate limiting | 100% |
| `internal/modes` | Execution mode selection (parallel, sequential, queue) | 90.8% |
| `internal/search` | Task search and filtering | 65.7% |
| `internal/template` | Prompt template engine | 100% |
| `internal/webhooks` | Webhook notification system | 66.1% |
| `internal/dashboard` | Real-time WebSocket dashboard (requires DB) | 39.2% |
| `internal/memory` | Process memory tracking (platform-specific: Linux `/proc`, macOS `ps`/`sysctl`) | 80.6% |
| `internal/callbacks` | Task lifecycle callbacks | 28.4% |
| `internal/flags` | CLI flag parsing | 63.3% |
| `pkg/telemetry` | OpenTelemetry traces, metrics, and spans | 66.5% |
| `pkg/types` | Shared types (Task, TaskStatus, Epic) | — |

### Key Interfaces

- **`workflow.GitManager`** — Defined in `internal/workflow/git_interface.go`. Decouples orchestration from `*git.WorktreeManager`. All worktree operations go through this interface.
- **`executor.Agent`** — Defined in `internal/executor/agent.go`. Pluggable agent execution with `ExecuteWithContext`, `SetProjectGuidelines`, `SetContextManager`, `SetTaskContext`.
- **`clock.Clock`** — Defined in `internal/clock/clock.go`. Injectable time source for deterministic testing.

## Key Concepts
- **Durable Workflows**: Every action is checkpointed by DBOS. If Drover crashes or gets interrupted, it can resume from exactly where it left off.
- **Git Worktrees**: To allow multiple agents to modify the same repository simultaneously without conflict, each worker operates in an isolated Git worktree.
- **Hierarchical Tasks**: Tasks can have sub-tasks (using the `task-123.1` format inspired by Beads). Sub-tasks run first, the parent runs last.
- **Deterministic Clock**: The `clock.Clock` interface replaces raw `time.Now()` calls, enabling fully deterministic time-based unit tests via `clock.MockClock`.
- **Workflow ID Generation**: Uses `sync/atomic.Int64` counter for thread-safe, unique workflow IDs.
- **Concurrent Side-Effects**: Dashboard broadcasts, webhooks, analytics, and event recording use `dbos.Go` for fire-and-forget concurrent execution, keeping them off the critical task-execution path.
- **Durable Delays**: Queue polling uses `dbos.Sleep` instead of `time.Sleep`, ensuring delays survive process crashes and DBOS recovery.
- **Durable Timeouts**: Agent execution uses `dbos.Select` to race the task step against a configurable timeout step. If Drover crashes and recovers, the timeout race replays from the DBOS checkpoint.
- **Dependency Cascade**: `OnTaskComplete` resolves blocker dependencies via `allBlockersComplete`, only enqueuing dependent tasks once ALL their blockers are marked complete. Uses `dbos.RunWorkflow` with `WithDelay(500ms)` for durable, staggered cascade scheduling.

## Development Workflow

When making modifications to `drover`:

1. Ensure changes to core loops or workflows respect DBOS checkpointing rules (e.g., only interact with the database via the DBOS transaction methods).
2. Never introduce non-deterministic operations inside a DBOS workflow function without wrapping it in a DBOS step/activity.
3. Use `dbos.Go` for fire-and-forget side effects (dashboard, webhooks, analytics). Use `dbos.RunAsStep` for operations whose results are needed.
4. Use `dbos.Sleep` instead of `time.Sleep` inside workflows for durable delays that survive recovery.
5. Use `dbos.Select` with `dbos.Go` channels for durable timeout races on long-running steps (e.g., agent execution).
6. Use the `clock.Clock` interface for any time-dependent logic; never use `time.Now()` directly in testable code.
7. When modifying the `GitManager` interface, update `mock_git_manager_test.go` to match.
8. When modifying the `executor.Agent` interface, update `MockAgent` in `step_functions_test.go` to match.

### Build and Test

```bash
# Build the CLI
go build -o drover ./cmd/drover

# Run all tests
go test ./...

# Run with race detector
go test -race ./...

# Run specific package tests with coverage
go test ./internal/workflow/... -coverprofile=coverage.out

# View coverage report
go tool cover -html=coverage.out
```

### Environment-Dependent Tests

All 24 packages pass in the current environment. The following tests require specific infrastructure:

- `internal/workflow` — DBOS integration tests (`TestDBOSOrchestrator_*`) skip without `DBOS_SYSTEM_DATABASE_URL`
- `internal/memory` — Platform-specific: uses build tags (`tracker_linux.go`, `tracker_darwin.go`)

## Known Guidelines
- Never introduce non-deterministic operations inside a DBOS workflow function without wrapping it in a DBOS step/activity.
- Use `dbos.Go` for fire-and-forget side effects (dashboard, webhooks, analytics). Use `dbos.RunAsStep` for operations whose results are needed by the workflow.
- Use `dbos.Sleep` instead of `time.Sleep` inside workflows for durable, crash-safe delays.
- Respect the OpenTelemetry spans; when adding new operations, ensure they are properly traced.
- Use `clock.Clock` for time, `sync/atomic` for counters — no raw `time.Now()` or non-atomic state in workflow paths.
- The `GitManager` interface is the standard for worktree operations — never reference `*git.WorktreeManager` directly in the orchestrator.
