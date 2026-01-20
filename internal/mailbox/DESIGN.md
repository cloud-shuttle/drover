# File-Based Task Mailbox Design

## Overview

The File-Based Task Mailbox provides an alternative to the in-memory/SQLite task queue. Tasks are stored as files in a directory structure, providing crash resilience and decoupling coordination from execution.

## Directory Structure

```
/tmp/drover/mailbox/
├── inbox/           # New tasks ready to be claimed
├── processing/      # Tasks currently being worked on
├── outbox/          # Successfully completed tasks
├── failed/          # Tasks that failed execution
└── .tmp/            # Temporary files for atomic operations
```

## File Format

### Task File JSON Schema

```json
{
  "id": "task_1234567890",
  "title": "Implement feature X",
  "description": "Add feature X with proper error handling",
  "epic_id": "epic_abc123",
  "parent_id": "",
  "sequence_number": 0,
  "type": "feature",
  "priority": 1,
  "status": "ready",
  "attempts": 0,
  "max_attempts": 3,
  "last_error": "",
  "claimed_by": "",
  "claimed_at": 0,
  "operator": "",
  "verdict": "unknown",
  "verdict_reason": "",
  "test_mode": "strict",
  "test_scope": "diff",
  "test_command": "",
  "created_at": 1705862400,
  "updated_at": 1705862400,
  "metadata": {
    "worktree": "/path/to/worktree",
    "dependencies": []
  }
}
```

### Result File (in outbox/)

```json
{
  "task_id": "task_1234567890",
  "status": "completed",
  "verdict": "pass",
  "verdict_reason": "Successfully implemented feature",
  "output": "Task output here...",
  "duration_ms": 45000,
  "completed_at": 1705862445
}
```

### Error File (in failed/)

```json
{
  "task_id": "task_1234567890",
  "status": "failed",
  "error": "execution failed: timeout",
  "attempts": 3,
  "last_attempt_at": 1705862500,
  "failed_at": 1705862500
}
```

## Atomic Operations

### Enqueue (inbox/)

1. Write task JSON to `.tmp/{task_id}.json.tmp`
2. `rename()` to `inbox/{task_id}.json`
3. If rename fails (file exists), return error

### Claim (inbox/ → processing/)

1. `readdir()` inbox/
2. For each task file:
   - Try `rename(inbox/{task_id}.json, processing/{task_id}.json)`
   - If successful, return task
   - If ENOENT, another worker claimed it, continue
3. If no tasks remain, return ErrNoTasks

### Complete (processing/ → outbox/)

1. Write result to `.tmp/{task_id}_result.json.tmp`
2. `rename()` to `outbox/{task_id}_result.json`
3. `unlink(processing/{task_id}.json)`

### Fail (processing/ → failed/)

1. Write error info to `.tmp/{task_id}_error.json.tmp`
2. `rename()` to `failed/{task_id}_error.json`
3. `unlink(processing/{task_id}.json)`

## Concurrency Safety

- `rename()` is atomic on POSIX systems
- Multiple workers can safely claim from inbox/
- Each worker gets unique tasks (no double-claim)
- Crashed workers leave tasks in processing/ (recovered by periodic scan)

## Cleanup Routine

Periodic cleanup of old files:
- outbox/: Remove after 7 days (configurable)
- failed/: Remove after 30 days (configurable)
- .tmp/: Remove files older than 1 hour (stale tmp files)

## Configuration

Environment variables:
- `DROVER_MAILBOX_DIR`: Directory path (default: `/tmp/drover/mailbox`)
- `DROVER_MAILBOX_ENABLED`: Enable file mailbox (default: `false`)
- `DROVER_MAILBOX_OUTBOX_RETENTION`: Days to keep outbox files (default: `7`)
- `DROVER_MAILBOX_FAILED_RETENTION`: Days to keep failed files (default: `30`)

## Error Codes

- `ErrMailboxDir`: Mailbox directory doesn't exist or isn't writable
- `ErrTaskExists`: Task file already exists in inbox
- `ErrNoTasks`: No tasks available to claim
- `ErrTaskNotFound`: Task file not found
- `ErrInvalidTask`: Task file has invalid JSON
