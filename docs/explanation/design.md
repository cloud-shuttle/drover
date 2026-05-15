# Drover Design Document

## Overview

Drover is a durable workflow orchestrator for parallel AI agent execution. It coordinates multiple Claude Code instances to complete project tasks while handling failures, dependencies, and resource contention.

**Primary Workflow Engine**: Drover uses **DBOS (Durable Operating System for Workflows)** as its primary orchestration engine. DBOS provides:
- Automatic crash recovery and checkpointing
- Built-in retry logic for failed operations
- Exactly-once execution guarantees
- Queue-based parallel execution with concurrency control

**Database Configuration**:
- **Development**: SQLite (default, zero setup)
- **Production**: PostgreSQL (via `DBOS_SYSTEM_DATABASE_URL`)

## Design Goals

1. **Durability** — Never lose progress, survive any failure
2. **Parallelism** — Maximize throughput with concurrent agents
3. **Correctness** — Respect dependencies, avoid conflicts
4. **Simplicity** — Minimal configuration, sensible defaults
5. **Observability** — Clear visibility into progress and issues (OpenTelemetry integration)

## Non-Goals

- Real-time collaboration (agents work independently)
- Custom AI models (Claude Code only)
- Distributed execution (single machine, multiple workers)
- IDE integration (CLI-first)

---

## Core Concepts

### Tasks

A task is the atomic unit of work. Each task:

- Has a unique ID, title, and optional description
- Belongs to zero or one epic
- Can depend on other tasks (blocked-by relationship)
- Has a priority (higher = more urgent)
- Tracks execution attempts and errors

```go
type Task struct {
    ID          string
    Title       string
    Description string
    EpicID      string
    Priority    int
    Status      TaskStatus  // ready, claimed, in_progress, blocked, completed, failed
    Attempts    int
    MaxAttempts int
    LastError   string
}
```

### Epics

An epic groups related tasks. Epics provide:

- Logical organization
- Filtered execution (`drover run --epic X`)
- Progress tracking at the feature level

### Dependencies

Tasks can declare dependencies via `blocked-by` relationships:

```
Task A ──blocked-by──► Task B
        ──blocked-by──► Task C
```

Task A remains in `blocked` status until both B and C are `completed`. When the last blocker completes, A automatically transitions to `ready`.

### Workers

A worker is a goroutine that:

1. Claims a ready task (atomic operation)
2. Creates an isolated git worktree
3. Executes Claude Code with the task prompt
4. Commits changes and merges to main
5. Marks task complete and unblocks dependents

Workers are managed by a DBOS queue with concurrency limits.

---

## System Architecture

### Component Stack

```
┌─────────────────────────────────────────────────────────────┐
│                         CLI (Cobra)                          │
│  drover init | run | add | epic | status | resume            │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                    DBOS Workflow Engine                      │
│  ┌─────────────────┐    ┌─────────────────────────────────┐ │
│  │ DroverRunWorkflow│    │ ExecuteTaskWorkflow             │ │
│  │ (orchestrator)   │───►│ (per-task, queued)              │ │
│  └─────────────────┘    └─────────────────────────────────┘ │
│           │                           │                      │
│           │    ┌──────────────────────┘                      │
│           │    │                                             │
│           ▼    ▼                                             │
│  ┌─────────────────────────────────────────────────────────┐│
│  │              DBOS Queue (concurrency=workers)            ││
│  │  Controls parallel execution, handles backpressure       ││
│  └─────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────┘
                              │
              ┌───────────────┼───────────────┐
              ▼               ▼               ▼
┌───────────────────┐ ┌───────────────┐ ┌───────────────┐
│   Git Worktree 1  │ │ Git Worktree 2│ │ Git Worktree N│
│  ┌─────────────┐  │ │ ┌───────────┐ │ │ ┌───────────┐ │
│  │ Claude Code │  │ │ │Claude Code│ │ │ │Claude Code│ │
│  └─────────────┘  │ │ └───────────┘ │ │ └───────────┘ │
└───────────────────┘ └───────────────┘ └───────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                   PostgreSQL / SQLite                        │
│  ┌──────────┐ ┌──────────┐ ┌──────────────┐ ┌────────────┐  │
│  │  tasks   │ │  epics   │ │ dependencies │ │ dbos_state │  │
│  └──────────┘ └──────────┘ └──────────────┘ └────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

### Data Flow

```
┌────────────────────────────────────────────────────────────────┐
│                                                                │
│  ┌──────┐    ┌──────┐    ┌──────┐    ┌──────┐    ┌──────────┐ │
│  │ready │───►│claimed│───►│in_   │───►│compl-│───►│ unblock  │ │
│  │      │    │      │    │progr-│    │eted  │    │dependents│ │
│  └──────┘    └──────┘    │ess   │    └──────┘    └──────────┘ │
│     ▲                    └──┬───┘                      │       │
│     │                       │                          │       │
│     │         ┌─────────────┼─────────────┐           │       │
│     │         ▼             ▼             ▼           │       │
│     │    ┌────────┐   ┌──────────┐   ┌────────┐       │       │
│     │    │blocked │   │ failed   │   │ retry  │       │       │
│     │    │        │   │(max tries│   │        │       │       │
│     │    └────┬───┘   └──────────┘   └───┬────┘       │       │
│     │         │                          │            │       │
│     └─────────┴──────────────────────────┘            │       │
│                        ▲                              │       │
│                        └──────────────────────────────┘       │
│                                                                │
└────────────────────────────────────────────────────────────────┘
```

---

## Key Design Decisions

### 1. DBOS for Durability

**Decision**: Use DBOS Go as the primary workflow orchestration engine.

**Rationale**:
- Battle-tested durable execution with automatic crash recovery
- Built-in queues with concurrency control
- Exactly-once execution semantics via checkpointing
- **SQLite support for local development** (zero setup)
- PostgreSQL for production (optional upgrade)

**Default Configuration**:
- Development: SQLite (`.drover/drover.db`) - no additional setup required
- Production: PostgreSQL via `DBOS_SYSTEM_DATABASE_URL` environment variable

**Trade-offs**:
- Additional dependency (mitigated by SQLite zero-config)
- Learning curve for DBOS patterns (well-documented)
- Production requires PostgreSQL (but dev works with SQLite)

### 2. Git Worktrees for Isolation

**Decision**: Each worker operates in its own git worktree.

**Rationale**:
- Complete filesystem isolation between workers
- No merge conflicts during parallel execution
- Easy cleanup on task completion
- Natural audit trail via git history

**Trade-offs**:
- Disk space overhead (full working copy per worker)
- Worktree management complexity
- Merge conflicts deferred to completion time

### 3. SQLite-Based Task State

**Decision**: Store task state in SQLite by default, with PostgreSQL for production.

**Rationale**:
- Atomic operations via transactions
- Consistent view across workers
- Native DBOS integration (workflow state + task state in same DB)
- Query flexibility for status and reporting
- **Zero setup for local development**

**Configuration**:
- Default: SQLite at `.drover/drover.db`
- Production: Set `DBOS_SYSTEM_DATABASE_URL` to PostgreSQL connection string

**Trade-offs**:
- State not visible in filesystem (but queryable via CLI)
- Schema migrations required for changes (handled by DBOS)

### 4. Single Orchestrator Workflow

**Decision**: One `DroverRunWorkflow` orchestrates all task execution.

**Rationale**:
- Single point of control for scheduling
- Easier to reason about concurrency
- Natural fit for DBOS durable workflow pattern
- Survives restarts cleanly

**Trade-offs**:
- Single point of failure (mitigated by DBOS durability)
- Potential bottleneck at high scale
- All state in one workflow context

### 5. Claude Code via Subprocess

**Decision**: Execute Claude Code as a subprocess, not embedded.

**Rationale**:
- Claude Code is a separate tool with its own runtime
- Clean separation of concerns
- Easy to swap AI backends in future
- No Node.js dependency in Go codebase

**Trade-offs**:
- Process spawn overhead
- Limited integration (stdout/stderr only)
- Error handling across process boundary

---

## Concurrency Model

### Task Claiming

Tasks are claimed atomically using database transactions:

```sql
UPDATE tasks
SET status = 'claimed', claimed_at = NOW()
WHERE id = ? AND status = 'ready'
```

Only one worker can successfully claim a task. Failed claims (0 rows affected) mean another worker got there first.

### Queue-Based Execution

DBOS queues manage worker concurrency:

```go
queue := dbos.NewWorkflowQueue(ctx, "drover-tasks",
    dbos.QueueConcurrency(config.Workers),
)
```

The queue ensures:
- At most N tasks execute simultaneously
- Backpressure when all workers busy
- Fair scheduling across enqueued tasks

### Dependency Resolution

When a task completes:

1. Query all tasks blocked by this one
2. For each blocked task, count remaining incomplete blockers
3. If count = 0, transition to `ready`

```sql
-- Find newly unblocked tasks
SELECT task_id FROM task_dependencies td
JOIN tasks t ON td.task_id = t.id
WHERE td.blocked_by = ? AND t.status = 'blocked'
GROUP BY task_id
HAVING COUNT(*) = (
    SELECT COUNT(*) FROM task_dependencies
    WHERE task_id = td.task_id
    AND blocked_by IN (SELECT id FROM tasks WHERE status = 'completed')
)
```

---

## Failure Handling

### Task Failure

1. **Retry**: If `attempts < max_attempts`, re-enqueue
2. **Fail**: If max retries exhausted, mark `failed`
3. **Block**: If failure indicates external blocker, mark `blocked`

### Workflow Failure

DBOS handles workflow failures automatically:

1. Each step is checkpointed before execution
2. On crash, `dbos.Launch()` recovers all incomplete workflows
3. Workflows resume from last completed step
4. Idempotent steps prevent duplicate side effects

### Worker Failure

If a worker crashes mid-task:

1. Task remains in `claimed` or `in_progress` status
2. Orchestrator detects stale claims (claimed_at > timeout)
3. Stale tasks returned to `ready` pool
4. Another worker picks up the task

---

## Database Schema

```sql
-- Epics group related tasks
CREATE TABLE epics (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    description TEXT,
    status TEXT DEFAULT 'open',
    created_at INTEGER NOT NULL
);

-- Tasks are the unit of work
CREATE TABLE tasks (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    description TEXT,
    epic_id TEXT,
    parent_id TEXT,                    -- For hierarchical sub-tasks
    sequence_number INTEGER DEFAULT 0, -- For ordering within parent
    priority INT DEFAULT 0,
    status TEXT DEFAULT 'ready',
    attempts INT DEFAULT 0,
    max_attempts INT DEFAULT 3,
    last_error TEXT,
    claimed_by TEXT,                   -- Worker that claimed this task
    claimed_at INTEGER,                -- Unix timestamp
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    FOREIGN KEY (epic_id) REFERENCES epics(id),
    FOREIGN KEY (parent_id) REFERENCES tasks(id) ON DELETE CASCADE
);

-- Dependencies define blocked-by relationships
CREATE TABLE task_dependencies (
    task_id TEXT NOT NULL,
    blocked_by TEXT NOT NULL,
    PRIMARY KEY (task_id, blocked_by),
    FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
    FOREIGN KEY (blocked_by) REFERENCES tasks(id) ON DELETE CASCADE
);

-- Worktrees track git worktree lifecycle for cleanup
CREATE TABLE worktrees (
    task_id TEXT PRIMARY KEY,
    path TEXT NOT NULL,
    branch TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    last_used_at INTEGER NOT NULL,
    status TEXT DEFAULT 'active',
    disk_size INTEGER DEFAULT 0,
    FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
);

-- Indexes for common queries
CREATE INDEX idx_tasks_status ON tasks(status);
CREATE INDEX idx_tasks_epic ON tasks(epic_id);
CREATE INDEX idx_tasks_parent ON tasks(parent_id);
CREATE INDEX idx_tasks_parent_seq ON tasks(parent_id, sequence_number);
CREATE INDEX idx_dependencies_blocked_by ON task_dependencies(blocked_by);
CREATE INDEX idx_worktrees_status ON worktrees(status);

-- DBOS manages its own tables for workflow state
-- dbos_workflow_status, dbos_workflow_inputs, etc.
```

---

## Performance Considerations

### Scaling

| Workers | Expected Throughput | Database Load |
|---------|---------------------|---------------|
| 1-4 | Low | Minimal |
| 4-8 | Medium | Light |
| 8-16 | High | Moderate |
| 16+ | Very High | Heavy (needs tuning) |

### Bottlenecks

1. **Database connections**: Each worker needs a connection
2. **Git operations**: Worktree creation/merge are I/O heavy
3. **Claude Code API**: Rate limits may apply
4. **Disk space**: Each worktree is a full copy

### Optimizations

- Connection pooling via DBOS
- Lazy worktree creation (on claim, not startup)
- Batch dependency resolution
- Incremental status updates (not full scan)

---

## Observability

### OpenTelemetry Integration

Drover provides built-in observability via OpenTelemetry:

**Traces** - Distributed tracing for operations:
- Workflow execution (root span)
- Task execution (child spans per task)
- Agent execution (Claude Code calls)
- Git operations (commit, merge)
- Error tracking with categorized errors

**Metrics** - Quantitative measurements:
- Counters: tasks claimed/completed/failed
- Histograms: task duration, agent duration
- Gauges: active workers, pending tasks
- Agent-specific: prompts sent, errors by type

**Storage**: ClickHouse for scalable trace/metric storage
**Visualization**: Grafana dashboards (included)

**Configuration**:
```bash
export DROVER_OTEL_ENABLED=true     # Enable (default: false)
export DROVER_OTEL_ENDPOINT=localhost:4317  # OTLP collector
```

### Trace Hierarchy

```
drover.workflow.run (root)
├── drover.task.execute
│   ├── drover.agent.execute (claude-code)
│   └── dbos.step (worktree, commit, merge)
└── drover.workflow.metrics
```

### Semantic Conventions

Drover uses OpenTelemetry semantic conventions:

| Attribute | Type | Description |
|-----------|------|-------------|
| `drover.task.id` | string | Task identifier |
| `drover.task.title` | string | Human-readable title |
| `drover.task.state` | string | ready/in_progress/completed/failed |
| `drover.worker.id` | string | Worker identifier |
| `drover.agent.type` | string | "claude-code" |
| `drover.epic.id` | string | Epic identifier |

See [scripts/telemetry/](../scripts/telemetry/) for full documentation.

---

## Security Considerations

### Code Execution

Claude Code executes arbitrary code. Drover does not sandbox this execution. Users should:

- Review generated code before merging to protected branches
- Use separate credentials for Drover execution
- Run in isolated environments for sensitive projects

### Database Access

- Use least-privilege database users
- Enable SSL for Postgres connections
- Rotate credentials regularly

### Git Operations

- Drover commits as configured git user
- Consider dedicated git identity for Drover
- Protect main branch with required reviews

---

## Future Considerations

### Potential Enhancements

1. **Web UI**: Real-time dashboard with task visualization
2. **Distributed execution**: Multiple machines coordinating
3. **Custom agents**: Support for other AI coding tools
4. **Beads integration**: Bidirectional sync with Beads format
5. **Webhooks**: Notifications on task completion/failure
6. ~~**Metrics**: Prometheus/OpenTelemetry integration~~ ✅ **Implemented**

### Known Limitations

1. Single machine only (no distributed workers)
2. Git-based projects only
3. Claude Code dependency
4. No real-time collaboration between agents
5. Limited conflict resolution (fail-fast on merge conflicts)

---

## Influences & Acknowledgments

Drover incorporates ideas and concepts from several innovative projects:

### Beads

**[Beads](https://github.com/beads-dev/beads)** heavily influenced Drover's task hierarchy design:

- **Hierarchical Task IDs** — Beads' `task-id.subtask` format inspired Drover's `task-123.1` syntax
- **Sub-task decomposition** — Breaking complex work into sequential, ordered pieces
- **Flat storage with hierarchy** — Hierarchical structure without nested complexity

Drover's sub-task system directly implements these patterns while adding durable workflow orchestration.

### Geoffrey Huntley's Ralph Wiggum

**[Geoffrey Huntley](https://github.com/gdhuntley)** articulated the "Ralph Wiggum" pattern:

- Delegating repetitive, well-defined tasks to AI agents
- Human provides direction and oversight
- Agents execute routine work at scale
- Focus on completion over perfection

This philosophy is central to Drover's design: break down projects, queue the work, and let agents drive it to completion.

### DBOS

**[DBOS](https://dbos.dev/)** provides the foundational workflow engine:

- Durable execution with automatic crash recovery
- Exactly-once semantics via checkpointing
- Built-in queues with concurrency control
- SQLite support for zero-config development

The [DBOS Go SDK](https://github.com/dbos-inc/dbos-transact-golang) makes these capabilities accessible to Go applications.

### Claude Code

**[Anthropic's Claude Code](https://claude.ai/code)** is the AI execution layer:

- Advanced code understanding and generation
- Tool use for file operations, testing, and more
- Stateful conversation context for complex tasks
- Human-in-the-loop oversight when needed

---

## Glossary

| Term | Definition |
|------|------------|
| **Task** | Atomic unit of work for an AI agent |
| **Epic** | Logical grouping of related tasks |
| **Worker** | Goroutine executing tasks via Claude Code |
| **Worktree** | Isolated git working directory |
| **Claim** | Atomic acquisition of a task by a worker |
| **Blocker** | Task that must complete before another can start |
| **Checkpoint** | DBOS-persisted workflow state for recovery |
| **Step** | Individual operation within a workflow |
