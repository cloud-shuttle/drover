---
title: Drover Improvements Design Document
description: Drover Improvements Design Document
product: drover-orchestrator
audience: platform-operator
doc_type: explanation
topics:
  - agent-jobs
surface: repo-docs
---
# Drover Improvements Design Document

**Version:** 1.0
**Date:** January 2026
**Status:** Draft
**Inspired by:** [Ramp's Inspect Background Agent](https://builders.ramp.com/post/why-we-built-our-background-agent)

---

## Executive Summary

This document outlines six major enhancement areas for Drover, totaling 49 tasks across 16 stories. The improvements focus on reducing worker cold-start time, enhancing observability, enabling dynamic task creation by agents, supporting human intervention, enabling collaboration, and improving CLI ergonomics.

**Total estimated effort:** ~117 hours

---

## Table of Contents

1. [Motivation](#1-motivation)
2. [Epic 1: Worktree Pre-warming & Caching](#2-epic-1-worktree-pre-warming--caching)
3. [Epic 2: Enhanced Observability Dashboard](#3-epic-2-enhanced-observability-dashboard)
4. [Epic 3: Agent-Spawned Sub-Tasks](#4-epic-3-agent-spawned-sub-tasks)
5. [Epic 4: Human-in-the-Loop Intervention](#5-epic-4-human-in-the-loop-intervention)
6. [Epic 5: Session Handoff & Multiplayer](#6-epic-5-session-handoff--multiplayer)
7. [Epic 6: CLI Ergonomics & Quick Capture](#7-epic-6-cli-ergonomics--quick-capture)
8. [Dependency Graph](#8-dependency-graph)
9. [Implementation Phases](#9-implementation-phases)
10. [Risks & Mitigations](#10-risks--mitigations)

---

## 1. Motivation

Ramp's blog post on their "Inspect" background agent revealed several patterns that could significantly improve Drover:

| Ramp Pattern | Drover Application |
|--------------|-------------------|
| Pre-built repository images every 30 minutes | Worktree pool pre-warming |
| Optimistic reading during sync | Read-only mode while git fetch completes |
| Agent-spawned sub-sessions | MCP tool for creating sub-tasks |
| Multiplayer support | Session export/import and operator attribution |
| Fast iteration with warm sandboxes | Pre-warmed worktree pool |
| Statistics showing merged PR rate | Task success metrics dashboard |

---

## 2. Epic 1: Worktree Pre-warming & Caching

### Problem Statement

Currently, each worker creates a fresh git worktree when claiming a task. This involves:
- Cloning/checking out the worktree (~2-10s depending on repo size)
- Installing dependencies (npm install, go mod download) (~10-60s)
- Any build/setup steps (~5-30s)

This cold-start penalty is paid for every task, even when workers could be reused.

### Proposed Solution

#### 2.1 Worktree Pool Manager

Maintain a pool of N pre-initialized worktrees ready for immediate assignment:

```
┌─────────────────────────────────────────────────────────┐
│                    Worktree Pool                         │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐     │
│  │  warm   │  │  warm   │  │ in-use  │  │  cold   │     │
│  │ (ready) │  │ (ready) │  │(claimed)│  │(creating)│    │
│  └─────────┘  └─────────┘  └─────────┘  └─────────┘     │
│       │            │                                     │
│       └────────────┴──── Available for claiming          │
└─────────────────────────────────────────────────────────┘
```

**Pool States:**
- `cold` - Being created/initialized
- `warm` - Ready for immediate use
- `in-use` - Claimed by a worker
- `stale` - Base branch outdated, needs refresh

**Configuration:**
```bash
drover run --workers 4 --pool-size 8 --pool-refresh 30m
```

#### 2.2 Dependency Installation Caching

Share dependency caches across worktrees:

```
.drover/
├── cache/
│   ├── node_modules/          # Shared npm packages (symlinked)
│   ├── go-mod-cache/          # GOMODCACHE
│   └── dep-hashes/            # Lock file hashes for invalidation
└── worktrees/
    ├── pool-1/
    │   └── node_modules -> ../../cache/node_modules
    └── pool-2/
        └── node_modules -> ../../cache/node_modules
```

**Cache Invalidation:**
- Hash `package-lock.json`, `go.sum`, etc.
- On mismatch, rebuild cache before marking pool warm

#### 2.3 Background Sync with Optimistic Reading

Allow workers to start analyzing tasks before git sync completes:

```go
type WorktreeState struct {
    SyncComplete bool
    SyncError    error
    ReadAllowed  bool  // Always true
    WriteAllowed bool  // Only after SyncComplete
}
```

The Claude Code agent can read files and plan its approach while the worktree syncs to HEAD. Write operations are queued until sync completes.

### Success Metrics

| Metric | Current | Target |
|--------|---------|--------|
| Time to first task start | 15-60s | <3s |
| Worker utilization | 60-70% | >90% |
| Disk usage per worktree | 100% | 20% (shared deps) |

---

## 3. Epic 2: Enhanced Observability Dashboard

### Problem Statement

The current TUI and web dashboard show task states but lack:
- Success/failure rate tracking
- Time-to-completion analytics
- Worker efficiency metrics
- Historical trend data

### Proposed Solution

Build on the existing OpenTelemetry/ClickHouse integration plan.

#### 3.1 Task Success Metrics

```sql
-- ClickHouse materialized view
CREATE MATERIALIZED VIEW task_success_rates AS
SELECT
    toStartOfHour(completed_at) as hour,
    epic_id,
    countIf(status = 'completed') as completed,
    countIf(status = 'failed') as failed,
    completed / (completed + failed) as success_rate,
    avg(duration_seconds) as avg_duration,
    quantile(0.5)(duration_seconds) as p50_duration,
    quantile(0.9)(duration_seconds) as p90_duration,
    quantile(0.99)(duration_seconds) as p99_duration
FROM task_completions
GROUP BY hour, epic_id;
```

**Dashboard Components:**
- Completion rate by epic (bar chart)
- Time-to-completion histogram
- Failure pattern clustering (common error messages)

#### 3.2 Worker Efficiency Metrics

```
┌─────────────────────────────────────────────────────────┐
│  Worker Utilization                    [████████░░] 80% │
├─────────────────────────────────────────────────────────┤
│  Worker 1  ████████████░░░░░░░░  task-abc (12m)        │
│  Worker 2  ██████████████████░░  task-def (18m)        │
│  Worker 3  ░░░░░░░░░░░░░░░░░░░░  idle (2m)             │
│  Worker 4  ████████░░░░░░░░░░░░  task-ghi (8m)         │
└─────────────────────────────────────────────────────────┘
```

**Metrics:**
- `drover.worker.utilization` (gauge) - % of workers actively processing
- `drover.worker.idle_time` (counter) - Cumulative idle seconds
- `drover.worker.throughput` (histogram) - Tasks completed per hour

#### 3.3 Live Activity Feed

WebSocket-based real-time event stream:

```typescript
interface TaskEvent {
  type: 'state_change' | 'log' | 'error';
  taskId: string;
  workerId: string;
  timestamp: string;
  data: {
    from?: TaskStatus;
    to?: TaskStatus;
    message?: string;
  };
}
```

**UI Component:**
- Scrolling feed of recent task transitions
- "Active prompting" counter (workers currently in Claude Code)
- Expandable log viewer per task

---

## 4. Epic 3: Agent-Spawned Sub-Tasks

### Problem Statement

Currently, task decomposition is manual. If Claude Code determines a task is too large or needs research, it has no way to delegate sub-work.

### Proposed Solution

Provide Claude Code with MCP tools to create and monitor sub-tasks.

#### 4.1 Sub-Task Creation Tool

```typescript
// MCP Tool Definition
{
  name: "create_subtask",
  description: "Create a sub-task that will be executed by another worker",
  inputSchema: {
    type: "object",
    properties: {
      title: { type: "string", description: "Task title" },
      description: { type: "string", description: "Detailed description" },
      type: {
        type: "string",
        enum: ["implementation", "research", "fix"],
        description: "Task type - research tasks are read-only"
      },
      waitForCompletion: {
        type: "boolean",
        default: false,
        description: "If true, block until sub-task completes"
      }
    },
    required: ["title", "description"]
  }
}
```

**Behavior:**
1. Parent task calls `create_subtask`
2. Sub-task created with automatic `blocked-by` to parent
3. Sub-task enters queue, assigned to available worker
4. If `waitForCompletion`, parent polls via `check_subtask_status`
5. On completion, results stored in parent's context

#### 4.2 Research Task Type

Research tasks have special properties:
- No git commits expected
- Can access multiple repos read-only
- Results stored as structured metadata

```go
type ResearchResult struct {
    TaskID    string
    Query     string
    Findings  []Finding
    Sources   []string
    CreatedAt time.Time
}

type Finding struct {
    Summary   string
    Relevance float64
    SourceRef string
}
```

#### 4.3 Task Decomposition Signal

Allow agents to signal when a task is too large:

```typescript
// MCP Tool
{
  name: "signal_decomposition",
  description: "Signal that this task should be broken into smaller pieces",
  inputSchema: {
    type: "object",
    properties: {
      reason: { type: "string" },
      suggestedSubtasks: {
        type: "array",
        items: {
          type: "object",
          properties: {
            title: { type: "string" },
            description: { type: "string" },
            estimatedComplexity: { type: "string", enum: ["small", "medium", "large"] }
          }
        }
      }
    }
  }
}
```

**Flow:**
1. Agent calls `signal_decomposition` with suggested breakdown
2. Drover creates sub-tasks automatically
3. Original task status → `decomposed`
4. When all sub-tasks complete, original task → `completed`

---

## 5. Epic 4: Human-in-the-Loop Intervention

### Problem Statement

When a task gets stuck or goes down the wrong path, there's no way to intervene without killing the worker and losing progress.

### Proposed Solution

#### 5.1 Task Pause and Resume

```bash
# Gracefully pause a running task
drover pause task-abc

# Resume with optional guidance
drover resume task-abc --hint "Try using the existing auth middleware"
```

**Pause Behavior:**
1. Signal Claude Code to stop at next safe point
2. Preserve worktree state (no cleanup)
3. Task status → `paused`
4. Worker returns to pool

**Resume Behavior:**
1. Claim paused task's worktree (same directory)
2. Inject any guidance into context
3. Continue Claude Code session with existing state

#### 5.2 Interactive Guidance Injection

Allow sending hints to running tasks without pausing:

```bash
# Add guidance to a running task's queue
drover hint task-abc "The auth token is stored in localStorage, not cookies"
```

**Implementation:**
- Each task maintains a guidance FIFO queue
- Queue checked at Claude Code prompt boundaries
- Guidance injected as user messages in conversation

```go
type GuidanceMessage struct {
    ID        string
    TaskID    string
    Message   string
    CreatedAt time.Time
    Delivered bool
}
```

#### 5.3 Dashboard Intervention UI

```
┌─────────────────────────────────────────────────────────┐
│  task-abc: Implement OAuth flow                         │
│  Status: in_progress (12m)  Worker: 2                   │
├─────────────────────────────────────────────────────────┤
│  [⏸ Pause]  [📝 Send Hint]  [📁 View Files]            │
├─────────────────────────────────────────────────────────┤
│  Hint: [                                             ]  │
│        [Send]                                           │
├─────────────────────────────────────────────────────────┤
│  Files:                                                 │
│  ├── src/auth/oauth.ts (modified)                      │
│  ├── src/auth/callback.ts (new)                        │
│  └── tests/auth.test.ts (modified)                     │
└─────────────────────────────────────────────────────────┘
```

---

## 6. Epic 5: Session Handoff & Multiplayer

### Problem Statement

Drover sessions are tied to a single operator. There's no way to:
- Hand off a session to a colleague
- Have multiple people contribute to the same project run
- Track who made which changes

### Proposed Solution

#### 6.1 Session State Export/Import

```bash
# Export current session state
drover export --output session-2026-01-13.drover

# Import on another machine
drover import session-2026-01-13.drover --continue
```

**Export Format:**
```json
{
  "version": "1.0",
  "exportedAt": "2026-01-13T10:30:00Z",
  "exportedBy": "peter@cloudshuttle.com",
  "repository": "github.com/cloud-shuttle/drover",
  "baseCommit": "abc123",
  "tasks": [...],
  "epics": [...],
  "completedCommits": [...],
  "worktreeSnapshots": [...]
}
```

#### 6.2 Multi-Operator Attribution

Track operator for each action:

```go
type Task struct {
    // ... existing fields
    CreatedBy   string    // Operator who created
    ModifiedBy  string    // Last operator to modify
    CompletedBy string    // Operator whose worker completed
}
```

**Git Integration:**
- Commits attributed to the operator who created the task
- Uses operator's GitHub credentials for auth
- Co-authored-by for multi-operator tasks

#### 6.3 Live Session Sharing

Dashboard generates shareable URLs:

```
https://drover.cloudshuttle.dev/session/abc123?token=xyz
```

**Capabilities:**
- Real-time view of task progress
- Send hints to tasks
- Add new tasks to queue
- Export snapshot

---

## 7. Epic 6: CLI Ergonomics & Quick Capture

### Problem Statement

Creating tasks requires verbose commands. Checking status requires entering the TUI.

### Proposed Solution

#### 7.1 Quick Task Capture

```bash
# Minimal input task creation
drover quick "fix the login redirect bug"

# With AI enrichment
drover quick "add dark mode support" --enrich
# AI suggests: epic=ui-improvements, blocked-by=task-theme-system, labels=frontend
```

#### 7.2 Voice Input

```bash
# Speech-to-text task creation
drover voice
# "Create a task to implement rate limiting on the API endpoints"
# → Created task-xyz: Implement rate limiting on API endpoints
```

**Implementation:**
- Use system speech recognition (macOS Dictation, Windows Speech)
- Or integrate Whisper for cross-platform

#### 7.3 Status At-a-Glance

```bash
# One-line status
$ drover status --oneline
🟢 4 running | 📋 12 queued | ✅ 28 done | 🔴 2 blocked

# Auto-refreshing watch mode
$ drover watch
┌─────────────────────────────────────────┐
│ Drover Status (auto-refresh 2s)         │
├─────────────────────────────────────────┤
│ Running:  4/4 workers                   │
│ Progress: ████████████░░░░░░ 65%        │
│ ETA:      ~45 minutes                   │
└─────────────────────────────────────────┘
```

#### 7.4 Shell Prompt Integration

```bash
# For PS1
export PS1='$(drover prompt) \$ '

# For Starship
# ~/.config/starship.toml
[custom.drover]
command = "drover prompt --format starship"
when = "test -f .drover/config.yaml"
```

Output: `🐂 4/16 ✓28`

---

## 8. Dependency Graph

```
EPIC-001: Worktree Pre-warming
├── TASK-001 (Pool data structure) ─┐
├── TASK-002 (Pool init) ───────────┼─► TASK-003, TASK-004, TASK-005, TASK-006
├── TASK-005 + TASK-006 ────────────┴─► TASK-007
└── TASK-008 (Async fetch) ──────────► TASK-009, TASK-010

EPIC-002: Observability (depends on OTel foundation)
├── TASK-011 (Completion rate) ──────► TASK-012, TASK-013, TASK-014
├── TASK-014 ────────────────────────► TASK-015, TASK-016
└── TASK-017 (WebSocket) ────────────► TASK-018, TASK-019, TASK-035, TASK-040

EPIC-003: Sub-Tasks
├── TASK-020 (API design) ───────────► TASK-021, TASK-023
├── TASK-021 ────────────────────────► TASK-022, TASK-026
├── TASK-023 ────────────────────────► TASK-024 ──► TASK-025
└── TASK-026 ────────────────────────► TASK-027 ──► TASK-028

EPIC-004: Human-in-the-Loop
├── TASK-029 (Pause) ────────────────► TASK-030 ──► TASK-031
├── TASK-032 (Guidance queue) ───────► TASK-033 ──► TASK-034
├── TASK-029 + TASK-017 ─────────────► TASK-035 ──► TASK-036, TASK-037
└── TASK-033 + TASK-035 ─────────────► TASK-036

EPIC-005: Multiplayer
├── TASK-038 (Export) ───────────────► TASK-039
├── TASK-017 ────────────────────────► TASK-040
└── TASK-041 (Operator field) ───────► TASK-042, TASK-043

EPIC-006: CLI
├── TASK-044 (Quick command) ────────► TASK-045, TASK-046
└── TASK-047 (Oneline status) ───────► TASK-048, TASK-049
```

**Critical Path:** TASK-001 → TASK-002 → TASK-005/006 → TASK-007 (21h)

**Independent Starting Points:**
- TASK-001, TASK-008, TASK-017, TASK-020, TASK-029, TASK-032, TASK-038, TASK-041, TASK-044, TASK-047

---

## 9. Implementation Phases

### Phase 1: Quick Wins (Week 1-2)
Focus on independent, high-value tasks:

| Task | Description | Value |
|------|-------------|-------|
| TASK-047 | `drover status --oneline` | Immediate UX improvement |
| TASK-044 | `drover quick` command | Lower barrier to entry |
| TASK-017 | WebSocket event stream | Foundation for live features |
| TASK-041 | Operator field on tasks | Data model prep for multiplayer |

### Phase 2: Pre-warming Foundation (Week 2-4)
Build the worktree pool:

| Task | Description | Value |
|------|-------------|-------|
| TASK-001 → TASK-004 | Pool manager | Core infrastructure |
| TASK-005, TASK-006 | Dependency caching | Disk/time savings |
| TASK-008, TASK-009 | Optimistic reading | Further latency reduction |

### Phase 3: Observability (Week 4-6)
Build on OTel foundation:

| Task | Description | Value |
|------|-------------|-------|
| TASK-011 → TASK-013 | Success metrics | Data-driven improvements |
| TASK-014 → TASK-016 | Worker efficiency | Identify bottlenecks |
| TASK-018, TASK-019 | Activity feed | Real-time visibility |

### Phase 4: Advanced Features (Week 6+)
Based on learnings from phases 1-3:

- Epic 3: Sub-tasks (if task decomposition is a pain point)
- Epic 4: Human intervention (if tasks frequently get stuck)
- Epic 5: Multiplayer (if team usage grows)

---

## 10. Risks & Mitigations

| Risk | Impact | Likelihood | Mitigation |
|------|--------|------------|------------|
| Worktree pool exhaustion | Workers idle | Medium | Auto-scale pool, alert on low availability |
| Dependency cache corruption | Build failures | Low | Hash validation, automatic rebuild |
| WebSocket connection drops | Missing events | Medium | Reconnect with replay from last event ID |
| Sub-task infinite loops | Resource exhaustion | Low | Max depth limit (default: 3), total sub-task cap |
| Pause/resume state corruption | Lost work | Medium | Checkpoint before pause, validate on resume |
| Multi-operator merge conflicts | Failed merges | Medium | Lock tasks during execution, sequential merges |

---

## Appendix A: Related Documents

- [Drover README.md](../README.md) - User documentation
- [Drover DESIGN.md](./DESIGN.md) - Core architecture
- [OTel Integration Design](../scripts/telemetry/README.md) - Observability foundation
- [Ramp Inspect Blog Post](https://builders.ramp.com/post/why-we-built-our-background-agent) - Inspiration source

---

## Appendix B: Open Questions

1. **Pool size heuristics** - Should pool size auto-scale based on queue depth?
2. **Sub-task billing** - How do we track token usage across parent/child tasks?
3. **Multiplayer conflict resolution** - What happens if two operators add conflicting tasks?
4. **Voice input privacy** - Should voice processing be local-only?

---

*Document generated: January 2026*
*Next review: After Phase 1 completion*
