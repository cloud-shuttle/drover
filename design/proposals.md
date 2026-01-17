# Drover Enhancement Proposals

This document consolidates all proposed enhancements to Drover. Features are organized by epic with implementation status and effort estimates.

## Table of Contents

1. [Status Legend](#status-legend)
2. [Epic 1: Event Streaming](#epic-1-event-streaming)
3. [Epic 2: Project Configuration](#epic-2-project-configuration)
4. [Epic 3: Context Window Management](#epic-3-context-window-management)
5. [Epic 4: Structured Task Outcomes](#epic-4-structured-task-outcomes)
6. [Epic 5: Enhanced CLI Controls](#epic-5-enhanced-cli-controls)
7. [Epic 6: Task Context Carrying](#epic-6-task-context-carrying)
8. [Epic 7: Worktree Pre-warming](#epic-7-worktree-pre-wamping-caching)
9. [Epic 8: Enhanced Observability Dashboard](#epic-8-enhanced-observability-dashboard)
10. [Epic 9: Human-in-the-Loop](#epic-9-human-in-the-loop-intervention)
11. [Epic 10: Multiplayer Support](#epic-10-session-handoff--multiplayer)

---

## Status Legend

| Status | Description |
|--------|-------------|
| **Proposed** | Design complete, awaiting implementation |
| **In Progress** | Currently being implemented |
| **Completed** | Feature has been implemented and released |

---

## Epic 1: Event Streaming

**Status:** Proposed | **Effort:** 1-2 weeks

Add real-time JSONL event streaming for task lifecycle events, enabling external integrations, notifications, and custom tooling.

### Motivation

- Enable Slack/Discord notifications for task completions
- Support custom dashboards and monitoring
- Allow integration with external CI/CD systems
- Provide audit trail for task execution

### Design

#### Event Types

```go
type EventType string

const (
    EventTaskStarted   EventType = "task.started"
    EventTaskCompleted EventType = "task.completed"
    EventTaskFailed    EventType = "task.failed"
    EventTaskBlocked   EventType = "task.blocked"
    EventTaskUnblocked EventType = "task.unblocked"
    EventTaskCancelled EventType = "task.cancelled"
)
```

#### CLI Interface

```bash
# Stream all events
drover stream

# Filter by event type
drover stream --type completed,failed

# Filter by epic
drover stream --epic epic-abc

# Include historical events
drover stream --since 2024-01-01T00:00:00Z
```

### Stories

| Story | Tasks | Effort |
|-------|-------|--------|
| Core Event Bus | 3 | 3 days |
| Stream Command | 3 | 2 days |

**Total:** 6 tasks, ~1 week

### Dependencies

- None (standalone)

---

## Epic 2: Project Configuration

**Status:** Proposed | **Effort:** 1 week

Support `.drover.toml` per-project configuration for task guidelines, worker constraints, and agent preferences.

### Motivation

- Different projects have different coding standards
- Some projects need more/fewer parallel workers
- Agent prompts should reflect project context
- Teams can customize behavior without global changes

### Design

#### Configuration Schema

```toml
# .drover.toml

# Agent configuration
agent = "claude-code"
max_workers = 4
task_timeout = "30m"

# Context settings
task_context_count = 5

# Size thresholds
max_description_size = "250KB"
max_diff_size = "250KB"

# Project-specific guidelines
guidelines = """
This is a Go project using DBOS for durability.
- Follow Go idioms and conventions
- Use structured logging with slog
"""

# Labels to apply to all tasks
default_labels = ["drover", "go"]
```

### Stories

| Story | Tasks | Effort |
|-------|-------|--------|
| Configuration Schema | 3 | 2 days |
| Task Guidelines Integration | 2 | 3 days |

**Total:** 5 tasks, ~1 week

### Dependencies

- Required by: Epic 3, Epic 6

---

## Epic 3: Context Window Management

**Status:** Proposed | **Effort:** 1 week

Intelligently manage large content to prevent context overflow, passing references instead of content when appropriate.

### Motivation

- Large diffs can exceed context windows
- Attached files may be too large to inline
- Agents can fetch content themselves when needed
- Prevents silent truncation or failures

### Design

#### Size Thresholds

```go
type ContentThresholds struct {
    MaxDescriptionSize ByteSize // Default: 250KB
    MaxDiffSize       ByteSize // Default: 250KB
    MaxFileSize       ByteSize // Default: 100KB
}
```

#### Reference Format

When content exceeds thresholds, replace with references:

```markdown
| Type | Path/SHA | Size | Fetch Command |
|------|----------|------|---------------|
| file | internal/worker/executor.go | 45KB | `cat internal/worker/executor.go` |
| diff | abc123 | 300KB | `git show abc123` |
```

### Stories

| Story | Tasks | Effort |
|-------|-------|--------|
| Content Size Detection | 2 | 2 days |
| Reference-Based Fallback | 2 | 3 days |

**Total:** 4 tasks, ~1 week

### Dependencies

- Requires: Epic 2 (configuration)

---

## Epic 4: Structured Task Outcomes

**Status:** Proposed | **Effort:** 1-2 weeks

Parse agent responses to extract structured completion status, improving blocker detection and reporting.

### Motivation

- Quick visual triage in TUI
- Better blocker detection
- Automated success validation
- Metrics and reporting

### Design

#### Verdict Types

```go
type Verdict string

const (
    VerdictPass    Verdict = "pass"
    VerdictFail    Verdict = "fail"
    VerdictBlocked Verdict = "blocked"
    VerdictUnknown Verdict = "unknown"
)
```

### Stories

| Story | Tasks | Effort |
|-------|-------|--------|
| Success Criteria Definition | 2 | 2 days |
| Verdict Extraction | 3 | 5 days |

**Total:** 5 tasks, ~1 week

### Dependencies

- None (standalone)

---

## Epic 5: Enhanced CLI Controls

**Status:** Proposed | **Effort:** 1 week

Add CLI commands for fine-grained job control: cancel, retry, and manual blocker resolution.

### Motivation

- Stop runaway tasks
- Retry transient failures
- Manually resolve blockers
- Better operational control

### Design

#### Command Interface

```bash
# Cancel a running task
drover cancel <task-id>

# Retry a failed task
drover retry <task-id> --force

# Manually resolve a blocked task
drover resolve <task-id> --note "Fixed manually"
```

### Stories

| Story | Tasks | Effort |
|-------|-------|--------|
| Cancel Command | 3 | 2 days |
| Retry Command | 2 | 2 days |
| Resolve Command | 3 | 3 days |

**Total:** 6 tasks, ~1 week

### Dependencies

- None (standalone)

---

## Epic 6: Task Context Carrying

**Status:** Proposed | **Effort:** 1 week

Include context from recent completed tasks to give agents memory of recent decisions and patterns.

### Motivation

- Agents make more consistent decisions
- Patterns from recent work inform current task
- Reduces repeated questions
- Better project coherence

### Design

#### Context Format

```markdown
## Recent Task Context

### T122: Implement worker pool (Pass)
*Completed 2 hours ago*
> Implemented worker pool with configurable size.

### T121: Add database migrations (Pass)
*Completed 5 hours ago*
> Created migration system using golang-migrate.

---

## Current Task

[Original task description...]
```

### Stories

| Story | Tasks | Effort |
|-------|-------|--------|
| Context Configuration | 1 | 1 day |
| Context Injection | 3 | 4 days |

**Total:** 4 tasks, ~1 week

### Dependencies

- Requires: Epic 2 (configuration)

---

## Epic 7: Worktree Pre-warming & Caching

**Status:** Proposed | **Effort:** 2-3 weeks

Maintain a pool of pre-initialized worktrees ready for immediate assignment, reducing cold-start time.

### Motivation

- Currently: 15-60s cold-start per task
- Target: <3s to first task execution
- Reduce dependency installation overhead
- Improve worker utilization

### Design

#### Worktree Pool

```
┌─────────────────────────────────────┐
│         Worktree Pool               │
│  ┌─────┐ ┌─────┐ ┌─────┐ ┌─────┐   │
│  │warm │ │warm │ │in-  │ │cold │   │
│  │     │ │     │ │use  │ │     │   │
│  └─────┘ └─────┘ └─────┘ └─────┘   │
└─────────────────────────────────────┘
```

### Stories

| Story | Tasks | Effort |
|-------|-------|--------|
| Pool Manager | 4 | 5 days |
| Dependency Caching | 2 | 4 days |
| Background Sync | 2 | 3 days |

**Total:** 8 tasks, ~2 weeks

### Dependencies

- None (standalone)

---

## Epic 8: Enhanced Observability Dashboard

**Status:** Proposed | **Effort:** 2-3 weeks

Build on existing OpenTelemetry integration to add task success metrics, worker efficiency tracking, and live activity feeds.

### Motivation

- Track success/failure rates over time
- Identify bottlenecks with worker efficiency
- Real-time visibility into task execution
- Historical trend analysis

### Design

#### Dashboard Components

- Task success rate by epic
- Time-to-completion histograms
- Worker utilization metrics
- Live activity feed (WebSocket)
- Failure pattern clustering

### Stories

| Story | Tasks | Effort |
|-------|-------|--------|
| Task Success Metrics | 3 | 4 days |
| Worker Efficiency | 3 | 4 days |
| Live Activity Feed | 3 | 5 days |

**Total:** 9 tasks, ~2 weeks

### Dependencies

- Requires: Existing OpenTelemetry integration

---

## Epic 9: Human-in-the-Loop Intervention

**Status:** Proposed | **Effort:** 2-3 weeks

Enable task pause/resume and guidance injection for running tasks.

### Motivation

- Correct tasks going down wrong path
- Provide hints without stopping execution
- Preserve work when intervention needed
- Reduce wasted iterations

### Design

#### Command Interface

```bash
# Pause a running task
drover pause <task-id>

# Resume with guidance
drover resume <task-id> --hint "Try using the existing auth middleware"

# Send hint without pausing
drover hint <task-id> "The token is in localStorage"
```

### Stories

| Story | Tasks | Effort |
|-------|-------|--------|
| Task Pause/Resume | 3 | 5 days |
| Guidance Injection | 2 | 4 days |
| Dashboard UI | 3 | 5 days |

**Total:** 8 tasks, ~2 weeks

### Dependencies

- Requires: Epic 8 (WebSocket events)

---

## Epic 10: Session Handoff & Multiplayer

**Status:** Proposed | **Effort:** 2-3 weeks

Enable session export/import and multi-operator attribution.

### Motivation

- Hand off work between team members
- Track who made which changes
- Collaborative project execution
- Attribution for commits and changes

### Design

#### Session Export/Import

```bash
# Export session
drover export --output session.drover

# Import on another machine
drover import session.drover --continue
```

#### Multi-Operator Attribution

```go
type Task struct {
    // ... existing fields
    CreatedBy   string // Operator who created
    ModifiedBy  string // Last operator to modify
    CompletedBy string // Operator whose worker completed
}
```

### Stories

| Story | Tasks | Effort |
|-------|-------|--------|
| Session Export/Import | 2 | 4 days |
| Multi-Operator Attribution | 2 | 3 days |
| Live Session Sharing | 2 | 4 days |

**Total:** 6 tasks, ~2 weeks

### Dependencies

- None (standalone)

---

## Implementation Phases

### Phase 1: Quick Wins (Week 1-2)

Independent, high-value tasks:
- Epic 2: Project Configuration (foundational)
- Epic 5: CLI Controls (high value)

### Phase 2: Pre-warming (Week 2-4)

- Epic 7: Worktree Pre-warming

### Phase 3: Observability (Week 4-6)

- Epic 8: Enhanced Dashboard

### Phase 4: Intelligence (Week 6+)

Based on learnings from Phases 1-3:
- Epic 1: Event Streaming
- Epic 3: Context Management
- Epic 4: Structured Outcomes
- Epic 6: Task Context

### Phase 5: Collaboration (Week 8+)

- Epic 9: Human-in-the-Loop
- Epic 10: Multiplayer

---

## Related Documents

- [Design Overview](DESIGN.md) - Core architecture
- [Architecture Diagrams](architecture.md) - System architecture
- [Documentation Index](../docs/index.md) - Complete documentation index

---

*Last updated: 2026-01-16*
