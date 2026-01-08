# 🐂 Drover

**Drive your project to completion with parallel AI agents.**

Drover is a durable workflow orchestrator that runs multiple Claude Code agents in parallel to complete your entire project. It manages task dependencies, handles failures gracefully, and guarantees progress through crashes and restarts.

> *"No task left behind."*

## Workflow Engine

Drover uses **DBOS (Durable Operating System for Workflows)** as its primary workflow engine:

- **Development**: SQLite-based orchestration (zero setup, works out of the box)
- **Production**: DBOS with PostgreSQL (set `DBOS_SYSTEM_DATABASE_URL`)

Both modes provide durable execution, automatic retries, and crash recovery.

## Why Drover?

You have a project with dozens of tasks. Running them one at a time is slow. Running them manually in parallel is chaotic. Drover solves this by:

- **Parallel execution** — Run 4, 8, or 16 Claude Code agents simultaneously
- **Durable workflows** — Survive crashes, restarts, and network failures
- **Smart scheduling** — Respects task dependencies, priorities, and blockers
- **Isolated workspaces** — Each agent works in its own git worktree
- **Automatic retries** — Failed tasks retry with exponential backoff
- **Progress tracking** — Real-time status and completion estimates

## Quick Start

```bash
# Install
go install github.com/cloud-shuttle/drover@latest

# Initialize in your project
cd my-project
drover init

# Create an epic and tasks
drover epic add "MVP Features"
drover add "Set up database schema" --epic epic-a1b2
drover add "Implement user authentication" --epic epic-a1b2
drover add "Build REST API" --epic epic-a1b2 --blocked-by task-x1y2
drover add "Add unit tests" --epic epic-a1b2 --blocked-by task-x1y2,task-z3w4

# Run everything to completion
drover run --workers 4

# Or run a specific epic
drover run --epic epic-a1b2
```

## Installation

### Prerequisites

- Go 1.22+
- Git
- [Claude Code CLI](https://claude.ai/code) installed and authenticated
- PostgreSQL (production) or SQLite (local dev, default)

### From Source

```bash
git clone https://github.com/cloud-shuttle/drover
cd drover
go build -o drover .
```

### With Go Install

```bash
go install github.com/cloud-shuttle/drover@latest
```

## Commands

| Command | Description |
|---------|-------------|
| `drover init` | Initialize Drover in current project |
| `drover run` | Execute all tasks to completion |
| `drover run --workers 8` | Run with 8 parallel agents |
| `drover run --epic <id>` | Run only tasks in specific epic |
| `drover add <title>` | Add a new task |
| `drover epic add <title>` | Create a new epic |
| `drover status` | Show current project status |
| `drover status --watch` | Live progress updates |
| `drover resume` | Resume interrupted workflows |

## Configuration

Drover uses sensible defaults but can be configured via environment variables or flags:

```bash
# Database (default: SQLite in .drover.db)
export DROVER_DATABASE_URL="postgresql://localhost/drover"

# Or use SQLite explicitly
export DROVER_DATABASE_URL="sqlite:///.drover.db"
```

### Task Options

```bash
# Set priority (higher = more urgent)
drover add "Critical fix" --priority 10

# Define dependencies
drover add "Build API" --blocked-by task-abc,task-def

# Assign to epic
drover add "New feature" --epic epic-xyz
```

## How It Works

### The Drover Loop

```
while project has incomplete tasks:
    1. Find all ready tasks (no unmet dependencies)
    2. Claim tasks up to worker limit
    3. Execute each task in isolated git worktree
    4. On success: commit, merge, unblock dependents
    5. On failure: retry or mark failed
    6. On blocked: create resolution task (optional)

    Sleep and repeat until complete
```

### Durability

Drover uses [DBOS](https://dbos.dev) for durable workflow execution. Every step is checkpointed to the database. If Drover crashes:

1. Restart with `drover resume`
2. DBOS automatically recovers all workflows from their last checkpoint
3. Partially completed tasks resume, not restart

### Task States

```
ready → claimed → in_progress → completed
                      ↓
                   blocked → (unblocked) → ready
                      ↓
                   failed (after max retries)
```

### Git Worktrees

Each worker operates in an isolated git worktree:

```
.drover/
└── worktrees/
    ├── task-a1b2/    # Worker 1
    ├── task-c3d4/    # Worker 2
    ├── task-e5f6/    # Worker 3
    └── task-g7h8/    # Worker 4
```

Changes are committed per-task and merged back to main upon completion.

## Examples

### Complete a Full Project

```bash
# You have a project with existing tasks
drover init
drover run

# Drover discovers all tasks, builds dependency graph,
# and drives everything to completion
```

### Parallel Feature Development

```bash
# Create an epic for a new feature
drover epic add "User Dashboard"

# Add independent tasks (can run in parallel)
drover add "Dashboard layout component" --epic epic-dash
drover add "Fetch user stats API" --epic epic-dash
drover add "User preferences storage" --epic epic-dash

# Add dependent tasks
drover add "Wire up dashboard to API" --epic epic-dash \
  --blocked-by task-layout,task-api

drover add "Add dashboard tests" --epic epic-dash \
  --blocked-by task-wire

# Run with high parallelism
drover run --workers 8 --epic epic-dash
```

### Resume After Interruption

```bash
# Running a long job...
drover run --workers 4
# ^C (interrupted) or crash

# Later, resume exactly where you left off
drover resume
# All workflows continue from last checkpoint
```

## Architecture

Drover is built on a pure Go stack:

| Component | Technology | Purpose |
|-----------|------------|---------|
| CLI | Cobra | Command-line interface |
| Workflows | DBOS Go | Durable execution |
| Database | PostgreSQL/SQLite | State persistence |
| AI Agent | Claude Code | Task execution |
| Isolation | Git Worktrees | Parallel workspaces |

See [DESIGN.md](./DESIGN.md) for detailed architecture documentation.

## Comparison

| Feature | Drover | Manual Claude | Cursor Background |
|---------|--------|---------------|-------------------|
| Parallel agents | ✅ 1-16+ | ❌ 1 | ✅ Limited |
| Crash recovery | ✅ Full | ❌ None | ❌ None |
| Dependency graph | ✅ Yes | ❌ Manual | ❌ No |
| Auto-retry | ✅ Yes | ❌ No | ❌ No |
| Progress tracking | ✅ Real-time | ❌ Manual | ⚠️ Basic |
| Git isolation | ✅ Worktrees | ❌ No | ❌ No |

## Contributing

Contributions welcome! Please read [CONTRIBUTING.md](./CONTRIBUTING.md) first.

```bash
# Run tests
go test ./...

# Run with race detector
go test -race ./...
```

## License

MIT — see [LICENSE](./LICENSE)

---

Built by [Cloud Shuttle](https://cloudshuttle.com.au) 🚀

*Drover keeps your tasks moving until they reach their destination: done.*
