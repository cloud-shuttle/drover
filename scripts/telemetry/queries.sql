-- ClickHouse Queries for Drover Telemetry Verification
--
-- Usage: curl 'http://localhost:8123/?query=SELECT%20...' < queries.sql
-- Or: clickhouse-client --queries-file queries.sql

-- ============================================
-- BASIC VERIFICATION QUERIES
-- ============================================

-- 1. Check if telemetry is receiving data
SELECT count() as total_traces FROM otel_traces;
-- Expected: > 0 after running drover with telemetry enabled

-- 2. List all trace names (span types)
SELECT DISTINCT SpanName
FROM otel_traces
ORDER BY SpanName;

-- Expected to see:
-- - drover.workflow.run
-- - drover.task.execute
-- - drover.agent.execute
-- (plus DBOS workflow/step spans)

-- 3. Check recent activity
SELECT
    toDateTime(Timestamp) as time,
    SpanName,
    StatusCode,
    count() as count
FROM otel_traces
WHERE Timestamp > now() - INTERVAL 1 HOUR
GROUP BY time, SpanName, StatusCode
ORDER BY time DESC
LIMIT 20;

-- ============================================
-- TASK EXECUTION QUERIES
-- ============================================

-- 4. Recent task executions with details
SELECT
    toDateTime(Timestamp) as time,
    SpanAttributes['drover.task.id'] as task_id,
    SpanAttributes['drover.task.title'] as task_title,
    SpanAttributes['drover.task.state'] as state,
    StatusCode,
    Duration / 1e9 as duration_seconds
FROM otel_traces
WHERE SpanName = 'drover.task.execute'
ORDER BY Timestamp DESC
LIMIT 20;

-- 5. Task success rate (all time)
SELECT
    countIf(StatusCode = 'Ok') as completed,
    countIf(StatusCode = 'Error') as failed,
    round(completed / (completed + failed) * 100, 2) as success_rate_percent
FROM otel_traces
WHERE SpanName = 'drover.task.execute';

-- 6. Task success rate (last 24h)
SELECT
    toStartOfHour(Timestamp) as hour,
    countIf(StatusCode = 'Ok') as completed,
    countIf(StatusCode = 'Error') as failed,
    round(completed / (completed + failed) * 100, 2) as success_rate_percent
FROM otel_traces
WHERE SpanName = 'drover.task.execute'
  AND Timestamp > now() - INTERVAL 24 HOUR
GROUP BY hour
ORDER BY hour DESC;

-- 7. Average task duration by status
SELECT
    StatusCode,
    count() as count,
    round(avg(Duration) / 1e9, 3) as avg_seconds,
    round(quantile(0.50)(Duration) / 1e9, 3) as p50_seconds,
    round(quantile(0.95)(Duration) / 1e9, 3) as p95_seconds,
    round(quantile(0.99)(Duration) / 1e9, 3) as p99_seconds
FROM otel_traces
WHERE SpanName = 'drover.task.execute'
GROUP BY StatusCode;

-- ============================================
-- AGENT PERFORMANCE QUERIES
-- ============================================

-- 8. Agent execution statistics
SELECT
    SpanAttributes['drover.agent.type'] as agent_type,
    count() as executions,
    countIf(StatusCode = 'Ok') as successful,
    countIf(StatusCode = 'Error') as failed,
    round(avg(Duration) / 1e9, 3) as avg_seconds,
    round(quantile(0.95)(Duration) / 1e9, 3) as p95_seconds
FROM otel_traces
WHERE SpanName = 'drover.agent.execute'
GROUP BY agent_type;

-- 9. Agent duration histogram
SELECT
    SpanAttributes['drover.agent.type'] as agent_type,
    round(Duration / 1e9, 2) as duration_seconds,
    count() as count
FROM otel_traces
WHERE SpanName = 'drover.agent.execute'
GROUP BY agent_type, duration_seconds
ORDER BY duration_seconds
LIMIT 50;

-- ============================================
-- WORKER QUERIES
-- ============================================

-- 10. Tasks by worker
SELECT
    SpanAttributes['drover.worker.id'] as worker_id,
    count() as task_count,
    countIf(StatusCode = 'Ok') as completed,
    countIf(StatusCode = 'Error') as failed,
    round(avg(Duration) / 1e9, 3) as avg_duration_seconds
FROM otel_traces
WHERE SpanName = 'drover.task.execute'
  AND SpanAttributes['drover.worker.id'] != ''
GROUP BY worker_id
ORDER BY task_count DESC;

-- ============================================
-- WORKFLOW QUERIES
-- ============================================

-- 11. Workflow execution summary
SELECT
    SpanAttributes['drover.workflow.type'] as workflow_type,
    count() as workflows,
    round(sum(Duration) / 1e9 / 60, 2) as total_minutes,
    round(avg(Duration) / 1e9, 2) as avg_seconds
FROM otel_traces
WHERE SpanName = 'drover.workflow.run'
GROUP BY workflow_type;

-- 12. Workflow metrics from attributes
SELECT
    SpanAttributes['drover.workflow.completed'] as completed,
    SpanAttributes['drover.workflow.failed'] as failed,
    SpanAttributes['drover.workflow.duration_seconds'] as duration_seconds
FROM otel_traces
WHERE SpanName = 'drover.workflow.run'
ORDER BY Timestamp DESC
LIMIT 10;

-- ============================================
-- TRACE EXPLORATION QUERIES
-- ============================================

-- 13. Find trace ID for a specific task
SELECT TraceId
FROM otel_traces
WHERE SpanAttributes['drover.task.id'] = 'YOUR_TASK_ID_HERE'
LIMIT 1;

-- 14. Full trace hierarchy for a task (replace TRACE_ID)
SELECT
    Timestamp,
    SpanName,
    ParentSpanId,
    StatusCode,
    Duration / 1e9 as duration_sec,
    SpanAttributes['drover.task.id'] as task_id
FROM otel_traces
WHERE TraceId = 'YOUR_TRACE_ID_HERE'
ORDER BY Timestamp;

-- 15. Trace tree with indentation
SELECT
    Timestamp,
    repeat('  ', length(ParentSpanId) / 16) || SpanName as tree,
    StatusCode,
    Duration / 1e9 as duration_sec
FROM otel_traces
WHERE TraceId = 'YOUR_TRACE_ID_HERE'
ORDER BY Timestamp
FORMAT Vertical;

-- ============================================
-- ERROR ANALYSIS QUERIES
-- ============================================

-- 16. Recent errors
SELECT
    toDateTime(Timestamp) as time,
    SpanName,
    SpanAttributes['drover.task.id'] as task_id,
    StatusMessage as error_message
FROM otel_traces
WHERE StatusCode = 'Error'
ORDER BY Timestamp DESC
LIMIT 20;

-- 17. Errors by span type
SELECT
    SpanName,
    count() as error_count,
    groupDistinct(StatusMessage) as error_messages
FROM otel_traces
WHERE StatusCode = 'Error'
GROUP BY SpanName
ORDER BY error_count DESC;

-- 18. Error rate over time
SELECT
    toStartOfHour(Timestamp) as hour,
    count() as total_spans,
    countIf(StatusCode = 'Error') as errors,
    round(errors / total_spans * 100, 2) as error_rate_percent
FROM otel_traces
WHERE Timestamp > now() - INTERVAL 24 HOUR
GROUP BY hour
ORDER BY hour DESC;

-- ============================================
-- METRIC QUERIES (if spanmetrics enabled)
-- ============================================

-- 19. View available metrics
SELECT DISTINCT Name
FROM otel_metrics
ORDER BY Name;

-- 20. Recent metric values
SELECT
    toDateTime(Timestamp) as time,
    Name,
    Value,
    Attributes
FROM otel_metrics
ORDER BY Timestamp DESC
LIMIT 20;

-- ============================================
-- STORAGE MANAGEMENT
-- ============================================

-- 21. Check table sizes
SELECT
    table,
    formatReadableSize(sum(bytes)) as size,
    sum(rows) as rows
FROM system.parts
WHERE database = 'drover_telemetry'
  AND active
GROUP BY table
ORDER BY sum(bytes) DESC;

-- 22. Oldest traces
SELECT min(Timestamp) as oldest_trace, max(Timestamp) as newest_trace
FROM otel_traces;

-- 23. Traces by day
SELECT
    toDate(Timestamp) as date,
    count() as trace_count
FROM otel_traces
GROUP BY date
ORDER BY date DESC;

-- ============================================
-- DIAGNOSTIC QUERIES
-- ============================================

-- 24. Check for missing attributes
SELECT
    count() as total_spans,
    countIf(SpanAttributes['drover.task.id'] = '') as missing_task_id,
    countIf(SpanAttributes['drover.worker.id'] = '') as missing_worker_id,
    countIf(ServiceName = '') as missing_service_name
FROM otel_traces;

-- 25. Service name distribution
SELECT
    ServiceName,
    count() as span_count
FROM otel_traces
GROUP BY ServiceName
ORDER BY span_count DESC;

-- 26. Span kind distribution
SELECT
    SpanKind,
    count() as span_count
FROM otel_traces
GROUP BY SpanKind
ORDER BY span_count DESC;
