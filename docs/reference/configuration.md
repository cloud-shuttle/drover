---
title: Environment and configuration reference
description: Environment variables and defaults for Drover Orchestrator workers, DBOS, agents, and observability.
product: drover-orchestrator
audience: platform-operator
doc_type: reference
topics:
  - agent-jobs
surface: repo-docs
---

# Environment and configuration reference

Configuration loads from **environment variables** (`internal/config/config.go`) with defaults suitable for local development. Project-local state lives under `.drover/` after `drover init`.

For CLI flags on `drover run`, see [CLI reference](cli.md).

## Database and durability

| Variable | Required | Default | Purpose |
|----------|----------|---------|---------|
| `DROVER_DATABASE_URL` | no | `sqlite://<cwd>/.drover/drover.db` | Task/epic backlog database |
| `DBOS_SYSTEM_DATABASE_URL` | no | *(unset — SQLite DBOS)* | PostgreSQL for DBOS workflow checkpoints ([production how-to](../how-to/set-up-postgres-production.md)) |

## Workers and agents

| Variable | Required | Default | Purpose |
|----------|----------|---------|---------|
| `DROVER_WORKERS` | no | `3` | Default parallel workers (overridden by `drover run -w`) |
| `DROVER_AGENT_TYPE` | no | `claude` | Agent backend: `claude`, `codex`, `amp`, `opencode`, `worker` |
| `DROVER_AGENT_PATH` | no | per type | Path to agent CLI binary |
| `DROVER_CLAUDE_PATH` | no | — | Deprecated alias for `DROVER_AGENT_PATH` |
| `DROVER_TASK_TIMEOUT` | no | `60m` | Max time per task execution |
| `DROVER_USE_WORKER_SUBPROCESS` | no | `false` | Run agents inside `drover-worker` subprocess |
| `DROVER_WORKER_BINARY` | no | `drover-worker` | Subprocess worker binary path |
| `DROVER_WORKER_MEMORY_LIMIT` | no | — | Per-worker RSS cap (e.g. `2G`) |

See [Configure agents](../how-to/configure-agents.md).

## Planning / building modes

| Variable | Default | Purpose |
|----------|---------|---------|
| `DROVER_WORKER_MODE` | `combined` | `planning`, `building`, or `combined` |
| `DROVER_REQUIRE_APPROVAL` | `false` | Require plan approval globally |
| `DROVER_PLANNING_REQUIRE_APPROVAL` | `false` | Plans stay pending until approved |
| `DROVER_PLANNING_AUTO_APPROVE_LOW` | `false` | Auto-approve low-complexity plans |
| `DROVER_PLANNING_MAX_STEPS` | `20` | Max steps per generated plan |
| `DROVER_BUILDING_APPROVED_ONLY` | `false` | Building skips non-approved plans |
| `DROVER_BUILDING_VERIFY_STEPS` | `false` | Verify after each plan step |
| `DROVER_REFINEMENT_ENABLED` | `false` | Regenerate plans after rejection |
| `DROVER_REFINEMENT_MAX_REFINEMENTS` | `3` | Max refinement attempts |

See [Planning vs building mode](../how-to/planning-vs-building-mode.md).

## Worktree pool

| Variable | Default | Purpose |
|----------|---------|---------|
| `DROVER_POOL_ENABLED` | `false` | Enable warm worktree pool |
| `DROVER_POOL_MIN_SIZE` | `2` | Minimum pooled worktrees |
| `DROVER_POOL_MAX_SIZE` | `10` | Maximum pooled worktrees |
| `DROVER_POOL_WARMUP` | `5m` | Pool warmup interval |
| `DROVER_POOL_CLEANUP_ON_EXIT` | `true` | Remove pool on exit |

## Backpressure (adaptive concurrency)

| Variable | Default | Purpose |
|----------|---------|---------|
| `DROVER_BACKPRESSURE_ENABLED` | `true` | AIMD-style concurrency tuning |
| `DROVER_BACKPRESSURE_INITIAL_CONCURRENCY` | `2` | Starting concurrency |
| `DROVER_BACKPRESSURE_MIN_CONCURRENCY` | `1` | Floor |
| `DROVER_BACKPRESSURE_MAX_CONCURRENCY` | `4` | Ceiling |
| `DROVER_BACKPRESSURE_RATE_LIMIT_BACKOFF` | `30s` | Initial backoff on rate limit |
| `DROVER_BACKPRESSURE_MAX_BACKOFF` | `5m` | Max backoff |
| `DROVER_BACKPRESSURE_SLOW_THRESHOLD` | `10s` | Slow response threshold |
| `DROVER_BACKPRESSURE_MEMORY_AWARE_ENABLED` | `true` | Throttle when host memory low |
| `DROVER_BACKPRESSURE_MEMORY_THRESHOLD_MB` | `1024` | Throttle below this free MB |
| `DROVER_BACKPRESSURE_MEMORY_CRITICAL_MB` | `512` | Stop spawning below this free MB |
| `DROVER_BACKPRESSURE_WORKER_RSS_LIMIT_MB` | `2048` | Per-worker RSS limit |

## Webhooks and analytics

| Variable | Default | Purpose |
|----------|---------|---------|
| `DROVER_WEBHOOKS_ENABLED` | `false` | Enable outbound webhooks |
| `DROVER_WEBHOOK_URL` | — | Webhook endpoint |
| `DROVER_WEBHOOK_SECRET` | — | HMAC secret |
| `DROVER_WEBHOOK_WORKERS` | `3` | Delivery worker count |
| `DROVER_ANALYTICS_ENABLED` | `false` | Local analytics collection |
| `DROVER_ANALYTICS_CONFIG` | `.drover/analytics.json` | Analytics config path |
| `DROVER_ANALYTICS_MAX_METRICS` | `10000` | In-memory metric cap |

## OpenTelemetry

| Variable | Default | Purpose |
|----------|---------|---------|
| `DROVER_OTEL_ENABLED` | `false` | Export traces/metrics |
| `DROVER_OTEL_ENDPOINT` | `localhost:4317` | OTLP collector |

## File mailbox

| Variable | Default | Purpose |
|----------|---------|---------|
| `DROVER_MAILBOX_ENABLED` | `false` | File-based task queue |
| `DROVER_MAILBOX_DIR` | `/tmp/drover/mailbox` | Mailbox directory |
| `DROVER_MAILBOX_OUTBOX_RETENTION` | `7` | Days to keep completed |
| `DROVER_MAILBOX_FAILED_RETENTION` | `30` | Days to keep failed |
| `DROVER_MAILBOX_ORPHAN_SCAN_MINUTES` | `5` | Orphan scan interval |

## Operator identity

| Variable | Purpose |
|----------|---------|
| `DROVER_OPERATOR` | Operator name for multiplayer features (also `~/.drover/config.json`) |

## Related

- [CLI reference](cli.md)
- [Set up PostgreSQL for production](../how-to/set-up-postgres-production.md)
- [Route LLM through Gateway](../how-to/route-llm-through-gateway.md)
