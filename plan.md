# LLM Proxy Mode Implementation Plan

## Overview
Implement a flexible LLM proxy system that routes requests through a central proxy server, enabling unified API access, rate limiting, cost tracking, and request logging across multiple providers.

## Providers to Implement
1. **Anthropic** - Claude models
2. **OpenAI** - GPT models
3. **GLM (ZhipuAI)** - Chinese language models
4. **Groq** - Fast inference
5. **Grok** - xAI models

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

## Package Structure

```
internal/llmproxy/
├── provider/
│   ├── provider.go          # Provider interface
│   ├── anthropic.go         # Anthropic implementation
│   ├── openai.go            # OpenAI implementation
│   ├── glm.go               # GLM (ZhipuAI) implementation
│   ├── groq.go              # Groq implementation
│   └── grok.go              # Grok implementation
├── server/
│   ├── server.go            # HTTP server
│   ├── handlers.go          # Request handlers
│   ├── streaming.go         # SSE streaming
│   └── middleware.go        # Rate limiting, auth
├── client/
│   └── client.go            # Proxy client for workers
├── config/
│   └── config.go            # Proxy configuration
├── cost/
│   └── tracker.go           # Cost tracking
├── logging/
│   └── logger.go            # Request/response logging
└── types.go                 # Shared types
```

## Implementation Tasks

### Task LP-1: Define Provider Interface and Types
**File:** `internal/llmproxy/provider/provider.go`

```go
type Provider interface {
    Name() string
    StreamChat(ctx context.Context, req *ChatRequest) (<-chan ChatChunk, error)
    Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
    GetModels() []Model
    Validate() error
}

type ChatRequest struct {
    Model    string
    Messages []Message
    Tools    []Tool
    // ... common fields
}

type ChatChunk struct {
    Content  string
    Delta    *MessageDelta
    Metadata map[string]interface{}
}
```

### Task LP-2: Implement Anthropic Provider
**File:** `internal/llmproxy/provider/anthropic.go`
- Use Anthropic Go SDK or direct HTTP client
- Support Claude 3.5 Sonnet, Haiku, Opus
- Implement streaming via SSE
- Tool/function calling support

### Task LP-3: Implement OpenAI Provider
**File:** `internal/llmproxy/provider/openai.go`
- Use OpenAI Go SDK
- Support GPT-4o, GPT-4o-mini, o1
- Streaming support
- Tool calling support

### Task LP-4: Implement GLM (ZhipuAI) Provider
**File:** `internal/llmproxy/provider/glm.go`
- GLM-4, GLM-4-Flash, GLM-4-Air
- API endpoint: `https://open.bigmodel.cn/api/paas/v4/chat/completions`
- JWT token generation for auth

### Task LP-5: Implement Groq Provider
**File:** `internal/llmproxy/provider/groq.go`
- LLaMA 3, Mixtral models
- API endpoint: `https://api.groq.com/openai/v1/chat/completions`
- OpenAI-compatible API

### Task LP-6: Implement Grok Provider
**File:** `internal/llmproxy/provider/grok.go`
- Grok-2, Grok-beta
- API endpoint: `https://api.x.ai/v1/chat/completions`
- OpenAI-compatible API

### Task LP-7: Implement Proxy HTTP Server
**File:** `internal/llmproxy/server/server.go`

```go
type Server struct {
    config    *Config
    providers map[string]Provider
    costTracker *cost.Tracker
    logger    *logging.Logger
    rateLimit *rate.Limiter
}

// Endpoints:
// POST /v1/chat/completions - Unified chat endpoint
// GET  /v1/models - List available models
// GET  /health - Health check
// GET  /metrics - Cost and usage metrics
```

### Task LP-8: Add SSE Streaming Support
**File:** `internal/llmproxy/server/streaming.go`
- Server-Sent Events for streaming responses
- Chunk aggregation and forwarding
- Connection cleanup on disconnect

### Task LP-9: Implement Proxy Client
**File:** `internal/llmproxy/client/client.go`
- HTTP client for workers to use
- Automatic retries with exponential backoff
- Fallback between providers

### Task LP-10: Add Rate Limiting
**File:** `internal/llmproxy/server/middleware.go`
- Token bucket rate limiting
- Per-IP and per-API-key limits
- Configurable limits per provider

### Task LP-11: Add Cost Tracking
**File:** `internal/llmproxy/cost/tracker.go`
- Track token usage per provider/model
- Calculate costs based on pricing
- Budget enforcement with configurable limits

### Task LP-12: Add Request/Response Logging
**File:** `internal/llmproxy/logging/logger.go`
- Log all requests and responses
- Include timing, token usage, costs
- Support structured logging (JSON)
- Optional PII redaction

### Task LP-13: Configuration and CLI Integration
**File:** `cmd/drover/main.go`
- New `proxy serve` command
- Configuration file support (YAML)
- Provider API key management
- Model aliases and mappings

## Configuration Example

```yaml
llmproxy:
  listen_addr: ":8080"
  log_level: "info"
  providers:
    anthropic:
      api_key: "${ANTHROPIC_API_KEY}"
      models:
        - name: "claude-sonnet-4"
          alias: "default"
    openai:
      api_key: "${OPENAI_API_KEY}"
      base_url: "https://api.openai.com/v1"
    glm:
      api_key: "${GLM_API_KEY}"
    groq:
      api_key: "${GROQ_API_KEY}"
    grok:
      api_key: "${GROK_API_KEY}"
  rate_limits:
    requests_per_minute: 100
    tokens_per_minute: 100000
  cost_budget:
    hourly_limit: 10.0
    daily_limit: 100.0
```

## Dependencies

```go
// New imports in go.mod
require (
    github.com/anthropic-sdk/anthropic-go/v1 v1.0.0
    github.com/sashabaranov/go-openai v1.20.0
    github.com/redis/go-redis/v9 v9.0.0  // For rate limiting cache
)
```

## Testing Strategy

1. Unit tests for each provider
2. Integration tests with mock servers
3. Load testing for rate limiting
4. End-to-end tests with real API keys (optional)

## Migration Path

1. Phase 1: Core infrastructure (interface, server, client)
2. Phase 2: Anthropic + OpenAI providers
3. Phase 3: GLM, Groq, Grok providers
4. Phase 4: Cost tracking and advanced features
5. Phase 5: TUI and web dashboard integration (future)
