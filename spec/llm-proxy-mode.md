# LLM Proxy Mode Specification

**Status:** ✅ Implemented | **Since:** v0.3.0 (Epic 6)

## Overview

Drover includes a unified LLM proxy server that routes requests to multiple providers (Anthropic, OpenAI, GLM, Groq, Grok) with rate limiting, cost tracking, and request logging.

## Architecture

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│   Worker/Agent  │────▶│  Proxy Server    │────▶│  Provider APIs  │
│                 │◀────│  (SSE Streaming) │◀────│  (Anthropic,    │
└─────────────────┘     │  - Rate Limit    │     │   OpenAI, etc.) │
                        │  - Cost Track    │     └─────────────────┘
                        │  - Logging       │
                        └──────────────────┘
```

## Supported Providers

| Provider | Models | API Endpoint |
|----------|--------|--------------|
| **Anthropic** | Claude 3.5 Sonnet, Haiku, Opus | `https://api.anthropic.com` |
| **OpenAI** | GPT-4o, GPT-4o-mini, o1 | `https://api.openai.com/v1` |
| **GLM** | GLM-4, GLM-4-Flash, GLM-4-Air | `https://open.bigmodel.cn/api/paas/v4` |
| **Groq** | LLaMA 3, Mixtral | `https://api.groq.com/openai/v1` |
| **Grok** | Grok-2, Grok-beta | `https://api.x.ai/v1` |

## Configuration

### Environment Variables

```bash
# Proxy server configuration
export DROVER_PROXY_ENABLED=true
export DROVER_PROXY_PORT=8080

# Provider API keys
export ANTHROPIC_API_KEY=sk-ant-...
export OPENAI_API_KEY=sk-...
export GLM_API_KEY=...
export GROQ_API_KEY=...
export GROK_API_KEY=...
```

### Configuration File

```yaml
# ~/.drover/proxy.yaml or .drover/proxy.yaml

listen_addr: ":8080"
log_level: "info"

providers:
  anthropic:
    api_key: "${ANTHROPIC_API_KEY}"
    enabled: true
    models:
      - name: "claude-sonnet-4-20250514"
        alias: "default"

  openai:
    api_key: "${OPENAI_API_KEY}"
    base_url: "https://api.openai.com/v1"
    enabled: true

# Rate limiting
rate_limit:
  requests_per_minute: 100
  tokens_per_minute: 100000
  enabled: true

# Cost tracking
cost_budget:
  hourly_limit: 10.0
  daily_limit: 100.0
  enabled: true
```

## API Endpoints

### POST /v1/chat/completions

Unified chat completion endpoint (OpenAI-compatible).

**Request:**
```json
{
  "model": "claude-sonnet-4",
  "messages": [
    {"role": "user", "content": "Hello!"}
  ],
  "stream": true,
  "temperature": 0.7
}
```

**Response (streaming):**
```
data: {"id":"chatcmpl-123","choices":[{"delta":{"content":"Hello"}}]}
data: {"id":"chatcmpl-123","choices":[{"delta":{"content":"!"}}]}
data: [DONE]
```

### GET /v1/models

List all available models from all providers.

**Response:**
```json
{
  "object": "list",
  "data": [
    {"id": "claude-sonnet-4", "provider": "anthropic"},
    {"id": "gpt-4o", "provider": "openai"}
  ]
}
```

### GET /health

Health check endpoint.

### GET /metrics

Cost and usage metrics.

## Rate Limiting

### Token Bucket Algorithm

```go
type tokenBucket struct {
    tokens     float64
    maxTokens  float64
    refillRate float64  // tokens per second
    lastRefill time.Time
}
```

### Limit Types

- **Requests per minute** - Number of API requests
- **Tokens per minute** - Total token count
- **IP-based limits** - Per-client IP
- **API key limits** - Per-API-key

### Configuration

```yaml
rate_limit:
  requests_per_minute: 100
  tokens_per_minute: 100000
  enabled: true
```

## Cost Tracking

### Cost Calculation

```go
type Cost struct {
    InputCost     float64  // Per 1M tokens
    OutputCost    float64  // Per 1M tokens
    TotalCost      float64
    InputTokens    int
    OutputTokens   int
    Currency       string
}
```

### Budget Enforcement

```yaml
cost_budget:
  hourly_limit: 10.0   # USD per hour
  daily_limit: 100.0   # USD per day
  enabled: true
```

When budget exceeded:
- Requests blocked with HTTP 429
- Error response includes budget info
- Resets at hour/day boundary

## Request Logging

### Log Entry

```go
type LogEntry struct {
    Timestamp  time.Time
    Provider   ProviderType
    Model      string
    RequestID  string
    Request    ChatRequest
    Response   ChatResponse
    Duration   time.Duration
    TokenUsage Usage
    Cost       *Cost
    IPAddress  string
    UserAgent  string
}
```

### Configuration

```yaml
logging:
  log_requests: true
  log_responses: false
  redact_pii: true
```

## Usage

### Start Proxy Server

```bash
# Using config file
drover proxy serve --config ~/.drover/proxy.yaml

# Using environment variables
export ANTHROPIC_API_KEY=sk-ant-...
drover proxy serve --port 8080
```

### Use Proxy from Worker

```bash
# Set proxy endpoint
export DROVER_LLM_PROXY_URL=http://localhost:8080

# Workers route requests through proxy
drover run --workers 4
```

### Direct API Usage

```bash
# Chat completion
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $API_KEY" \
  -d '{
    "model": "claude-sonnet-4",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'

# List models
curl http://localhost:8080/v1/models
```

## Model Aliases

Configure friendly aliases for models:

```yaml
providers:
  openai:
    models:
      - name: "gpt-4o"
        alias: "default"      # Use when no model specified
      - name: "gpt-4o-mini"
        alias: "fast"         # Use for speed
```

## Client Integration

### Drover Worker Integration

Workers automatically use proxy when configured:

```go
type WorkerConfig struct {
    LLMProxyURL string  // Set to proxy endpoint
    Provider     string  // Route to specific provider
}

// Worker routes all LLM requests through proxy
```

### Standalone Client

```go
import "github.com/cloud-shuttle/drover/internal/llmproxy/client"

client := client.New("http://localhost:8080", apiKey)
resp, err := client.Chat(ctx, req)
```

## See Also

- [Internal LLMProxy Documentation](../internal/llmproxy/README.md)
- [Provider Interface](../internal/llmproxy/provider/provider.go)
- [Cost Tracker](../internal/llmproxy/server/cost.go)
- [Rate Limiter](../internal/llmproxy/server/ratelimit.go)

---

*Last updated: 2026-01-16*
