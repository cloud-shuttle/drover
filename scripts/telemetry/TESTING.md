# Testing Drover Telemetry Locally

This guide walks through testing the OpenTelemetry observability stack for Drover using Docker Compose.

## Prerequisites

- Docker installed locally
- Docker Compose installed (or `docker compose` plugin)
- Drover built from source

## Quick Test

### Step 1: Start the Telemetry Stack

```bash
# From the drover repository root
cd /path/to/drover
docker compose -f docker-compose.telemetry.yaml up -d
```

Expected output:
```
✓ ClickHouse container started
✓ OTel Collector container started
✓ Grafana container started
```

Verify services are running:
```bash
docker compose -f docker-compose.telemetry.yaml ps
```

### Step 2: Initialize a Test Drover Project

```bash
# Create a test project
mkdir /tmp/drover-test
cd /tmp/drover-test
git init
git checkout -b main

# Initialize Drover
/path/to/drover init

# Add some test tasks
/path/to/drover add "Add telemetry to Drover" "Implement OpenTelemetry traces and metrics"
/path/to/drover add "Create Grafana dashboards" "Build dashboards for observability"
/path/to/drover add "Write documentation" "Document the telemetry setup"
```

### Step 3: Run Drover with Telemetry Enabled

```bash
# Enable telemetry
export DROVER_OTEL_ENABLED=true
export DROVER_OTEL_ENDPOINT=localhost:4317
export DROVER_ENV=development

# Run Drover (this will generate traces/metrics)
/path/to/drover run
```

Expected telemetry behavior:
- Spans are created for: workflow run, task execution, agent calls
- Metrics are recorded: tasks claimed, completed, agent duration
- Data is sent to OTel collector via OTLP (localhost:4317)

### Step 4: Verify Data in ClickHouse

```bash
# Check if traces are being received
curl 'http://localhost:8123/?query=SELECT%20count()%20FROM%20otel_traces'

# Should return a number > 0

# View recent traces
curl 'http://localhost:8123/?query=SELECT%20SpanName%2C%20StatusCode%2C%20Duration%20FROM%20otel_traces%20ORDER%20BY%20Timestamp%20DESC%20LIMIT%2010'
```

### Step 5: View Grafana Dashboard

1. Open Grafana: http://localhost:3000
2. Login: `admin` / `admin`
3. Navigate to "Drover" → "Drover Telemetry" dashboard

You should see:
- Task status breakdown (completed vs failed)
- Average task duration over time
- Tasks by worker
- Agent execution metrics

## Manual Verification Queries

### 1. Check Traces are Being Received

```sql
-- ClickHouse HTTP interface
http://localhost:8123/

-- Query: Total trace count
SELECT count() as total_traces FROM otel_traces;

-- Expected: number > 0 after running drover
```

### 2. View Recent Task Executions

```sql
SELECT
    Timestamp,
    SpanAttributes['drover.task.id'] as task_id,
    SpanAttributes['drover.task.title'] as task_title,
    StatusCode,
    Duration / 1e9 as duration_seconds
FROM otel_traces
WHERE SpanName = 'drover.task.execute'
ORDER BY Timestamp DESC
LIMIT 10;
```

### 3. Task Success Rate

```sql
SELECT
    countIf(StatusCode = 'Ok') as completed,
    countIf(StatusCode = 'Error') as failed,
    round(completed / (completed + failed) * 100, 2) as success_rate
FROM otel_traces
WHERE SpanName = 'drover.task.execute';
```

### 4. Agent Performance

```sql
SELECT
    SpanAttributes['drover.agent.type'] as agent_type,
    count() as executions,
    avg(Duration) / 1e9 as avg_duration_sec,
    quantile(0.95)(Duration) / 1e9 as p95_duration_sec
FROM otel_traces
WHERE SpanName = 'drover.agent.execute'
GROUP BY agent_type;
```

### 5. Full Trace for a Task

```sql
SELECT
    Timestamp,
    SpanName,
    ParentSpanId,
    StatusCode,
    Duration / 1e9 as duration_sec,
    SpanAttributes
FROM otel_traces
WHERE TraceId = (
    SELECT TraceId
    FROM otel_traces
    WHERE SpanAttributes['drover.task.id'] = 'YOUR_TASK_ID'
    LIMIT 1
)
ORDER BY Timestamp
FORMAT Vertical;
```

## Expected Span Hierarchy

When you run `drover run`, you should see traces like this:

```
drover.workflow.run (root)
├── drover.task.execute (for each task)
│   ├── drover.agent.execute (claude-code)
│   │   └── (agent operations)
│   └── (DBOS steps for worktree, commit, merge)
└── workflow completion metrics
```

## Troubleshooting

### No data in ClickHouse

1. **Check Collector Logs**
```bash
docker compose -f docker-compose.telemetry.yaml logs otel-collector
```

2. **Verify Drover is Sending**
```bash
# Check environment variables
echo $DROVER_OTEL_ENABLED
echo $DROVER_OTEL_ENDPOINT

# Verify collector is reachable
nc -zv localhost 4317
```

3. **Check ClickHouse Connection**
```bash
# Test ClickHouse
curl 'http://localhost:8123/?query=SELECT%201'

# Check tables exist
curl 'http://localhost:8123/?query=SHOW%20TABLES'
```

### Grafana Dashboard Issues

1. **Verify Datasource**
   - Go to Configuration → Data Sources
   - Click "ClickHouse" datasource
   - Test connection
   - Should show green "Connection OK"

2. **Dashboard Not Showing Data**
   - Check time range (top right corner)
   - Set to "Last 5 minutes" or "Last 1 hour"
   - Refresh dashboard

### Collector Not Starting

1. **Check ClickHouse is Ready**
```bash
docker compose -f docker-compose.telemetry.yaml logs clickhouse
```

2. **Verify Configuration**
```bash
docker compose -f docker-compose.telemetry.yaml config
```

3. **Restart Stack**
```bash
docker compose -f docker-compose.telemetry.yaml down
docker compose -f docker-compose.telemetry.yaml up -d
```

## Cleanup

```bash
# Stop and remove containers
docker compose -f docker-compose.telemetry.yaml down

# Remove volumes (deletes all telemetry data)
docker compose -f docker-compose.telemetry.yaml down -v

# Clean up test project
rm -rf /tmp/drover-test
```

## Performance Testing

To generate more telemetry data for testing:

```bash
# Add many tasks
for i in {1..20}; do
  /path/to/drover add "Test task $i" "Description for task $i"
done

# Run with multiple workers
export DROVER_WORKERS=3
/path/to/drover run
```

This will create:
- 20 task execution spans
- 20 agent execution spans
- Multiple worker activity spans
- Metrics for parallel execution

## Expected Metrics

After running a test, you should see these metrics in Grafana:

| Metric | Type | Expected Values |
|--------|------|-----------------|
| `drover_tasks_claimed_total` | Counter | Number of tasks |
| `drover_tasks_completed_total` | Counter | Completed tasks |
| `drover_agent_prompts_total` | Counter | Number of agent calls |
| `drover_agent_duration_seconds` | Histogram | Duration distribution |
| `drover_task_duration_seconds` | Histogram | Task duration distribution |
