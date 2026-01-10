# Expected Telemetry Behavior

This document describes what you should expect to see when running Drover with OpenTelemetry enabled.

## When Telemetry is Disabled (Default)

```bash
./drover run
```

**Behavior:**
- No telemetry initialization messages
- No spans or metrics created
- Normal operation, no overhead
- No network connections to OTLP endpoint

## When Telemetry is Enabled

```bash
export DROVER_OTEL_ENABLED=true
export DROVER_OTEL_ENDPOINT=localhost:4317
./drover run
```

### Startup Behavior

If the OTLP endpoint is **reachable**:
```
✅ Drover starts normally
(no additional output - telemetry is silent)
```

If the OTLP endpoint is **unreachable**:
```
Warning: failed to initialize telemetry: connection refused
✅ Drover continues normally (graceful degradation)
```

### During Execution

When tasks execute, the following telemetry is generated:

#### 1. Spans Created

For each task execution:
```
drover.workflow.run (workflow root)
├── drover.task.execute
│   ├── dbos.step (createWorktreeStep)
│   ├── dbos.step (executeClaudeStep)
│   │   └── drover.agent.execute
│   ├── dbos.step (commitChangesStep)
│   └── dbos.step (mergeToMainStep)
└── drover.workflow.run completion
```

#### 2. Metrics Recorded

Counters incremented:
- `drover_tasks_claimed_total` - Each task is claimed
- `drover_agent_prompts_total` - Each agent prompt
- `drover_tasks_completed_total` - Each successful task
- `drover_tasks_failed_total` - Each failed task

Histograms recorded:
- `drover_task_duration_seconds` - Task execution time
- `drover_agent_duration_seconds` - Agent execution time

#### 3. Span Attributes

Each span includes Drover-specific attributes:

```go
// Task span attributes
drover.task.id         // Task identifier
drover.task.title      // Human-readable title
drover.task.state      // ready/in_progress/completed/failed
drover.task.priority   // 1-5
drover.task.attempt    // Retry attempt number

// Worker attributes
drover.worker.id       // Worker identifier (if applicable)

// Epic attributes
drover.epic.id         // Epic identifier (if in epic)

// Agent attributes
drover.agent.type      // "claude-code"
drover.agent.model     // Model used (if available)

// Project attributes (when added)
drover.project.id      // Project identifier
drover.project.path    // Project path
```

## Example: Running 3 Tasks

### Console Output

```
🐂 Starting DBOS workflow (queued) with 3 tasks
📋 Enqueuing 3 ready tasks (out of 3 total)
📤 Enqueued task task-1: Setup project structure
📤 Enqueued task task-2: Implement feature
📤 Enqueued task task-3: Write tests
⏳ Waiting for task result...
👷 Executing task task-1: Setup project structure
✅ Task task-1 completed in 2.3s
⏳ Waiting for task result...
👷 Executing task task-2: Implement feature
✅ Task task-2 completed in 15.7s
⏳ Waiting for task result...
👷 Executing task task-3: Write tests
✅ Task task-3 completed in 4.2s

📊 Queue execution complete in 22.1s
```

### Telemetry Generated (Behind the Scenes)

**Spans:** ~15-20 spans created
- 1 workflow span
- 3 task spans
- 12 step spans (4 per task)
- 3 agent spans

**Metrics:**
- `drover_tasks_claimed_total`: 3
- `drover_tasks_completed_total`: 3
- `drover_agent_prompts_total`: 3
- `drover_task_duration_seconds`: samples at 2.3, 15.7, 4.2 seconds
- `drover_agent_duration_seconds`: samples at ~2, 15, 4 seconds

### In Grafana Dashboard

**Task Status Panel:**
```
Completed: 3 ████
Failed:    0
```

**Task Duration Chart:**
```
Duration (s)
    20 ┤
    15 ┤     █
    10 ┤
     5 ┤  █     █
     0 └────────────
       t1  t2  t3
```

**Success Rate:**
```
100% ████████████████████
```

## Error Scenarios

### Task Failure

When a task fails:

```
❌ Task task-2 failed: agent execution error
```

**Telemetry:**
- Span status: `Error`
- Error recorded on span with error type
- `drover_tasks_failed_total` incremented
- `drover_task_duration_seconds` includes failed task
- `drover_agent_errors_total` incremented

### Agent Timeout

When Claude times out:

```
❌ Task task-1 failed: claude timed out after 60m
```

**Telemetry:**
- Span status: `Error`
- Error category: `timeout`
- `drover_agent_errors_total` with error_type `TimeoutError`

### Worktree Error

When worktree creation fails:

```
❌ Task task-1 failed: creating worktree: git error
```

**Telemetry:**
- Span status: `Error`
- Error category: `worktree`
- Error recorded before task execution

## Performance Impact

### With Telemetry Disabled
- Zero overhead
- No additional goroutines
- No memory allocation for telemetry

### With Telemetry Enabled (No Collector)
- Minimal overhead (~1-2% CPU)
- Spans created but dropped (no batch processing)
- ~1KB per span in memory until dropped

### With Telemetry Enabled + Collector
- Slightly more overhead (~2-5% CPU)
- Batch processing every 5 seconds
- Network I/O to OTLP endpoint
- Typically < 1ms added per operation

## Trace Sampling

### Development Environment
```go
SampleRate: 1.0  // 100% of traces sampled
```

### Production Environment
```go
SampleRate: 0.1  // 10% of traces sampled
```

Set via environment:
```bash
export DROVER_ENV=production
```

## Verifying Telemetry is Working

### Quick Health Check

```bash
# 1. Check ClickHouse is accessible
curl 'http://localhost:8123/?query=SELECT%201'

# 2. Check for Drover traces
curl 'http://localhost:8123/?query=SELECT%20count()%20FROM%20otel_traces%20WHERE%20ServiceName%3D%27drover%27'

# 3. Check recent activity
curl 'http://localhost:8123/?query=SELECT%20SpanName%2C%20count()%20FROM%20otel_traces%20WHERE%20ServiceName%3D%27drover%27%20GROUP%20BY%20SpanName'
```

### Expected Query Results

After running `drover run` with 3 tasks:

```sql
-- Trace count
SELECT count() FROM otel_traces WHERE ServiceName='drover';
-- Result: ~15-20

-- Span names
SELECT SpanName, count() FROM otel_traces WHERE ServiceName='drover' GROUP BY SpanName;
-- Result:
-- drover.workflow.run           | 1
-- drover.task.execute           | 3
-- drover.agent.execute          | 3
-- (plus DBOS step spans)

-- Task completion
SELECT countIf(StatusCode='Ok'), countIf(StatusCode='Error')
FROM otel_traces WHERE SpanName='drover.task.execute';
-- Result: 3, 0 (for successful run)
```

## Common Issues

### Issue: No data in ClickHouse

**Symptoms:**
- Grafana dashboard shows "No data"
- ClickHouse query returns 0 rows

**Checks:**
1. Telemetry enabled? `echo $DROVER_OTEL_ENABLED`
2. Collector running? `docker ps | grep otel`
3. Drover executed? Check console output
4. Endpoint correct? `echo $DROVER_OTEL_ENDPOINT`

### Issue: Dropped traces

**Symptoms:**
- Some traces missing
- Incomplete data

**Causes:**
- Collector not keeping up (check logs)
- Network issues
- Sampling too aggressive

### Issue: High memory usage

**Symptoms:**
- Drover process using more memory

**Solutions:**
- Reduce sample rate
- Check collector batch size
- Ensure collector is accepting data

## Troubleshooting Commands

```bash
# Check collector logs
docker compose -f docker-compose.telemetry.yaml logs otel-collector --tail 50

# Check ClickHouse is receiving data
docker compose -f docker-compose.telemetry.yaml exec clickhouse \
  clickhouse-client --query "SELECT count() FROM drover_telemetry.otel_traces"

# Restart the stack
docker compose -f docker-compose.telemetry.yaml restart

# View collector metrics
curl http://localhost:8888/metrics
```
