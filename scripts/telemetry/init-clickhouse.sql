-- ClickHouse initialization for Drover telemetry
-- This script is run automatically when the ClickHouse container starts

-- Create database
CREATE DATABASE IF NOT EXISTS drover_telemetry;

-- Use the database
USE drover_telemetry;

-- Create OTel traces table (if not created by exporter)
-- The ClickHouse exporter auto-creates tables, but we define them here for reference

-- Traces table (main span data)
CREATE TABLE IF NOT EXISTS otel_traces
(
    Timestamp DateTime64(9),
    TraceId String,
    SpanId String,
    ParentSpanId String,
    TraceState String,
    SpanName String,
    SpanKind LowCardinality(String),
    ServiceName LowCardinality(String),
    ResourceAttributes Map(String, String),
    SpanAttributes Map(String, String),
    Duration Int64,
    StatusCode LowCardinality(String),
    StatusMessage String,
    Events Nested
    (
        Timestamp DateTime64(9),
        Name String,
        Attributes Map(String, String)
    ),
    Links Nested
    (
        TraceId String,
        SpanId String,
        TraceState String,
        Attributes Map(String, String)
    )
)
ENGINE = MergeTree()
ORDER BY (ServiceName, SpanName, toDate(Timestamp), TraceId)
TTL toDate(Timestamp) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

-- Materialized view: Task execution summary by project
CREATE MATERIALIZED VIEW IF NOT EXISTS drover_task_summary_mv
ENGINE = SummingMergeTree()
ORDER BY (project_id, date, status)
AS SELECT
    SpanAttributes['drover.project.id'] AS project_id,
    toDate(Timestamp) AS date,
    StatusCode AS status,
    count() AS task_count,
    avg(Duration) / 1e9 AS avg_duration_seconds,
    quantile(0.95)(Duration) / 1e9 AS p95_duration_seconds,
    quantile(0.99)(Duration) / 1e9 AS p99_duration_seconds
FROM otel_traces
WHERE SpanName = 'drover.task.execute'
GROUP BY project_id, date, status;

-- Summary table for task metrics
CREATE TABLE IF NOT EXISTS drover_task_summary
(
    project_id String,
    date Date,
    status LowCardinality(String),
    task_count UInt64,
    avg_duration_seconds Float64,
    p95_duration_seconds Float64,
    p99_duration_seconds Float64
)
ENGINE = SummingMergeTree()
ORDER BY (project_id, date, status);

-- Materialized view: Worker utilization
CREATE MATERIALIZED VIEW IF NOT EXISTS drover_worker_utilization_mv
ENGINE = SummingMergeTree()
ORDER BY (worker_id, hour)
AS SELECT
    SpanAttributes['drover.worker.id'] AS worker_id,
    toStartOfHour(Timestamp) AS hour,
    count() AS tasks_completed,
    sum(Duration) / 1e9 AS total_work_seconds,
    avg(Duration) / 1e9 AS avg_task_duration
FROM otel_traces
WHERE SpanName = 'drover.task.execute' AND StatusCode = 'Ok'
GROUP BY worker_id, hour;

-- Worker utilization table
CREATE TABLE IF NOT EXISTS drover_worker_utilization
(
    worker_id String,
    hour DateTime,
    tasks_completed UInt64,
    total_work_seconds Float64,
    avg_task_duration Float64
)
ENGINE = SummingMergeTree()
ORDER BY (worker_id, hour);

-- Materialized view: Blocker analysis
CREATE MATERIALIZED VIEW IF NOT EXISTS drover_blocker_analysis_mv
ENGINE = SummingMergeTree()
ORDER BY (blocker_type, project_id, date)
AS SELECT
    SpanAttributes['drover.blocker.type'] AS blocker_type,
    SpanAttributes['drover.project.id'] AS project_id,
    toDate(Timestamp) AS date,
    count() AS occurrences,
    avg(Duration) / 1e9 AS avg_resolution_time
FROM otel_traces
WHERE SpanName = 'drover.blocker.detect'
GROUP BY blocker_type, project_id, date;

-- Blocker analysis table
CREATE TABLE IF NOT EXISTS drover_blocker_analysis
(
    blocker_type LowCardinality(String),
    project_id String,
    date Date,
    occurrences UInt64,
    avg_resolution_time Float64
)
ENGINE = SummingMergeTree()
ORDER BY (blocker_type, project_id, date);

-- Materialized view: Agent performance
CREATE MATERIALIZED VIEW IF NOT EXISTS drover_agent_performance_mv
ENGINE = SummingMergeTree()
ORDER BY (agent_type, date)
AS SELECT
    SpanAttributes['drover.agent.type'] AS agent_type,
    toDate(Timestamp) AS date,
    count() AS executions,
    avg(Duration) / 1e9 AS avg_duration_seconds,
    quantile(0.95)(Duration) / 1e9 AS p95_duration_seconds
FROM otel_traces
WHERE SpanName = 'drover.agent.execute'
GROUP BY agent_type, date;

-- Agent performance table
CREATE TABLE IF NOT EXISTS drover_agent_performance
(
    agent_type LowCardinality(String),
    date Date,
    executions UInt64,
    avg_duration_seconds Float64,
    p95_duration_seconds Float64
)
ENGINE = SummingMergeTree()
ORDER BY (agent_type, date);

-- Metrics table (will be created by exporter)
CREATE TABLE IF NOT EXISTS otel_metrics
(
    Timestamp DateTime64(9),
    Name String,
    Value Float64,
    Attributes Map(String, String),
    Resource Map(String, String)
)
ENGINE = MergeTree()
ORDER BY (Name, toDate(Timestamp), Timestamp)
TTL toDate(Timestamp) + INTERVAL 30 DAY;

-- Logs table (will be created by exporter)
CREATE TABLE IF NOT EXISTS otel_logs
(
    Timestamp DateTime64(9),
    TraceId String,
    SpanId String,
    SeverityNumber Int32,
    SeverityText String,
    Body String,
    ResourceAttributes Map(String, String),
    Attributes Map(String, String)
)
ENGINE = MergeTree()
ORDER BY (toDate(Timestamp), Timestamp, TraceId)
TTL toDate(Timestamp) + INTERVAL 30 DAY;

-- Create indexes for common queries
-- Note: ClickHouse uses skip indexes rather than traditional B-tree indexes

-- Allow mutations on tables for TTL processing
SYSTEM STOP MERGES ON otel_traces;
SYSTEM START MERGES ON otel_traces;

-- Grant permissions
GRANT ALL ON drover_telemetry.* TO default;
