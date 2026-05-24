---
title: Drover Orchestrator documentation
description: Diátaxis index for self-hosted parallel agent orchestration with DBOS durable workflows.
product: drover-orchestrator
audience:
  - evaluator
  - platform-operator
doc_type: explanation
topics:
  - documentation
  - agent-jobs
surface: repo-docs
---

# Drover Orchestrator documentation

Documentation follows [Diátaxis](https://diataxis.fr/): four types of content for four different needs.

## One access path (read this first)

**Drover Orchestrator** runs on **your machine or CI** — parallel coding agents in Git worktrees with DBOS crash recovery. It is **not** the hosted **agent job** API in [Drover Cloud](../../drover-cloud/CONTEXT.md) (Milestone A). See [Cloud boundary](explanation/cloud-boundary.md).

| Need | Type | Start here |
|------|------|------------|
| **Learn** by doing | Tutorial | [First parallel epic](tutorials/first-parallel-epic.md) |
| **Accomplish** a task | How-to | [How-to guides](how-to/) *(coming soon)* |
| **Look up** facts | Reference | [CLI reference](reference/cli.md) · [Feature specs](../spec/) |
| **Understand** concepts | Explanation | [Architecture overview](explanation/architecture.md) |

## Tutorials

Step-by-step lessons for newcomers.

- [First parallel epic](tutorials/first-parallel-epic.md) — `drover init` → epic → blocked tasks → `drover run`

## How-to guides

Goal-oriented recipes. *(Quadrant in progress)*

## Reference

- [Feature specifications](../spec/) — durable workflows, parallel execution, LLM proxy, webhooks, and more
- [CLI reference](reference/cli.md)

## Explanation

Background, design rationale, and mental models.

- [Architecture overview](explanation/architecture.md)
- [Design document](explanation/design.md)
- [Sequence diagrams](explanation/sequence.md)
- [State machine](explanation/state-machine.md)
- [Enhancement proposals](explanation/proposals.md)
- [RoboRev enhancements](explanation/roborev-enhancements.md)
- [Worktree prewarming dashboard](explanation/worktree-prewarming-dashboard.md)
- [Cloud boundary](explanation/cloud-boundary.md) — orchestrator vs hosted agent jobs

## Internal (operators & release history)

Not part of the public Diátaxis quadrants:

- [Release reviews](internal/reviews/)
- [Testing plans & checklists](internal/testing/)

## Other

- [Project overview](../README.md) — install, quick start, command cheat sheet
- [Domain glossary](../CONTEXT.md)
- [Org backlog](../../docs/TASKS.md)

New pages under `docs/` require YAML frontmatter per the org [content taxonomy](../../docs/taxonomy.yaml); validate with [`scripts/validate-content-frontmatter.sh`](../scripts/validate-content-frontmatter.sh).
