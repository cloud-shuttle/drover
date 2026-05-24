# Drover Orchestrator — domain glossary

Canonical language for **Drover Orchestrator** (`drover` repo). Shared actors (**customer**, **member**, **agent**) are defined in [`../CONTEXT-MAP.md`](../CONTEXT-MAP.md). Hosted **agent jobs** and **provisioning work** live in [`../drover-cloud/CONTEXT.md`](../drover-cloud/CONTEXT.md). The in-worker agent engine is [`../drover-code/CONTEXT.md`](../drover-code/CONTEXT.md). Implementation details belong in `docs/` and ADRs, not here.

## Core product noun

**Drover Orchestrator**  
The canonical name for this product: a durable workflow engine that runs many coding **agents** in parallel against one Git repository using isolated **worktree runs**. Prefer *Drover Orchestrator* in platform docs when distinguishing from **Drover Cloud** (SaaS control plane) or Gateway’s internal “agent loop.” CLI/product shorthand: *Drover* or `drover` when context is unambiguous.

## Work hierarchy

**Epic**  
A named container for related **tasks** (e.g. “MVP Features”). Scheduling and progress roll up at epic scope; an **orchestrator run** may target one epic or the whole backlog.

**Task**  
One unit of agent work with status, priority, optional **blockers**, and optional hierarchical ids (e.g. `task-123.1`). A **task run** executes exactly one **task** to completion (with retries). Distinct from a hosted **agent job** (one remote worker contract run—see [`../drover-cloud/CONTEXT.md`](../drover-cloud/CONTEXT.md)).

**Blocker**  
A dependency edge: task B is blocked until task A completes. The orchestrator enqueues B only when all blockers are satisfied.

**Orchestrator run**  
One invocation of the orchestrator CLI (e.g. `drover run`) that schedules and executes eligible **tasks** until done, paused, or failed—checkpointed by an **orchestrator workflow** (DBOS) so it survives crashes. Not a **platform workflow**, not a hosted **agent job**, not Gateway MCP **agent observability** traffic. DBOS placement: [ADR 0007](../drover-brain/docs/adr/0007-dbos-placement-by-bounded-context.md).

**Worktree run**  
One isolated Git worktree plus one **agent** execution for a single **task** inside an **orchestrator run**. Multiple **worktree runs** may proceed in parallel up to the configured worker limit.

## Agents and execution surface

**Orchestrator agent**  
Any pluggable backend the orchestrator invokes for a **worktree run** (Claude Code, Codex, Amp, OpenCode, **Drover Code**, etc.). The **agent** process runs locally inside the worktree on the machine hosting the **orchestrator run**—not on a Cloud **worker instance** unless a future integration explicitly provisions one per **task**.

**Self-hosted orchestrator run**  
Default and Milestone A scope: the operator runs `drover` on their machine (or CI) against a local clone of the repository. Worktrees, merges, and agent processes stay on that host. **Drover Cloud** does not dispatch **orchestrator runs** in Milestone A.

## Platform boundary (Milestone A)

**Milestone A**  
**Drover Orchestrator is out of scope** for the hosted SaaS path. **Drover Cloud** exposes single **agent jobs** only (1:1 **worker instance** → one headless **Drover Code** run). No `drover run` on platform workers; no epic/task API in the **Cloud console**.

**Future platform integration**  
A member-facing flow may later invoke **Drover Orchestrator** to fan out parallel work—either by running many hosted **agent jobs** from one request (**job dispatch**) or by hosting **orchestrator runs** on platform infrastructure. That design is not locked; do not document it as current behavior in Cloud or Code glossaries.

## Distinct “orchestration” words (do not conflate)

| Term | Context | Meaning |
|------|---------|---------|
| **Orchestrator run** | `drover` | Parallel **task** execution in Git worktrees |
| **Agent job** | **Drover Cloud** | One hosted remote **Drover Code** run |
| **Job dispatch** | **Drover Code** / BYOC | Client fans out N **agent jobs** (coordinator mode) |
| **Provisioning workflow** | **Drover Cloud** | Customer lifecycle **platform workflow** (DBOS) |
| **Platform workflow** | **Drover Cloud** | Operator SaaS durable work — see [ADR 0007](../drover-brain/docs/adr/0007-dbos-placement-by-bounded-context.md) |
| **Orchestrator workflow** | **Drover Orchestrator** | Task/worktree DBOS — same ADR |
| **MCP aggregation** | **Drover Gateway** | LLM-driven tool routing to registered MCP servers |
