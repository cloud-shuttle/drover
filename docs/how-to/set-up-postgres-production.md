---
title: Set up PostgreSQL for production orchestrator runs
description: Enable DBOS durable workflows on PostgreSQL while keeping the local task backlog database.
product: drover-orchestrator
audience: platform-operator
doc_type: how-to
topics:
  - agent-jobs
  - deployment
surface: repo-docs
---

# Set up PostgreSQL for production orchestrator runs

By default, **Drover Orchestrator** uses:

- **SQLite** at `.drover/drover.db` for task/epic backlog state (created by `drover init`)
- **DBOS with SQLite** for durable workflow checkpoints (zero extra services)

For production or long-running CI, set **`DBOS_SYSTEM_DATABASE_URL`** so workflow state and checkpoints live in **PostgreSQL**. Task rows still use the project-local SQLite file unless you also override `DROVER_DATABASE_URL`.

## When you need PostgreSQL

| Scenario | SQLite (default) | PostgreSQL (`DBOS_SYSTEM_DATABASE_URL`) |
|----------|------------------|----------------------------------------|
| Local tutorial / laptop | ✅ | Optional |
| CI job on ephemeral disk | ✅ | Recommended if runners are recycled mid-epic |
| Team server running 24/7 orchestrator | ⚠️ | ✅ |
| Surviving DBOS process crash with shared state | Single host only | ✅ across restarts |

## Prerequisites

- PostgreSQL 14+ (local Docker or managed instance)
- Network access from the host running `drover run`
- Go-built `drover` binary (same repo)

## Step 1 — Create database and role

```bash
createdb drover_orchestrator
# Or in psql:
# CREATE USER drover WITH PASSWORD '...';
# CREATE DATABASE drover_orchestrator OWNER drover;
```

Connection string format:

```bash
export DBOS_SYSTEM_DATABASE_URL="postgresql://drover:SECRET@localhost:5432/drover_orchestrator?sslmode=disable"
```

Use `sslmode=require` for managed cloud databases.

## Step 2 — Initialize the project (unchanged)

```bash
cd my-repo
drover init
```

This still creates `.drover/drover.db` (SQLite) for tasks. DBOS creates its own system tables in PostgreSQL on first launch.

## Step 3 — Run with DBOS + Postgres

```bash
export DBOS_SYSTEM_DATABASE_URL="postgresql://drover:SECRET@localhost:5432/drover_orchestrator?sslmode=disable"
export DROVER_AGENT_TYPE=claude

drover run --workers 4 --verbose
```

You should see:

```text
🐂 Using DBOS workflow engine (PostgreSQL)
```

Without the variable, `drover run` uses the SQLite-based orchestrator path.

## Step 4 — Crash recovery check

1. Start `drover run` on a multi-task epic.
2. Send `Ctrl+C` mid-run.
3. Re-run `drover run` with the same `DBOS_SYSTEM_DATABASE_URL`.

DBOS resumes from the last checkpoint; ready tasks continue. See also `drover resume` (shortcut that re-invokes the durable run path).

## Optional — override task database URL

```bash
export DROVER_DATABASE_URL="postgresql://drover:SECRET@localhost:5432/drover_tasks?sslmode=disable"
```

Most deployments keep **tasks in SQLite** for simplicity and only move **workflow durability** to Postgres. Only point `DROVER_DATABASE_URL` at Postgres if you have a deliberate multi-host task-store design.

## Docker Compose example (dev)

```yaml
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: drover
      POSTGRES_PASSWORD: drover
      POSTGRES_DB: drover_orchestrator
    ports:
      - "5433:5432"
```

```bash
export DBOS_SYSTEM_DATABASE_URL="postgresql://drover:drover@localhost:5433/drover_orchestrator?sslmode=disable"
```

## Observability

Pair production Postgres with OpenTelemetry (see project README):

```bash
export DROVER_OTEL_ENABLED=true
export DROVER_OTEL_ENDPOINT=localhost:4317
drover run --workers 4
```

## Related

- [Durable workflows spec](../../spec/durable-workflows.md)
- [Architecture overview](../explanation/architecture.md)
- [Configure agents](configure-agents.md)
