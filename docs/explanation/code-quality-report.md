---
title: "Platform Code Quality Status Report"
description: "Comprehensive Cyclomatic Complexity and Code Health audit across all drover-org repositories"
product: platform
audience: member
doc_type: explanation
topics:
  - governance-policy
surface: repo-docs
---

# Platform Code Quality Status Report

This document presents a comprehensive code health and quality status report across the thirteen (13) core repositories of the **Drover Platform**. 

Using our custom static analysis and complexity engine (`scripts/quality-gate.py`), we have audited all modules to determine file counts, control-flow decision density, and peak cognitive complexity hotspots.

---

## Centralized Platform Audit Grid

| Repository Name | Bounded Context / Core Role | Audited Files | Peak Complexity | Hotspot Component | Quality Status |
|:---|:---|:---:|:---:|:---|:---:|
| **`drover`** | Self-hosted CLI orchestrator, DAG scheduling, DBOS, worktree sandbox | 91 | 468 | `cmd/drover/commands.go` | 🟡 Needs Modular Refactor |
| **`drover-cloud`** | SaaS tenant billing, Zitadel provisioning, Console API, plans | 79 | 26 | `console/src/lib/cloud.ts` | 🟢 Excellent (Highly Modular) |
| **`drover-brain`** | Vector chunking, semantic indexer, hybrid RAG database, MCP query | 1201 | 585 | `core/providers/gemini/responses.go` | 🟡 Complexity in LLM Parsers |
| **`drover-gateway`** | Edge API gateway, OIDC router, virtual key custody, token buckets | 1153 | 585 | `core/providers/gemini/responses.go` | 🟡 High Provider Complexity |
| **`drover-muster`** | MCP Tool registry, capability binding solver, semantic verifier | 66 | 33 | `capability_handlers.go` | 🟢 Excellent (Low Complexity) |
| **`drover-guard`** | Access control engine (OPA / SpiceDB ReBAC), signature loggers | 12 | 2 | `internal/audit/clickhouse.go` | 🟢 Outstanding (Minimalist) |
| **`drover-warden`** | Prompt injection filters, action boundaries, Beads schema check | 33 | 11 | `pkg/baml/baml_client/runtime.go` | 🟢 Excellent (Well Encapsulated) |
| **`drover-ch-optimiser`** | ClickHouse tuners, cost analytics, query projections compiler | 4523 | 363 | `assert/assertions.go` (vendored) | 🟢 Healthy (High Dependency Volume) |
| **`drover-sh`** | Public developer lander, static docs compiler, Nuxt engine | 33 | 1 | `.nitro/types/nitro.d.ts` | 🟢 Excellent (Static/Decoupled) |
| **`drover-sqlforge`** | AST schema migrations compiler, plan/apply transformations | 54 | 15 | `internal/project/runtime.go` | 🟢 Excellent (Clean Logic) |
| **`drover-code`** | Sandboxed worker runtime engine, headless CLI worker | 379 | 119 | `internal/tui/model.go` | 🟢 Healthy (TUI Component Heavy) |
| **`drover-libs`** | Shared corporate libraries, panic recovery, JSON telemetry | 2 | 1 | `pkg/clock/clock.go` | 🟢 Excellent (Utility Core) |
| **`drover-ui-tokens`** | Brand tailwind scale, Spacing coordinate schema primitives | 0 | 0 | None (Pure CSS & JSON schemas) | 🟢 Excellent (Zero Logic) |

---

## Bounded-Context Audits & Quality Plans

### 1. Drover Orchestrator (`drover`)
* **Role**: Orchestrates deep sandboxed Git worktrees, compiles DAG dependency graphs, and schedules execution.
* **hotspots**: `cmd/drover/commands.go` has a massive peak complexity of **468** (Complexity estimate based on the main CLI command handler).
* **QE Action Plan**:
  * **Property-Based Testing (PBT)**: Write generators to emit random task tree graphs (cycles, variable widths) to verify that the scheduler never cycles or deadlocks.
  * **Modular Extraction**: Subdivide the CLI runner commands inside `commands.go` into isolated package directories.

### 2. Drover Cloud (`drover-cloud`)
* **Role**: Handles Zitadel SaaS client onboarding, billing pipelines, and subscription plans.
* **Hotspots**: The code is highly modular, with a peak complexity of only **26** in the Solid console UI lib.
* **QE Action Plan**:
  * **Tenant Isolation Assertion**: Write E2E mock router calls to assert that any HTTP request missing a valid `X-Dev-Org-ID` header, or containing a mismatching tenant ID, fails secure-default style (returning `401`/`403`).

### 3. Drover Brain & Gateway (`drover-brain` / `drover-gateway`)
* **Role**: Gateway handles credential custody and rate limiting. Brain handles vector databases and RAG indices.
* **Hotspots**: Auditing identifies high complexity (**585**) in the provider response translation layer (`core/providers/gemini/responses.go`). This is typical for AST-mapping of varied LLM JSON payloads.
* **QE Action Plan**:
  * **Mutation Testing**: Invert provider authorization token checks to ensure all failures strictly return `401 Unauthorized` without exposing backend stack traces.
  * **PBT on Chunkers**: Test paragraph/overlap text split sizes to guarantee they satisfy original text size constraints.

### 4. Drover Muster (`drover-muster`)
* **Role**: Solves skill DAGs and validates MCP configurations.
* **Hotspots**: Excellent health profile, peaking at **33** inside the API handlers.
* **QE Action Plan**:
  * **PBT DAG Solver**: Generate thousands of randomized dependency hierarchies with version overrides to confirm skill resolution determinism.

### 5. Drover Guard & Warden (`drover-guard` / `drover-warden`)
* **Role**: Guard handles access control (SpiceDB ReBAC); Warden handles prompt semantic safety filtering.
* **Hotspots**: Peaking at **11** in the Warden BAML client.
* **QE Action Plan**:
  * **Default-Deny Gating**: Verify that any query to Guard returns `false` if SpiceDB connectivity fails.
  * **Prompt Injection PBT**: Feed highly nested prompt payloads to Warden to assert the semantic safety boundary remains closed.

### 6. Drover Code (`drover-code`)
* **Role**: Unikernel worker environment that executes agent jobs.
* **Hotspots**: Statically audits to a peak complexity of **119** inside TUI modeling code.
* **QE Action Plan**:
  * **Cgroup Confinement**: Run integration suites that attempt memory leaks or infinite loop spikes to ensure container/unikernel boundaries clamp the process.

---

## Executing the Scans Locally

Developers and CI pipelines can easily verify these metrics on any branch prior to merging.

### Running a Strict CRAP and Complexity Check
To search for all files in a repository (e.g. `drover-cloud`) that exceed a CRAP limit of `45.0` (or `30.0`):
```bash
python3 scripts/quality-gate.py ./drover-cloud --limit 30.0
```

### Running the Standard Platform Scan
For general continuous integration validation (run automatically during pipeline execution):
```bash
./scripts/run-quality-gate.sh
```
