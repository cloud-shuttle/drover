---
title: Resume after crash
description: Recover a drover run interrupted by Ctrl+C or process failure using DBOS durable checkpoints.
product: drover-orchestrator
audience: platform-operator
doc_type: tutorial
topics:
  - agent-jobs
surface: repo-docs
---

# Tutorial: Resume after crash

**Drover Orchestrator** checkpoints workflow state so an interrupted `drover run` can continue without redoing finished tasks. This lesson uses a small epic, interrupts mid-run, then resumes.

## Prerequisites

- Completed [First parallel epic](first-parallel-epic.md) (or equivalent: `drover init`, tasks in backlog)
- An agent CLI installed ([Configure agents](../how-to/configure-agents.md))
- Optional: `DBOS_SYSTEM_DATABASE_URL` for PostgreSQL-backed durability ([Set up PostgreSQL](../how-to/set-up-postgres-production.md))

## Step 1 — Create a multi-task epic

```bash
cd my-drover-webpage   # or your test repo
drover epic add "Crash recovery demo"
drover add "Task A — base change" --epic epic-xxx
drover add "Task B — depends on A" --epic epic-xxx --blocked-by task-aaa
drover add "Task C — depends on A" --epic epic-xxx --blocked-by task-aaa
```

Replace `epic-xxx` and `task-aaa` with IDs from command output.

## Step 2 — Start a parallel run

```bash
export DROVER_AGENT_TYPE=claude
drover run --workers 2 --epic epic-xxx --verbose
```

Wait until **at least one task completes** and a second task is in progress (or queued).

## Step 3 — Interrupt the orchestrator

Press `Ctrl+C` in the terminal running `drover run`.

Check partial progress:

```bash
drover status --tree
```

You should see a mix of `completed`, `in_progress`, `ready`, and `blocked` states.

## Step 4 — Resume

Re-run the orchestrator on the same epic:

```bash
drover run --workers 2 --epic epic-xxx
```

Or use the convenience command (same effect — re-enters the durable run loop):

```bash
drover resume
```

### Expected behavior

1. **Completed tasks** are not re-executed from scratch.
2. **In-progress tasks** may be retried according to claim/stall timeouts (`DROVER_CLAIM_TIMEOUT`, `DROVER_STALL_TIMEOUT` in [configuration reference](../reference/configuration.md)).
3. When blockers clear, **ready tasks** run — often two in parallel if `--workers 2`.

## Step 5 — Verify completion

```bash
drover status
```

All tasks should reach `completed`. Confirm Git history:

```bash
git log --oneline -5
```

## SQLite vs PostgreSQL

| Storage | Interrupt recovery |
|---------|-------------------|
| Default SQLite DBOS | Works on same machine; `.drover/` must be intact |
| `DBOS_SYSTEM_DATABASE_URL` | Survives orchestrator process death; shared across hosts if DB is central |

For CI runners that disappear after each job, use PostgreSQL and a persistent `DROVER_DATABASE_URL` only if you need shared task state across machines.

## Related

- [Durable workflows spec](../../spec/durable-workflows.md)
- [CLI reference](../reference/cli.md) — `drover resume`, `drover retry`
- [Set up PostgreSQL for production](../how-to/set-up-postgres-production.md)
