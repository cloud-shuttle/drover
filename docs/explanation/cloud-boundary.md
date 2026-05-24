---
title: Cloud boundary — orchestrator vs hosted agent jobs
description: Why Drover Orchestrator is self-hosted and out of scope for Milestone A Drover Cloud.
product: drover-orchestrator
audience:
  - evaluator
  - platform-operator
doc_type: explanation
topics:
  - agent-jobs
  - provisioning
surface: repo-docs
---

# Cloud boundary — orchestrator vs hosted agent jobs

Two different products use the word “orchestration.” Do not conflate them.

## Self-hosted orchestrator run

**Drover Orchestrator** (`drover` repo) runs on **your machine or CI**:

- One `drover run` schedules many **tasks** in parallel Git **worktrees**
- Checkpointed by a **orchestrator workflow** (DBOS) so crashes are recoverable
- Agents (Claude Code, Codex, Drover Code, …) execute **locally** inside each worktree

This is the default and current scope for the `drover` CLI.

## Hosted agent job (Drover Cloud)

**Drover Cloud** exposes single **agent jobs** for Milestone A:

- One **worker instance** → one headless **Drover Code** run
- Submitted via `POST /api/v1/agent-jobs`; progress over SSE
- Runs in isolated Unikraft workers, not your laptop’s worktrees

There is **no** `drover run` on platform workers and **no** epic/task API in the Cloud console today.

## Comparison

| | **Orchestrator run** | **Hosted agent job** |
|--|----------------------|----------------------|
| Product | `drover` | `drover-cloud` |
| Unit of work | **Task** (with blockers, epics) | **Agent job** (single run) |
| Where it runs | Your host / CI | Cloud worker (UKC) |
| Parallelism | N worktrees, one repo | N jobs = N separate API calls |
| Milestone A | Self-hosted only | ✅ Shipped |

## Future platform integration

A member-facing flow may later fan out parallel work from Cloud—either many **agent jobs** from one request or hosted **orchestrator runs**. That design is **not locked**; do not document it as current behavior.

## Related

- [Drover Cloud CONTEXT.md](../../../drover-cloud/CONTEXT.md) — Milestone A scope
- [Drover Orchestrator CONTEXT.md](../../CONTEXT.md) — work hierarchy glossary
- [Architecture overview](architecture.md)
