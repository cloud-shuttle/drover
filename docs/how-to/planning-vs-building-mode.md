---
title: Planning vs building mode
description: Split plan authoring from code execution with drover run --mode and drover plan review.
product: drover-orchestrator
audience: platform-operator
doc_type: how-to
topics:
  - agent-jobs
surface: repo-docs
---

# Planning vs building mode

Drover can run agents in three **worker modes** so you can review implementation plans before any code changes land in Git.

| Mode | `drover run --mode` | Behavior |
|------|---------------------|----------|
| **Combined** | `combined` (default) | Plan and implement in one agent session |
| **Planning** | `planning` | Produce plans only — no file edits |
| **Building** | `building` | Execute **approved** plans only |

Full specification: [`spec/planning-building-modes.md`](../../spec/planning-building-modes.md).

## When to use each mode

- **Combined** — fastest iteration on trusted backlogs; no human plan gate.
- **Planning + building** — epics where you want a human or lead engineer to approve approach before parallel implementation.
- **Planning only** — backlog grooming: generate plans for later review without spending build tokens.

## Two-phase workflow (recommended)

### 1. Create tasks

```bash
drover init
drover epic add "Auth refresh"
drover add "Design OAuth callback flow" --epic epic-auth
drover add "Implement token exchange" --epic epic-auth --blocked-by task-abc
```

### 2. Generate plans

```bash
drover run --workers 2 --mode planning
```

Planning workers write structured plans to the workspace database (status `pending` until approved).

### 3. Review and approve

Interactive TUI:

```bash
drover plan review
```

Or non-interactive:

```bash
drover plan list --status pending
drover plan show <plan-id>
drover plan approve <plan-id>
# drover plan reject <plan-id> --feedback "Add rollback steps"
```

### 4. Execute approved plans

```bash
drover run --workers 4 --mode building --building-approved-only
```

Building workers refuse tasks whose plans are not `approved` when `--building-approved-only` is set.

## Useful flags and environment variables

| Flag / env | Effect |
|------------|--------|
| `--mode planning\|building\|combined` | Worker mode for this run |
| `DROVER_WORKER_MODE` | Same as `--mode` when flag omitted |
| `--require-approval` | Global gate: plans need approval before build |
| `--planning-require-approval` | Planning mode always leaves plans pending |
| `--planning-auto-approve-low` | Auto-approve low-complexity plans |
| `--planning-max-steps N` | Cap steps per plan (default 20) |
| `--building-approved-only` | Building mode skips non-approved plans |
| `--building-verify-steps` | Run verification after each plan step |
| `--refinement-enabled` | Regenerate plans after rejection |
| `--refinement-max-refinements N` | Limit refinement loops (default 3) |

Environment mirrors (see `internal/config/config.go`):

```bash
export DROVER_PLANNING_REQUIRE_APPROVAL=true
export DROVER_BUILDING_APPROVED_ONLY=true
export DROVER_REFINEMENT_ENABLED=true
```

## Combined mode (single pass)

```bash
drover run --workers 4 --mode combined
```

Equivalent to legacy behavior: no `drover plan review` step unless you opt in with `--require-approval`.

## Operational tips

- Run **planning** and **building** passes as separate `drover run` invocations — each honors DBOS checkpointing; interrupting mid-run is safe.
- Use `drover status` and `drover plan list` between phases to see blocked vs ready tasks.
- Rejected plans with `--refinement-enabled` re-enter planning with feedback embedded in the prompt (see spec § Plan Refinement).

## Related

- [CLI reference](../reference/cli.md) — `drover plan` subcommands
- [Configure agents](configure-agents.md)
- [First parallel epic tutorial](../tutorials/first-parallel-epic.md)
