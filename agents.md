# Agent Context for Drover

Welcome, AI Agent. This document provides context, architecture, and instructions for working within the `drover` repository.

## Ecosystem Role

> **Part of the Drover Ecosystem**: `drover` acts as the **Parallel Orchestrator**. It manages workflows, breaks down tasks, and parallelizes execution by spanning multiple instances of the core engine (`drover-code`) inside the execution sandbox (`drover-cloud`). 

## Project Overview

Drover is a durable workflow orchestrator designed to parallelize AI coding tasks. Instead of running a single agent (like Claude Code or Cursor) linearly, Drover breaks down a large epic into dependencies, respects blockers, and executes tasks using multiple agents concurrently inside isolated Git worktrees.

### Core Technologies
- **Language**: Go 1.22+
- **Workflow Engine**: DBOS (Durable Operating System)
- **Database**: PostgreSQL (Production) / SQLite (Local Development)
- **CLI Framework**: Cobra
- **Observability**: OpenTelemetry

## Key Concepts
- **Durable Workflows**: Every action is checkpointed by DBOS. If Drover crashes or gets interrupted, it can resume from exactly where it left off.
- **Git Worktrees**: To allow multiple agents to modify the same repository simultaneously without conflict, each worker operates in an isolated Git worktree.
- **Hierarchical Tasks**: Tasks can have sub-tasks (using the `task-123.1` format inspired by Beads). Sub-tasks run first, the parent runs last.

## Development Workflow
When making modifications to `drover`:
1. Ensure changes to core loops (`internal/loop`) or workflows respect DBOS checkpointing rules (e.g., only interact with the database via the DBOS transaction methods).
2. To test your changes, run:
   ```bash
   go test ./...
   ```
3. To compile the CLI:
   ```bash
   go build -o drover .
   ```

## Known Guidelines
- Never introduce non-deterministic operations inside a DBOS workflow function without wrapping it in a DBOS step/activity.
- Respect the OpenTelemetry spans; when adding new operations, ensure they are properly traced.
