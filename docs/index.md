# Drover Documentation Index

Welcome to the Drover documentation. This page serves as the central hub for all project documentation.

## Quick Links

- [README.md](../README.md) - Project overview, quick start, and usage
- [CONTRIBUTING.md](../CONTRIBUTING.md) - Contribution guidelines
- [Design](#design-documents) - Architecture and design decisions
- [Feature Specs](#feature-specifications) - Detailed feature documentation
- [Guides](#user-guides) - How-to guides and tutorials

---

## Design Documents

### Core Architecture

| Document | Description | Status |
|----------|-------------|--------|
| [Architecture](../design/architecture.md) | System architecture diagrams and component organization | Current |
| [State Machine](../design/state-machine.md) | Task state transitions and lifecycle | Current |
| [Sequence Diagrams](../design/sequence.md) | Runtime flow and interaction diagrams | Current |
| [Design Overview](../design/DESIGN.md) | Comprehensive design document with decisions and rationale | Current |

### Feature Proposals (Pending Implementation)

| Document | Description | Effort |
|----------|-------------|--------|
| [Enhancement Proposals](../design/proposals.md) | Consolidated enhancement proposals | ~6-8 weeks |

---

## Feature Specifications

### Implemented Features

| Feature | Spec | Status |
|---------|------|--------|
| **Durable Workflows** | [DBOS-based Orchestration](#durable-workflows) | ✅ Implemented |
| **Parallel Execution** | [Multi-Worker Execution](#parallel-execution) | ✅ Implemented |
| **Git Worktrees** | [Isolated Workspaces](#git-worktrees) | ✅ Implemented |
| **Sub-Tasks** | [Hierarchical Tasks](#sub-tasks) | ✅ Implemented |
| **Planning/Building Modes** | [Planning & Building Separation](#planning-building-modes) | ✅ Implemented |
| **LLM Proxy Mode** | [Multi-Provider LLM Proxy](#llm-proxy-mode) | ✅ Implemented |
| **OpenTelemetry** | [Observability & Metrics](#opentelemetry-observability) | ✅ Implemented |
| **Webhooks** | [Event Notifications](#webhooks) | ✅ Implemented |
| **Feature Flags** | [Dynamic Feature Toggles](#feature-flags) | ✅ Implemented |
| **Backpressure** | [Flow Control](#backpressure) | ✅ Implemented |
| **Code Search** | [Semantic Code Search](#code-search) | ✅ Implemented |
| **Plan Review TUI** | [Interactive Plan Review](#plan-review-tui) | ✅ Implemented |

### Proposed Features

| Feature | Description | Epic |
|---------|-------------|------|
| **Event Streaming** | JSONL event stream for integrations | E1 |
| **Project Configuration** | `.drover.toml` per-project settings | E2 |
| **Context Management** | Intelligent context window handling | E3 |
| **Structured Outcomes** | Pass/fail verdict extraction | E4 |
| **CLI Controls** | Cancel, retry, resolve commands | E5 |
| **Task Context** | Recent task memory | E6 |
| **Worktree Pooling** | Pre-warmed worktree cache | Future |
| **Human-in-the-Loop** | Pause, hint, intervention | Future |
| **Multiplayer** | Session handoff & collaboration | Future |

---

## User Guides

| Guide | Description |
|-------|-------------|
| [Quick Start](#quick-start) | Get started in 5 minutes |
| [Project Planning](./PROJECT_PLANNING.md) | Plan epics and tasks with Claude |
| [CLI Reference](#cli-reference) | Complete command reference |
| [Configuration](#configuration) | Environment variables and options |
| [Observability](../scripts/telemetry/README.md) | OpenTelemetry setup and dashboards |

---

## Developer Resources

| Resource | Description |
|----------|-------------|
| [Contributing](../CONTRIBUTING.md) | Development setup and guidelines |
| [Project Structure](#project-structure) | Codebase organization |
| [Testing](#testing) | Test coverage and running tests |
| [Design Principles](#design-principles) | Core design philosophy |

---

## Feature Specifications (Detailed)

### Durable Workflows

**Status:** ✅ Implemented | **Since:** v0.1.0

Drover uses [DBOS](https://dbos.dev/) for durable workflow execution. Every operation is checkpointed to the database, enabling automatic crash recovery.

**Key Features:**
- Automatic crash recovery
- Exactly-once execution semantics
- SQLite for development, PostgreSQL for production
- Queue-based parallel execution with backpressure

**See Also:** [Architecture](../design/architecture.md), [State Machine](../design/state-machine.md)

---

### Parallel Execution

**Status:** ✅ Implemented | **Since:** v0.1.0

Run 1-16 AI agents simultaneously to complete projects faster.

**Key Features:**
- Configurable worker count (`--workers N`)
- Dependency-aware scheduling
- Automatic task claiming and distribution
- Worker pool management

**See Also:** [Architecture](../design/architecture.md), [Sequence Diagrams](../design/sequence.md)

---

### Git Worktrees

**Status:** ✅ Implemented | **Since:** v0.1.0

Each worker operates in an isolated git worktree for parallel execution without conflicts.

**Key Features:**
- Isolated working directories per task
- Automatic worktree creation and cleanup
- Per-task commits merged to main branch
- Worktree pooling for performance (v0.3.0+)

**See Also:** [Architecture](../design/architecture.md), [CLI Reference](#cli-reference)

---

### Sub-Tasks

**Status:** ✅ Implemented | **Since:** v0.2.0

Hierarchical task decomposition using Beads-style IDs (e.g., `task-123.1`, `task-123.1.2`).

**Key Features:**
- Create sub-tasks via `--parent` flag
- Hierarchical ID syntax
- Sequential sub-task execution
- Parent waits for all children

**See Also:** [CLI Reference](#cli-reference), [State Machine](../design/state-machine.md)

---

### Planning/Building Modes

**Status:** ✅ Implemented | **Since:** v0.3.0 (Epic 5)

Separate planning from building with distinct worker modes.

**Key Features:**
- `combined` mode (default): Single worker for both
- `planning` mode: Dedicated planning agents
- `building` mode: Execute approved plans
- TUI for plan review and approval

**See Also:** [Internal Modes Documentation](../internal/modes/README.md)

---

### LLM Proxy Mode

**Status:** ✅ Implemented | **Since:** v0.3.0 (Epic 6)

Unified multi-provider LLM proxy with rate limiting, cost tracking, and logging.

**Supported Providers:**
- Anthropic (Claude)
- OpenAI (GPT)
- GLM (ZhipuAI)
- Groq
- Grok (xAI)

**Key Features:**
- Unified API endpoint
- Per-provider rate limiting
- Cost budget enforcement
- Request/response logging

**See Also:** [Internal LLMProxy Documentation](../internal/llmproxy/README.md)

---

### OpenTelemetry Observability

**Status:** ✅ Implemented | **Since:** v0.2.0

Built-in observability with OpenTelemetry, ClickHouse, and Grafana.

**Key Features:**
- Distributed tracing for workflows and tasks
- Metrics: completion rates, duration histograms
- Grafana dashboards included
- One-line telemetry enablement

**Quick Start:**
```bash
docker compose -f docker-compose.telemetry.yaml up -d
export DROVER_OTEL_ENABLED=true
drover run
```

**See Also:** [Observability Guide](../scripts/telemetry/README.md)

---

### Webhooks

**Status:** ✅ Implemented | **Since:** v0.2.0

HTTP webhooks for task lifecycle events.

**Key Features:**
- Event-driven notifications
- Multiple webhook endpoints
- Retry with exponential backoff
- Per-event filtering

**See Also:** [Internal Webhooks Documentation](../internal/webhooks/README.md)

---

### Feature Flags

**Status:** ✅ Implemented | **Since:** v0.2.0

Dynamic feature toggles via configuration file.

**Key Features:**
- JSON-based flag configuration
- Boolean and string values
- Runtime flag updates
- Per-environment overrides

**See Also:** [Internal Flags Documentation](../internal/flags/README.md)

---

### Backpressure

**Status:** ✅ Implemented | **Since:** v0.2.0

Flow control to prevent overwhelming systems.

**Key Features:**
- Token bucket rate limiting
- Configurable thresholds
- Automatic throttling
- Metrics integration

**See Also:** [Internal Backpressure Documentation](../internal/backpressure/README.md)

---

### Code Search

**Status:** ✅ Implemented | **Since:** v0.2.0

Semantic code search for intelligent code discovery.

**Key Features:**
- Natural language queries
- Code embeddings
- Ranked results
- Integration with worker context

**See Also:** [Internal Search Documentation](../internal/search/README.md)

---

### Plan Review TUI

**Status:** ✅ Implemented | **Since:** v0.3.0

Terminal UI for reviewing and approving implementation plans.

**Key Features:**
- Interactive plan browser
- Plan detail view
- Approve/reject workflow
- Feedback collection

**Usage:**
```bash
drover plan review
```

---

## CLI Reference

### Core Commands

| Command | Description |
|---------|-------------|
| `drover init` | Initialize Drover in current project |
| `drover run` | Execute all tasks to completion |
| `drover run --workers N` | Run with N parallel workers |
| `drover run --epic <id>` | Run only tasks in specific epic |
| `drover resume` | Resume interrupted workflows |

### Task Management

| Command | Description |
|---------|-------------|
| `drover add <title>` | Add a new task |
| `drover add <title> --parent <id>` | Add sub-task to parent |
| `drover add "task-123.N title"` | Add sub-task with hierarchical syntax |
| `drover add <title> --blocked-by <ids>` | Add task with dependencies |
| `drover status` | Show current project status |
| `drover status --tree` | Show hierarchical task tree |
| `drover status --watch` | Live progress updates |

### Epic Management

| Command | Description |
|---------|-------------|
| `drover epic add <title>` | Create a new epic |
| `drover epic list` | List all epics |
| `drover epic status <id>` | Show epic details |

### Plan Management

| Command | Description |
|---------|-------------|
| `drover plan review` | Review and approve plans (TUI) |
| `drover plan list` | List all plans |
| `drover plan show <id>` | Show plan details |

### Reset & Cleanup

| Command | Description |
|---------|-------------|
| `drover reset` | Reset all tasks back to ready |
| `drover reset task-abc` | Reset specific task |
| `drover reset --failed` | Reset all failed tasks |
| `drover worktree prune` | Clean up completed task worktrees |
| `drover worktree prune -a` | Clean up all worktrees |

---

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DROVER_DATABASE_URL` | `sqlite:///.drover.db` | Database connection string |
| `DROVER_AGENT_TYPE` | `claude` | Agent type (claude, codex, amp, opencode) |
| `DROVER_AGENT_PATH` | (auto) | Path to agent binary |
| `DROVER_OTEL_ENABLED` | `false` | Enable OpenTelemetry |
| `DROVER_OTEL_ENDPOINT` | `localhost:4317` | OTLP collector endpoint |
| `DROVER_ENV` | `development` | Deployment environment |

### Database Configuration

**Development (SQLite):**
```bash
# Default, no configuration needed
drover run
```

**Production (PostgreSQL):**
```bash
export DROVER_DATABASE_URL="postgresql://user:pass@localhost/drover"
drover run
```

---

## Project Structure

```
drover/
├── cmd/drover/              # CLI entry point
│   ├── main.go
│   └── commands.go
├── internal/
│   ├── analytics/           # Analytics and metrics
│   ├── backpressure/        # Flow control
│   ├── config/              # Configuration management
│   ├── db/                  # Database operations
│   ├── executor/            # Agent interface
│   ├── flags/               # Feature flags
│   ├── git/                 # Git worktree management
│   ├── llmproxy/            # LLM proxy server
│   │   ├── provider/        # Provider implementations
│   │   ├── server/          # HTTP server
│   │   └── client/          # Proxy client
│   ├── modes/               # Planning/building modes
│   ├── search/              # Code search
│   ├── template/            # Task templates
│   ├── webhooks/            # Webhook system
│   ├── workflow/            # DBOS workflows
│   └── pkg/
│       └── telemetry/       # OpenTelemetry
├── design/                  # Design documentation
├── docs/                    # Additional documentation
├── scripts/                 # Utility scripts
│   └── telemetry/           # Observability configs
└── pkg/
    └── types/               # Shared types
```

---

## Testing

### Run All Tests

```bash
go test ./...
```

### Run Specific Package

```bash
go test ./internal/workflow/...
```

### Run with Coverage

```bash
go test -cover ./...
```

### Run with Race Detector

```bash
go test -race ./...
```

### Test Observability

```bash
./scripts/telemetry/test-stack.sh
```

---

## Design Principles

1. **Durability First** - Never lose progress, survive any failure
2. **Parallelism** - Maximize throughput with concurrent agents
3. **Correctness** - Respect dependencies, avoid conflicts
4. **Simplicity** - Minimal configuration, sensible defaults
5. **Observability** - Clear visibility into progress and issues

---

## Additional Resources

### Inspirations & Acknowledgments

- **[DBOS](https://dbos.dev/)** - Durable workflow engine
- **[Beads](https://github.com/beads-dev/beads)** - Hierarchical task ID format
- **[Claude Code](https://claude.ai/code)** - AI execution layer
- **[Ramp's Inspect](https://builders.ramp.com/post/why-we-built-our-background-agent)** - Background agent patterns

### Related Projects

- **[roborev](https://github.com/wesm/roborev)** - Background AI coding agent
- **[aider](https://github.com/paul-gauthier/aider)** - AI pair programming
- **[cursor](https://cursor.sh)** - AI-powered IDE

---

## Need Help?

- **GitHub Issues**: Bug reports and feature requests
- **Discussions**: Design questions and RFCs
- **Contributing**: See [CONTRIBUTING.md](../CONTRIBUTING.md)

---

*Last updated: 2026-01-16* | *Version: 0.3.0*
