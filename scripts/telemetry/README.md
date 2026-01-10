# Drover Observability

This directory contains the OpenTelemetry observability stack for Drover, including ClickHouse storage, OTel Collector, and Grafana dashboards.

## Quick Start

### 1. Start the Telemetry Stack

```bash
# From the drover repository root
docker compose -f docker-compose.telemetry.yaml up -d

# Verify services are running
docker compose -f docker-compose.telemetry.yaml ps
```

Services started:
- **ClickHouse** (port 9000 native, 8123 HTTP) - Trace/metric storage
- **OTel Collector** (port 4317 gRPC, 4318 HTTP) - OTLP receiver
- **Grafana** (port 3000) - Visualization dashboards

### 2. Enable Telemetry in Drover

```bash
export DROVER_OTEL_ENABLED=true
export DROVER_OTEL_ENDPOINT=localhost:4317

# Run Drover commands
./drover run
```

### 3. View Dashboards

Open Grafana at http://localhost:3000
- Username: `admin`
- Password: `admin`

The Drover dashboard shows:
- Task execution metrics (completed, failed, duration)
- Worker utilization
- Agent performance
- Error rates

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

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DROVER_OTEL_ENABLED` | `false` | Enable OpenTelemetry |
| `DROVER_OTEL_ENDPOINT` | `localhost:4317` | OTLP collector endpoint |
| `DROVER_ENV` | `development` | Deployment environment |

## Key Queries

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

### Average Task Duration by Project
```sql
SELECT
    SpanAttributes['drover.project.id'] AS project,
    avg(Duration) / 1e9 AS avg_seconds,
    quantile(0.95)(Duration) / 1e9 AS p95_seconds
FROM otel_traces
WHERE SpanName = 'drover.task.execute'
  AND Timestamp > now() - INTERVAL 1 HOUR
GROUP BY project;
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

# To remove volumes (delete all telemetry data)
docker compose -f docker-compose.telemetry.yaml down -v
```

## Troubleshooting

### Collector not receiving data
```bash
# Check collector logs
docker compose -f docker-compose.telemetry.yaml logs otel-collector

# Verify OTLP endpoint is accessible
nc -zv localhost 4317
```

### ClickHouse connection issues
```bash
# Check ClickHouse logs
docker compose -f docker-compose.telemetry.yaml logs clickhouse

# Test ClickHouse connection
curl 'http://localhost:8123/' --data 'SELECT 1'
```

### No data in Grafana
1. Verify Drover has `DROVER_OTEL_ENABLED=true`
2. Check ClickHouse has data:
   ```bash
   curl 'http://localhost:8123/?query=SELECT%20count()%20FROM%20otel_traces'
   ```
3. Verify Grafana datasource configuration

## Production Considerations

1. **Sampling**: Reduce sampling rate in production (default 10%)
2. **Retention**: Adjust TTL based on storage capacity
3. **Authentication**: Enable TLS for OTLP in production
4. **Scaling**: Run collector as a separate service

## Development

### Adding New Spans

```go
import "github.com/cloud-shuttle/drover/pkg/telemetry"

func DoWork(ctx context.Context) {
    ctx, span := telemetry.StartTaskSpan(ctx, "drover.work.do",
        telemetry.TaskAttrs(id, title, state, priority, 1)...)
    defer span.End()

    // Your work here

    telemetry.RecordTaskCompleted(ctx, workerID, projectID, duration)
}
```

### Adding New Metrics

```go
// In pkg/telemetry/metrics.go
var myCounter metric.Int64Counter

func initMetrics() error {
    // ...
    myCounter, err = meter.Int64Counter("drover_my_counter")
    // ...
}

// Record metric
telemetry.RecordMyCustomMetric(ctx, value)
```
