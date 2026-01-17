# OpenTelemetry Observability Specification

**Status:** ✅ Implemented | **Since:** v0.2.0

## Overview

Drover includes built-in observability via OpenTelemetry. Collect distributed traces, metrics, and logs with one-line configuration.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ Drover CLI                                                  │
│  • pkg/telemetry/ (OTel SDK)                                │
│  • Spans: workflow, task, agent, git operations             │
│  • Metrics: counters, gauges, histograms                    │
└─────────────────────┬───────────────────────────────────────┘
                      │ OTLP (gRPC/HTTP)
                      ▼
┌─────────────────────────────────────────────────────────────┐
│ OTel Collector                                              │
│  • Receives: OTLP traces/metrics/logs                       │
│  • Processes: batch, memory_limiter, spanmetrics            │
│  • Exports: ClickHouse                                      │
└─────────────────────┬───────────────────────────────────────┘
                      │ Native protocol
                      ▼
┌─────────────────────────────────────────────────────────────┐
│ ClickHouse                                                  │
│  • Tables: otel_traces, otel_metrics, otel_logs             │
│  • Materialized views: task summary, worker utilization     │
│  • TTL: 30 days                                             │
└─────────────────────────────────────────────────────────────┘
```

## Quick Start

### 1. Start the Observability Stack

```bash
# From drover repository root
docker compose -f docker-compose.telemetry.yaml up -d

# Verify services
docker compose -f docker-compose.telemetry.yaml ps
```

Services started:
- **ClickHouse** (ports 9000 native, 8123 HTTP)
- **OTel Collector** (ports 4317 gRPC, 4318 HTTP)
- **Grafana** (port 3000)

### 2. Enable Telemetry

```bash
export DROVER_OTEL_ENABLED=true
export DROVER_OTEL_ENDPOINT=localhost:4317

# Run Drover
drover run
```

### 3. View Dashboards

Open Grafana: http://localhost:3000
- Username: `admin`
- Password: `admin`

## Telemetry Types

### Traces

Distributed tracing for operation lifecycles.

#### Trace Hierarchy

```
drover.workflow.run (root)
├── drover.task.execute
│   ├── drover.agent.execute (claude-code)
│   ├── drover.git.worktree.create
│   ├── drover.git.commit
│   ├── drover.git.merge
│   └── drover.git.worktree.remove
└── drover.workflow.metrics
```

#### Spans

| Span Name | Description | Attributes |
|------------|-------------|------------|
| `drover.workflow.run` | Main workflow execution | `drover.project.id`, `drover.run.id` |
| `drover.task.execute` | Single task execution | `drover.task.id`, `drover.task.title`, `drover.worker.id` |
| `drover.agent.execute` | Claude Code execution | `drover.agent.type`, `drover.agent.model` |
| `drover.git.*` | Git operations | `drover.git.operation`, `drover.git.branch` |

### Metrics

Quantitative measurements over time.

#### Counters

| Metric | Description | Attributes |
|--------|-------------|------------|
| `drover.tasks.claimed` | Total tasks claimed | `drover.epic.id` |
| `drover.tasks.completed` | Total tasks completed | `drover.epic.id`, `drover.task.status` |
| `drover.tasks.failed` | Total tasks failed | `drover.epic.id`, `drover.error.type` |
| `drover.agent.prompts` | Agent prompts sent | `drover.agent.type` |

#### Histograms

| Metric | Description | Unit |
|--------|-------------|------|
| `drover.task.duration` | Task execution time | ms |
| `drover.agent.duration` | Agent execution time | ms |
| `drover.git.operation.duration` | Git operation time | ms |

#### Gauges

| Metric | Description |
|--------|-------------|
| `drover.workers.active` | Currently active workers |
| `drover.tasks.pending` | Tasks awaiting execution |
| `drover.tasks.in_progress` | Tasks currently executing |

### Logs

Structured logging with trace correlation.

```go
logger := log.With(
    "trace_id", traceID,
    "span_id", spanID,
    "task_id", taskID,
)
```

## Semantic Conventions

### Task Attributes

| Attribute | Type | Description |
|-----------|------|-------------|
| `drover.task.id` | string | Task identifier |
| `drover.task.title` | string | Human-readable title |
| `drover.task.state` | string | ready/in_progress/completed/failed |
| `drover.task.priority` | int | Task priority (0-100) |
| `drover.task.epic_id` | string | Associated epic |
| `drover.worker.id` | string | Worker identifier |

### Agent Attributes

| Attribute | Type | Description |
|-----------|------|-------------|
| `drover.agent.type` | string | Agent type (claude-code, codex, amp) |
| `drover.agent.model` | string | Model name |
| `drover.agent.provider` | string | Provider (anthropic, openai) |

### Error Attributes

| Attribute | Type | Description |
|-----------|------|-------------|
| `drover.error.type` | string | Error category |
| `drover.error.message` | string | Error message |
| `drover.error.retry_count` | int | Number of retries |

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DROVER_OTEL_ENABLED` | `false` | Enable OpenTelemetry |
| `DROVER_OTEL_ENDPOINT` | `localhost:4317` | OTLP collector endpoint |
| `DROVER_ENV` | `development` | Deployment environment |
| `DROVER_OTEL_SAMPLING` | `1.0` | Trace sampling rate (0.0-1.0) |

### Configuration File

```yaml
# ~/.drover/config.yaml

telemetry:
  enabled: true
  endpoint: "localhost:4317"
  environment: "production"
  sampling: 0.1  # 10% sampling in production

  service:
    name: "drover"
    version: "0.3.0"
```

## Instrumentation Guide

### Adding Spans

```go
import "github.com/cloud-shuttle/drover/pkg/telemetry"

func MyOperation(ctx context.Context) error {
    ctx, span := telemetry.StartTaskSpan(ctx, "drover.my.operation",
        telemetry.TaskAttrs(id, title, state, priority)...)
    defer span.End()

    // Your logic here
    err := doSomething(ctx)
    if err != nil {
        telemetry.RecordError(span, err, "OperationFailed", "category")
        return err
    }

    telemetry.RecordTaskCompleted(ctx, workerID, projectID, duration)
    return nil
}
```

### Adding Metrics

```go
// Record a metric
telemetry.RecordTaskClaimed(ctx, taskID, epicID)

// Record task completion with duration
telemetry.RecordTaskCompleted(ctx, workerID, projectID, duration)

// Record error
telemetry.RecordTaskFailed(ctx, taskID, epicID, err, "timeout")
```

### Custom Attributes

```go
attrs := []attribute.KeyValue{
    attribute.String("custom.field", "value"),
    attribute.Int("custom.count", 42),
}

ctx, span := telemetry.StartSpan(ctx, "operation.name",
    trace.WithAttributes(attrs...),
)
```

## Grafana Dashboards

### Built-in Dashboards

1. **Drover Overview**
   - Task completion rate
   - Worker utilization
   - Average task duration
   - Error rate

2. **Task Details**
   - Task execution timeline
   - Agent performance
   - Git operation metrics

3. **Worker Pool**
   - Active workers over time
   - Worker distribution
   - Idle time

### Import Dashboards

```bash
# Dashboards are provisioned automatically
# Located in scripts/telemetry/grafana/dashboards/
```

## Queries

### Task Success Rate (Last 24h)

```sql
SELECT
    countIf(StatusCode = 'Ok') AS completed,
    countIf(StatusCode = 'Error') AS failed,
    completed / (completed + failed) * 100 AS success_rate
FROM otel_traces
WHERE SpanName = 'drover.task.execute'
  AND Timestamp > now() - INTERVAL 24 HOUR;
```

### Average Task Duration by Epic

```sql
SELECT
    SpanAttributes['drover.epic.id'] AS epic,
    avg(Duration) / 1e9 AS avg_seconds,
    quantile(0.95)(Duration) / 1e9 AS p95_seconds
FROM otel_traces
WHERE SpanName = 'drover.task.execute'
  AND Timestamp > now() - INTERVAL 1 HOUR
GROUP BY epic;
```

### Trace for Specific Task

```sql
SELECT *
FROM otel_traces
WHERE TraceId = (
    SELECT TraceId
    FROM otel_traces
    WHERE SpanAttributes['drover.task.id'] = 'task-123'
    LIMIT 1
)
ORDER BY Timestamp;
```

## Stopping the Stack

```bash
docker compose -f docker-compose.telemetry.yaml down

# Remove volumes (delete all data)
docker compose -f docker-compose.telemetry.yaml down -v
```

## Troubleshooting

### Collector Not Receiving Data

```bash
# Check collector logs
docker compose -f docker-compose.telemetry.yaml logs otel-collector

# Verify endpoint
nc -zv localhost 4317
```

### No Data in Grafana

1. Verify Drover has telemetry enabled:
   ```bash
   env | grep DROVER_OTEL
   ```

2. Check ClickHouse has data:
   ```bash
   curl 'http://localhost:8123/?query=SELECT%20count()%20FROM%20otel_traces'
   ```

3. Verify Grafana datasource configuration

## Production Considerations

### Sampling

Reduce sampling in production to minimize overhead:

```bash
export DROVER_OTEL_SAMPLING=0.1  # 10% sampling
```

### Retention

Adjust data retention in ClickHouse:

```sql
ALTER TABLE otel_traces MODIFY TTL 7 DAY;
```

### Authentication

Enable TLS for production:

```yaml
telemetry:
  endpoint: "prod-collector:4317"
  tls:
    enabled: true
    insecure_skip_verify: false
```

## See Also

- [Observability Guide](../scripts/telemetry/README.md)
- [Telemetry Tests](../scripts/telemetry/TESTING.md)
- [Expected Behavior](../scripts/telemetry/EXPECTED_BEHAVIOR.md)
- [Telemetry Package](../pkg/telemetry/README.md)

---

*Last updated: 2026-01-16*
