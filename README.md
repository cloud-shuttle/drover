# Drover

**No task left behind** 🐂

Drover is an orchestration tool that drives your project to completion using parallel AI agents. It discovers tasks from your project tracker, manages execution across multiple workers, and ensures every task gets done—even when things get blocked.

## Features

- **Parallel AI Workers**: Run multiple Claude Code instances simultaneously
- **Durable Execution**: Resume interrupted runs with checkpointed state
- **Auto-Unblocking**: Automatically creates tasks for newly discovered blockers
- **Progress Dashboard**: Real-time TUI for monitoring progress
- **Git Worktree Isolation**: Each worker gets its own isolated environment
- **Smart Scheduling**: Priority-based task queuing with dependency tracking

## Prerequisites

Drover integrates with two external tools:

1. **Beads (`bd`)**: A project tracker for managing tasks and epics
   - Install: See [Beads documentation](https://github.com/cloud-shuttle/beads)
   - Drover uses this to discover work and track completion

2. **Claude Code (`claude`)**: Anthropic's CLI for Claude
   - Install: `npm install -g @anthropic-ai/claude-code`
   - Drover spawns Claude Code instances to execute tasks

## Installation

### From source

```bash
git clone https://github.com/cloud-shuttle/drover.git
cd drover
cargo install --path .
```

### From crates.io (coming soon)

```bash
cargo install drover
```

## Configuration

Create a `.drover.toml` file in your project directory:

```toml
# Number of parallel workers (default: 4)
workers = 4

# Task timeout in seconds (default: 600)
timeout = 600

# Max retry attempts per task (default: 3)
retries = 3

# Auto-create tasks to fix blockers (default: true)
auto_unblock = true

# Database for durable state
# SQLite: sqlite://path/to/db.db
# Postgres: postgres://user:pass@host/db
database = "sqlite://.drover.db"

# Stall threshold in seconds (default: 300)
stall_threshold = 300

# Poll interval in milliseconds (default: 5000)
poll_interval = 5000

# Git worktree directory for parallel execution
# worktree_dir = ".drover/worktrees"
```

Run `drover run --help` to see command-line overrides for any config option.

## Usage

### 1. Muster: See what work exists

```bash
# List all tasks in the project
drover muster

# Show only ready tasks
drover muster --ready

# Show only blocked tasks
drover muster --blocked

# Show task dependencies
drover muster --deps

# Filter by epic
drover muster --epic EPIC-ID

# Output as JSON
drover muster --json
```

### 2. Status: Check project health

```bash
# Get overall project status
drover status

# Status for a specific epic
drover status --epic EPIC-ID
```

Output includes:
- Total / ready / blocked / completed task counts
- Progress percentage
- ETA estimation (for active runs)

### 3. Run: Execute tasks to completion

```bash
# Run with default config
drover run

# Use specific number of workers
drover run --workers 8

# Dry run - show what would be done
drover run --dry-run

# Enable TUI dashboard
drover run --dashboard

# Focus on a single epic
drover run --epic EPIC-ID

# Set task timeout
drover run --timeout 300

# Limit tasks processed (for testing)
drover run --limit 5

# Specify project directory
drover run --project-dir /path/to/project
```

During a run, Drover:
1. Discovers all tasks from Beads
2. Spawns N worker processes (N = workers config)
3. Each worker claims ready tasks and executes via Claude Code
4. Completed tasks are marked as closed in Beads
5. Blocked tasks trigger auto-creation of unblock tasks
6. Progress is checkpointed to the database

### 4. Resume: Continue interrupted runs

```bash
# List recent runs
drover resume

# Resume a specific run (by ID)
drover resume RUN-ID
```

Note: Full resume functionality is still in development. Currently, `drover resume` shows run history but restarting a run requires starting fresh with `drover run`.

## How It Works

### Task Discovery

Drover queries the `bd` tool for:
- **Epics**: Collections of related tasks
- **Tasks**: Individual work items with status, priority, blockers
- **Dependencies**: Tasks can be blocked by other tasks (via `blocked_by`)

### Worker Execution

Each worker:
1. Claims the highest-priority ready task
2. Creates a git worktree for isolation
3. Invokes Claude Code with the task description
4. Monitors for timeout or errors
5. Reports completion/failure back to the orchestrator

### Blocking & Auto-Unblocking

When a task fails due to a blocker:
1. Drover parses the error for blocker IDs
2. If `auto_unblock` is enabled, creates new tasks for each blocker
3. Dependent tasks are requeued when blockers complete

### Durable Execution

All state is persisted to SQLite/Postgres:
- Run metadata (start time, manifest, completion status)
- Task progress updates
- Can resume after crashes/interruptions

## Output

During a run, you'll see:

```
🐂 Drover - Epic: User Authentication

Target: Epic: User Authentication
Tasks: 25 total, 12 ready, 8 blocked
Workers: 4

[████████████████░░░░░░░░░░░░░░░░░░] 40.0% (10/25)

Status:
  Total:   25
  Ready:   12
  Blocked: 3
  Failed:  0

Epics:
  ○ User Authentication (40%)
  ○ API Design (0%)
```

## Development

### Running tests

```bash
cargo test
```

### Building for release

```bash
cargo build --release
```

The release binary will be at `target/release/drover`.

## License

MIT

## Contributing

Contributions welcome! Please open an issue or PR.
