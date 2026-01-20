# Drover Worker CLI Design

## Overview

The `drover-worker` binary is a process-isolated task executor that runs Claude Code in a separate OS process. This ensures memory is reclaimed when tasks complete.

## CLI Interface

### Command Syntax

```bash
drover-worker execute \
  --task-id <string> \
  --worktree <path> \
  --title <string> \
  --description <string> \
  [--epic-id <string>] \
  [--guidance <string>] \
  [--timeout <duration>] \
  [--claude-path <path>] \
  [--verbose] \
  [--memory-limit <string>]
```

### Flags

| Flag | Required | Description | Example |
|------|----------|-------------|---------|
| `--task-id` | Yes | Unique task identifier | `task-123` |
| `--worktree` | Yes | Path to git worktree for execution | `/tmp/drover/worktrees/task-123` |
| `--title` | Yes | Task title | `"Fix authentication bug"` |
| `--description` | Yes | Task description | `"Users cannot login..."` |
| `--epic-id` | No | Parent epic ID | `epic-456` |
| `--guidance` | No | JSON array of guidance messages | `[{"id":"g1","message":"..."}]` |
| `--timeout` | No | Task timeout (default: 30m) | `1h`, `30m`, `45s` |
| `--claude-path` | No | Path to Claude binary (default: `claude`) | `/usr/local/bin/claude` |
| `--verbose` | No | Enable verbose logging | |
| `--memory-limit` | No | Worker memory limit (Linux cgroup) | `512M`, `2G` |

### Input Format (Alternative: STDIN)

Instead of CLI flags, task can be provided via stdin as JSON:

```json
{
  "id": "task-123",
  "title": "Fix authentication bug",
  "description": "Users cannot login with valid credentials",
  "epic_id": "epic-456",
  "worktree": "/tmp/drover/worktrees/task-123",
  "guidance": [
    {"id": "g1", "message": "Check the auth middleware"}
  ],
  "timeout": "30m",
  "claude_path": "claude",
  "verbose": true,
  "memory_limit": "512M"
}
```

Usage:
```bash
cat task.json | drover-worker execute -
```

## Output Format

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Task execution failed |
| 2 | Invalid input |
| 124 | Timeout |
| 137 | OOM killed |

### STDOUT JSON Result

```json
{
  "success": true,
  "task_id": "task-123",
  "output": "Full Claude Code output...",
  "error": null,
  "duration_ms": 45230,
  "signal": "ok",
  "verdict": "pass",
  "verdict_reason": "Task completed successfully"
}
```

### Worker Signals (for Backpressure)

The `signal` field indicates downstream health:

| Signal | Meaning | Concurrency Action |
|--------|---------|-------------------|
| `ok` | Normal execution | Maintain/increase |
| `rate_limited` | Rate limit detected | Decrease, apply backoff |
| `slow_response` | Response > threshold | Monitor, may decrease |
| `api_error` | Transient API error | May decrease |

### STDERR Protocol

#### Heartbeat Messages (for crash recovery)

Every 10 seconds, worker writes heartbeat to stderr:

```json
{"type":"heartbeat","task_id":"task-123","timestamp":1704067200}
```

#### Progress Updates

```json
{"type":"progress","task_id":"task-123","message":"Running tests..."}
```

#### Debug Messages (verbose mode)

```json
{"type":"debug","message":"Prompt length: 1234 chars"}
```

## Signal Detection

### Rate Limit Detection

Worker detects rate limiting by scanning Claude output for patterns:

```go
var rateLimitPatterns = []string{
    "pre-flight check is taking longer than expected",
    "rate limit",
    "too many requests",
    "429",
}
```

### Slow Response Detection

Response time measured from start to completion. Threshold configurable (default: 10s).

## Memory Limit Enforcement

### Linux (cgroup v2)

Using `systemd-run` wrapper:

```bash
systemd-run --scope \
  -p MemoryMax=512M \
  -p MemoryHigh=400M \
  --user \
  drover-worker execute ...
```

### Direct cgroup (alternative)

```go
// Set memory limit via cgroup v2
func setMemoryLimit(pid int, limit int64) error {
    // Write to /sys/fs/cgroup/user.slice/user-$(id -u).slice/memory.max
}
```

### Portable Fallback

Set `RLIMIT_AS` via `syscall.Setrlimit`:

```go
var rlimit syscall.Rlimit
rlimit.Cur = limit
rlimit.Max = limit
syscall.Setrlimit(syscall.RLIMIT_AS, &rlimit)
```

## Example Usage

### Basic Execution

```bash
drover-worker execute \
  --task-id task-123 \
  --worktree /tmp/drover/wt-task-123 \
  --title "Add login feature" \
  --description "Implement OAuth2 login"
```

### With Memory Limit

```bash
drover-worker execute \
  --task-id task-123 \
  --worktree /tmp/drover/wt-task-123 \
  --title "Add login feature" \
  --description "Implement OAuth2 login" \
  --memory-limit 512M
```

### Via JSON Input

```bash
echo '{
  "id": "task-123",
  "title": "Add login feature",
  "description": "Implement OAuth2 login",
  "worktree": "/tmp/drover/wt-task-123"
}' | drover-worker execute -
```

## Implementation Structure

```
internal/worker/
├── DESIGN.md          # This document
├── cli.go             # CLI flag parsing
├── executor.go        # Claude execution logic
├── signal.go          # Signal detection
├── heartbeat.go       # Heartbeat protocol
├── result.go          # Result formatting
└── memory.go          # Memory limit enforcement

cmd/drover-worker/
└── main.go            # Entry point
```
