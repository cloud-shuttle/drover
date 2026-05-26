---
title: Route LLM traffic through Drover Gateway
description: Point self-hosted orchestrator agent CLIs at a local or shared Drover Gateway for unified provider routing.
product: drover-orchestrator
audience: platform-operator
doc_type: how-to
topics:
  - agent-jobs
  - llm-routing
surface: repo-docs
---

# Route LLM traffic through Drover Gateway

Self-hosted **`drover run`** agents (Claude Code, Codex, Amp, OpenCode) call provider APIs from the **agent CLI process** in each worktree. Drover does not proxy those calls itself — you configure the **agent's environment** so traffic goes through [**Drover Gateway**](../../../drover-gateway/CONTEXT.md) instead of hitting providers directly.

This differs from [**hosted agent jobs**](../../../drover-cloud/docs/how-to/configure-gateway-hosted-agents.md), where Cloud injects Gateway URLs and virtual keys into the worker environment.

## Why route through Gateway

- One place for provider keys (no keys on every developer laptop)
- Failover, caching, and governance plugins (virtual keys, rate limits)
- Consistent request logging for cost and debugging

## Prerequisites

- Running Gateway instance (local Compose from Milestone A stack or standalone deploy)
- At least one provider API key configured on Gateway (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, …)
- Agent CLI installed ([Configure agents](configure-agents.md))

## Step 1 — Start Gateway locally

From **drover-org** (with sibling checkouts):

```bash
docker compose -f docker-compose.milestone-a.yml up -d drover-gateway
curl -sf http://localhost:8080/health
```

Default listen port is **8080**. Adjust host/port if your deployment differs.

## Step 2 — Choose the Gateway base URL per agent

Gateway exposes provider-compatible paths. Map them to the env vars each CLI reads **before** `drover run`:

| Agent type | Typical env var | Example (local Gateway) |
|------------|-----------------|-------------------------|
| Claude Code | `ANTHROPIC_BASE_URL` | `http://localhost:8080/anthropic` |
| Codex / OpenAI-style | `OPENAI_BASE_URL` or vendor docs | `http://localhost:8080/v1` |
| Amp / OpenCode | See vendor docs | Gateway OpenAI-compatible `/v1` when supported |

Use the **same API key** Gateway expects (virtual key or provider key configured in Gateway config):

```bash
export ANTHROPIC_API_KEY="sk-bf-..."   # or your dev virtual key
export ANTHROPIC_BASE_URL="http://localhost:8080/anthropic"
```

Hosted workers use `AGENT_JOBS_LLM_BASE_URL` with the same path shape — see [Cloud Gateway how-to](../../../drover-cloud/docs/how-to/configure-gateway-hosted-agents.md).

## Step 3 — Run the orchestrator

Environment variables are inherited by child agent processes in each worktree:

```bash
export DROVER_AGENT_TYPE=claude
export ANTHROPIC_BASE_URL="http://localhost:8080/anthropic"
export ANTHROPIC_API_KEY="your-gateway-virtual-key"

cd my-repo
drover run --workers 2 --verbose
```

Verify in Gateway logs that requests arrive with expected model and dimension headers when you add them for tracing.

## Brain loop prevention (do not copy hosted headers)

If the same Gateway also serves **Drover Brain** indexing:

- Brain uses `X-Source: drover-brain` and an **internal** virtual key without MCP tools.
- Orchestrator agents must **not** send `X-Source: drover-brain`.
- Do not reuse Brain's internal key for agent work — use a separate **agent-facing** virtual key with tools enabled.

See [Brain Gateway integration](../../../drover-brain/docs/explanation/gateway-integration.md) and Gateway CONTEXT § loop prevention.

## Built-in `drover proxy` (not production-ready)

The repo includes an in-process LLM proxy (`internal/llmproxy/`, spec [`spec/llm-proxy-mode.md`](../../spec/llm-proxy-mode.md)). The CLI command `drover proxy` is currently a **stub** ("coming soon"). For production-style routing today, prefer **Drover Gateway** or configure agent CLIs directly.

## MCP tools through Gateway

Self-hosted orchestrator agents that invoke MCP via their CLI follow the agent vendor's MCP configuration. Register MCP servers on Gateway per [Gateway MCP docs](../../../drover-gateway/docs/mcp/overview.mdx) when your agent stack routes tool calls through the gateway LLM path.

Hosted third-party MCP binding (Muster + Guard) is **not** on the self-hosted `drover run` path — see [ADR 0005](../../../drover-brain/docs/adr/0005-drover-guard-hosted-integration.md).

## Related

- [Configure agents](configure-agents.md)
- [Cloud boundary](../explanation/cloud-boundary.md)
- [Gateway quickstart](../../../drover-gateway/docs/quickstart/gateway/setting-up.mdx) (upstream Mintlify)
