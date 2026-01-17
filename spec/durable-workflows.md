# Durable Workflows Specification

**Status:** ✅ Implemented | **Since:** v0.1.0 | **Epic:** Core

## Overview

Drover uses [DBOS (Durable Operating System)](https://dbos.dev/) as its primary workflow orchestration engine. DBOS provides automatic crash recovery, exactly-once execution semantics, and built-in queue management.

## Key Features

- **Automatic crash recovery** - Survive process crashes and restarts
- **Exactly-once execution** - Operations are checkpointed, never duplicated
- **SQLite support** - Zero-config development with embedded database
- **PostgreSQL support** - Production-grade database with `DBOS_SYSTEM_DATABASE_URL`
- **Queue-based execution** - Built-in concurrency control and backpressure

## Architecture

```
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
```

## Database Configuration

### Development (SQLite)

Default configuration, no setup required:

```bash
drover run
# Uses .drover/drover.db automatically
```

### Production (PostgreSQL)

Set environment variable:

```bash
export DROVER_DATABASE_URL="postgresql://user:pass@localhost/drover"
drover run
```

Or use DBOS connection string:

```bash
export DBOS_SYSTEM_DATABASE_URL="postgresql://user:pass@localhost:5432/drover"
drover run
```

## Workflow Types

### DroverRunWorkflow

Main orchestration loop that runs until all tasks complete.

```go
func DroverRunWorkflow(ctx dbos.FunctionContext, config RunConfig) RunResult {
    for {
        // 1. Find ready tasks
        tasks := findReadyTasks()

        // 2. Claim tasks atomically
        for _, task := range tasks {
            claimTask(ctx, task)
            enqueueTask(ctx, task)
        }

        // 3. Durable sleep (survives restart)
        dbos.Sleep(ctx, 2*time.Second)

        // 4. Check completion
        if allTasksComplete() {
            break
        }
    }
    return result
}
```

### ExecuteTaskWorkflow

Per-task workflow executed by workers.

```go
func ExecuteTaskWorkflow(ctx dbos.FunctionContext, taskID string) error {
    // 1. Mark task in_progress
    updateTaskStatus(ctx, taskID, "in_progress")

    // 2. Create worktree
    worktreePath := createWorktree(ctx, taskID)

    // 3. Execute agent (Claude Code)
    err := executeAgent(ctx, worktreePath, task)

    // 4. Commit and merge
    if err == nil {
        commitAndMerge(ctx, worktreePath)
        updateTaskStatus(ctx, taskID, "completed")
    } else {
        updateTaskStatus(ctx, taskID, "failed")
    }

    // 5. Cleanup
    removeWorktree(ctx, worktreePath)

    // 6. Unblock dependents
    unblockDependents(ctx, taskID)

    return err
}
```

## Crash Recovery

DBOS automatically recovers workflows from the last checkpointed step.

```bash
# Running...
drover run

# ^C (interrupted) or crash

# Resume - continues from exact point
drover resume
```

### Recovery Flow

1. DBOS loads workflow state from database
2. Resumes from last completed step
3. Partially completed tasks don't restart
4. In-flight operations are detected and handled

## Queue Management

DBOS queues manage worker concurrency:

```go
queue := dbos.NewWorkflowQueue(ctx, "drover-tasks",
    dbos.QueueConcurrency(config.Workers),
)
```

### Queue Properties

- **Concurrency limit** - At most N tasks execute simultaneously
- **Backpressure** - Queue fills when all workers busy
- **Fair scheduling** - FIFO ordering with priority support

## State Persistence

### Checkpoints

Each workflow step is checkpointed before execution:

```sql
-- DBOS manages these tables
dbos_workflow_status   -- Workflow execution state
dbos_workflow_inputs   -- Workflow input arguments
dbos_workflow_outputs  -- Step return values
dbos_workflow_notifications  -- Inter-workflow communication
```

### Task State

Task state is stored in application tables:

```sql
CREATE TABLE tasks (
    id TEXT PRIMARY KEY,
    status TEXT,  -- ready, claimed, in_progress, blocked, completed, failed
    attempts INT,
    last_error TEXT,
    created_at INTEGER,
    updated_at INTEGER
);
```

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DROVER_DATABASE_URL` | `sqlite:///.drover.db` | Database connection |
| `DBOS_SYSTEM_DATABASE_URL` | (from DROVER_DATABASE_URL) | DBOS database |
| `DROVER_ENV` | `development` | Environment name |

### Run Configuration

```go
type RunConfig struct {
    Workers      int           // Number of parallel workers
    EpicID       string        // Optional: filter by epic
    ContinueOnFailure bool     // Continue after task failures
    MaxAttempts   int           // Maximum retry attempts per task
}
```

## Error Handling

### Automatic Retry

Tasks retry automatically on transient failures:

```go
if attempts < maxAttempts {
    // Re-enqueue for retry
    updateTaskStatus(taskID, "ready")
    attempts++
}
```

### Max Retries Exceeded

```go
if attempts >= maxAttempts {
    // Mark as failed
    updateTaskStatus(taskID, "failed")
    lastError = err.Error()
}
```

## Performance Considerations

### Scaling

| Workers | Throughput | Database Load |
|---------|------------|---------------|
| 1-4 | Low | Minimal |
| 4-8 | Medium | Light |
| 8-16 | High | Moderate |

### Bottlenecks

1. **Database connections** - Each worker needs a connection
2. **Disk I/O** - Worktree creation/merge are I/O heavy
3. **Agent execution** - LLM API latency dominates

### Optimizations

- Connection pooling via DBOS
- Lazy worktree creation
- Batch dependency resolution
- Incremental status updates

## See Also

- [Architecture Diagrams](../design/architecture.md)
- [State Machine](../design/state-machine.md)
- [Sequence Diagrams](../design/sequence.md)
- [DBOS Documentation](https://docs.dbos.dev/)

---

*Last updated: 2026-01-16*
