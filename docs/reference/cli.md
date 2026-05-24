---
title: Command line interface (CLI) reference
description: Complete command syntax, options, subcommands, and flag specifications for the Drover Orchestrator.
product: drover-orchestrator
audience:
  - platform-operator
  - customer-admin
  - evaluator
doc_type: reference
topics:
  - agent-jobs
  - governance-policy
surface: repo-docs
---

# Command line interface (CLI) reference

The `drover` CLI is the unified command line interface for **Drover Orchestrator**, a durable, high-performance execution engine that routes parallel AI coding agents (`drover-worker` wrapping Claude Code) through git-sandboxed worktrees to complete complex software backlogs.

This document details every command, subcommand, flag, and configuration interface supported by the `drover` binary.

---

## Global syntax and flags

All Drover commands use standard Unix syntax:

```bash
drover [command] [subcommand] [flags]
```

### Global flags

| Flag | Short | Default | Description |
|---|---|---|---|
| `--help` | `-h` | — | Display help, subcommands, and syntax examples for the specified command. |
| `--version` | `-v` | — | Print the compiled binary version (e.g., `0.3.0`). |

---

## Initialization and setup

### `drover init`

Initialize a new local Drover workspace database and folder structure inside the current project root.

```bash
drover init
```

* **Storage Engine**: Creates a hidden `.drover/` directory in the current working directory.
* **Metadata Schema**: Initializes a local SQLite workspace database at `.drover/drover.db` to log tasks, epics, execution cycles, sandboxes, and events.
* **Workspace Seed**: Automatically spawns `.drover/task_template.yaml` to configure standardized, high-quality instructions for LLM planning agents.
* **Durable Database Modes**:
  * **Default (SQLite)**: Standalone, zero-dependency engine suitable for local development.
  * **Production (PostgreSQL)**: Automatically activated if the environment variable `DBOS_SYSTEM_DATABASE_URL` is set. Checkpoints, transactions, and queues will be routed to the specified PostgreSQL instance.

---

### `drover install`

Install the compiled `drover` binary globally on the host operating system.

```bash
drover install
```

* **Purpose**: Compiles the source and copies the binary to:
  * `~/bin/` (if it exists and is in the user's `$PATH`)
  * `/usr/local/bin/` (default Unix/macOS fallback, may require `sudo`)
  * `C:\Program Files\Drover\` (Windows systems)
* **Pre-requisites**: Requires Go and Git. If executed within the source repository, it automatically executes a platform-targeted `go build -o drover ./cmd/drover` before installation.

---

## Task and epic creation

### `drover add`

Create and register a new task inside the project backlog.

```bash
drover add "<title>" [flags]
```

* **Hierarchical IDs**: To create sub-tasks, pass a parent task ID with `--parent` (e.g., creating `task-123.1` under parent `task-123`) or specify the hierarchical sequence prefix directly in the title string (e.g., `drover add "task-123.4 Write authentication helper"`). The maximum nesting depth is 2 levels (Epic → Parent → Child).
* **Backlog Verification**: By default, Drover runs quality assurance checks on task titles and descriptions to ensure actionability (e.g. verifying defined acceptance criteria and target files). Use `--skip-validation` to override.

#### Flags

| Flag | Short | Default | Description |
|---|---|---|---|
| `--description` | `-d` | `""` | Detailed task specification supporting markdown formatting. |
| `--epic` | `-e` | `""` | Target Epic ID to bind the task to (e.g., `epic-101`). |
| `--parent` | `-P` | `""` | Parent task ID under which this task should be nested as a sub-task. |
| `--priority` | `-p` | `0` | Execution priority order index (higher values execute first). |
| `--blocked-by` | — | `[]` | Comma-separated list of task IDs that must complete before this task executes. |
| `--skip-validation` | — | `false` | Bypass backlog quality control validation checks. |
| `--test-mode` | — | `"strict"` | Verification failure constraint: `strict` (any broken test blocks merge), `lenient` (warns but permits merge), or `disabled`. |
| `--test-scope` | — | `"diff"` | Verification boundary limit: `diff` (run tests affecting edited files only), `all` (run full suite), or `skip`. |
| `--test-command` | — | `""` | Override command used by workers to verify changes (e.g. `make test-unit`). |

---

### `drover quick`

Rapidly capture a task in the local backlog with zero prompts, validation checks, or interactive friction.

```bash
drover quick "<title>"
```

* **Purpose**: Perfect for rapid captures during standups, meetings, or brainstorming.
* **Output**: Instantly logs the task and outputs the generated identifier (e.g. `⚡ Quick capture: task-824`).

---

### `drover epic`

Group related engineering tasks, user stories, and feature scopes into parent Epics.

```bash
drover epic [command]
```

#### Subcommands

##### `drover epic add`
Register a new Epic category.
* **Usage**: `drover epic add "<title>" [flags]`
* **Flags**:
  * `-d, --description`: A high-level description outlining the epic scope.

---

### `drover spec`

Ingest a design specification and auto-generate structured Epics, stories, parent tasks, and sub-task graphs.

```bash
drover spec <path> [flags]
```

* **Action Flow**: Leverages Claude to read raw markdown files or JSONL backlog sheets, construct logical task DAGs with dependencies, synthesize acceptance criteria, and seed the local workspace database.
* **Inputs**: Accepts a single specification file (e.g., `spec.md`) or a directory containing multiple design documents.

#### Flags

| Flag | Default | Description |
|---|---|---|
| `--dry-run` | `false` | Parse spec and preview task graph outputs without writing to the database. |
| `--yes` | `false` | Bypass confirmation prompts (`[y/N]`). |
| `--model` | `"claude-sonnet-4-20250514"` | Anthropic AI model used to process the specifications. |
| `--direct-api` | `false` | Bypasses local developer LLM proxy to hit the Anthropic API directly (requires `ANTHROPIC_API_KEY`). |

---

## Workflow execution

### `drover run`

Launch the parallel execution engine to drive all eligible ready tasks to completion.

```bash
drover run [flags]
```

* **Adaptive Concurrency**: Specify worker threads using `--workers`. Spawns sandboxed, independent git worktrees for each claimed task.
* **Worktree pre-warming (Pooling)**: Use `--pool` to accelerate worker startup. Warm pools maintain pre-built directories containing caches (`node_modules`, build artifacts), reducing bootstrap time.
* **Exactly-Once execution**: Built on DBOS durable workflows. If execution is terminated by a crash, network failure, or `Ctrl+C`, run `drover run` again to seamlessly pick up right at the last database checkpoint.

#### Flags

| Flag | Short | Default | Description |
|---|---|---|---|
| `--workers` | `-w` | `1` | Maximum parallel worker threads to spawn. |
| `--epic` | — | `""` | Restrict runner execution strictly to tasks associated with this epic ID. |
| `--require-approval` | — | `false` | Halts execution to require explicit human sign-off on generated planning documents. |
| `--mode` | — | `"combined"` | Execution phase restriction: `combined` (plan + build), `planning` (generate plans only), or `building` (execute approved plans). |
| `--building-approved-only` | — | `false` | Restricts building only to tasks with approved, verified implementation plans. |
| `--building-verify-steps` | — | `false` | Forces unit testing verification after every individual file modification step. |
| `--planning-auto-approve-low` | — | `false` | Auto-approve generated plans for low-complexity tasks. |
| `--planning-max-steps` | — | `20` | Maximum limit of instruction steps permitted in a plan. |
| `--refinement-enabled` | — | `false` | Auto-refine and regenerate planning documents on human rejection. |
| `--refinement-max-refinements` | — | `3` | Maximum refinement retries allowed before task is marked failed. |
| `--pool` | — | `false` | Enable warm git worktree pooling. |
| `--pool-min` | — | `2` | Minimum active warm worktrees kept in reserve. |
| `--pool-max` | — | `10` | Maximum capacity of pre-warmed worktrees. |
| `--verbose` | `-v` | `false` | Render detailed tracing logs and debug streams. |

---

### `drover resume`

A CLI recovery shortcut to pick up interrupted workflows.

```bash
drover resume
```

* **Behavior**: Since DBOS durable execution automatically checkpoints all workflow stages, running `drover resume` prompts the user to simply invoke `drover run`, which naturally recovers all partial tasks from their last database state.

---

## Planning management

### `drover plan`

Inspect, approve, or reject implementation plans constructed by planning workers prior to source code changes.

```bash
drover plan [command]
```

#### Subcommands

* **`list`**: Render all implementation plans in the workspace database. Filter using `--status` / `-s` (`draft`, `pending`, `approved`, `rejected`, `executing`, `completed`, `failed`).
* **`show <plan-id>`**: Render complete details of a specific plan, including modified files, target test suites, and individual step actions.
* **`approve <plan-id>`**: Mark a plan as approved to immediately queue the task for the coding worker stage.
* **`reject <plan-id>`**: Reject a plan, preventing coding changes. Triggers AI plan refinement loops if configured.
* **`delete <plan-id>`**: Permanently remove a draft plan from the database.
* **`review`**: Start an interactive CLI console to step through all pending implementation plans.

---

## Progress and status monitoring

### `drover status`

Display real-time progression, backlog sizing, and task execution graphs.

```bash
drover status [flags]
```

#### Flags

| Flag | Short | Default | Description |
|---|---|---|---|
| `--tree` | `-t` | `false` | Show a nested task dependency tree illustrating Epic binds and blockers. |
| `--watch` | `-w` | `false` | Render a live-updating status dashboard directly in the terminal interface. |
| `--oneline` | — | `false` | Print a single-line summary with completion percentage (ideal for shell prompts/CI logs). |

#### Output Example (`drover status --tree`)

```text
🐂 Drover Status: 67% complete (4/6 tasks)

📌 [Epic] ep-100: Auth Integration
├── ✅ task-1: Initialize JWT helper [succeeded]
├── ⏳ task-2: Add verification middleware [running]
│   └── 🔒 task-2.1: Add NY variant to button [blocked]
└── 🚫 task-3: Setup local redis test cache [failed]
```

---

### `drover watch`

Start a live terminal session showing active task progress, worker status, and raw tail logs in real-time.

```bash
drover watch
```

---

### `drover info`

Inspect full metadata, execution logs, sandboxing status, and error logs for a specific task.

```bash
drover info <task-id>
```

* **Outputs**: Includes registration timestamps, full description, epic relationships, dependency blockers, sandbox worktree paths, attempt history, and stdout/stderr output streams from all build and test execution cycles.

---

### `drover dashboard`

Launch the local web dashboard for a visual overview of your workspace.

```bash
drover dashboard [flags]
```

* **Features**: Provides a visual browser console on a local port displaying dependency flowcharts, task completion lists, live console tail streams, and system logs.

#### Flags

| Flag | Short | Default | Description |
|---|---|---|---|
| `--port` | `-p` | `"3847"` | Port binding for the dashboard server. |
| `--open` | — | `false` | Automatically open the dashboard in your default browser upon startup. |

---

### `drover stream`

Query or stream real-time task lifecycle events to automate integrations or pipeline scripts.

```bash
drover stream [flags]
```

* **Event Types**: Monitors `started`, `completed`, `failed`, `blocked`, `unblocked`, `cancelled`, `claimed`, `paused`, and `resumed`.
* **Output Format**: Supports human-readable log formats or streaming JSON Lines (`--jsonl`).

#### Flags

| Flag | Default | Description |
|---|---|---|
| `--type` | `[]` | Filter by event type (e.g. `task.completed,task.failed`). |
| `--epic` | `""` | Filter events to a specific Epic ID. |
| `--task` | `""` | Filter events to a specific Task ID. |
| `--since` | `""` | Include events after RFC3339 timestamp (e.g. `2026-05-22T12:00:00Z`). |
| `--until` | `""` | Include events up to RFC3339 timestamp. |
| `--follow` | `false` | Stream new events in real-time. |
| `--jsonl` | `false` | Print events in JSON Lines format. |
| `--limit` | `0` | Max historical events to return (0 indicates unlimited). |

---

## Execution flow control

### `drover pause`

Freeze execution of an active, running task workspace.

```bash
drover pause <task-id>
```

* **Effect**: Suspends the execution process inside the worker's git sandbox and locks the sandbox directory, allowing safe inspector intrusion.

---

### `drover resume-task`

Resume a paused task workspace.

```bash
drover resume-task <task-id> [flags]
```

* **Effect**: Resumes parallel agent execution inside the sandbox worktree.
* **Flags**:
  * `-H, --hint`: Inject custom steering instructions directly upon resume (e.g., `drover resume-task task-102 --hint "Make sure to import from pkg/errors"`).

---

### `drover edit`

Print the absolute path to a paused task's sandboxed git worktree.

```bash
drover edit <task-id>
```

* **Usage**: Allows engineers to open the isolated task workspace directory in their IDE (e.g., `code $(drover edit task-102)`), review partially written files, make manual code fixes, and run local testing cycles before unpausing.

---

### `drover cancel`

Stop a running agent and cancel its task execution.

```bash
drover cancel <task-id>
```

* **Behavior**: Terminates worker subprocesses, releases workspace locks, and registers the task status as `cancelled`.

---

### `drover retry`

Reschedule a failed or cancelled task.

```bash
drover retry <task-id> [flags]
```

* **Behavior**: Clears failure records, increments attempt limits, and reverts task status to `ready` for a fresh run in the scheduling queue.
* **Flags**:
  * `--force`: Reset attempt counters to retry even if the task has hit its maximum retry limits.

---

### `drover reset`

Reset task statuses in your backlog to return them to the `ready` state.

```bash
drover reset [TASK_IDS...] [flags]
```

* **Multi-Target**: If task IDs are provided, resets only those specific tasks. If no IDs are provided, resets tasks by status filters.
* **Defaults**: If called with no arguments or flags, automatically resets all tasks in `claimed`, `in-progress`, or `completed` states.

#### Flags

| Flag | Default | Description |
|---|---|---|
| `--completed` | `false` | Reset completed tasks back to `ready`. |
| `--in-progress` | `false` | Reset actively running, in-progress tasks back to `ready`. |
| `--claimed` | `false` | Reset claimed tasks. |
| `--failed` | `false` | Reset failed tasks (perfect for clearing blockages). |

---

### `drover resolve`

Manually unblock a task, bypassing its remaining dependency blockers.

```bash
drover resolve <task-id> [flags]
```

* **Behavior**: Removes all registered `blocked-by` task dependencies, marks the task status as `ready`, and records an engineering justification note in the event stream.
* **Flags**:
  * `--note`: Mandatory note describing how dependencies were resolved.

---

### `drover hint`

Send steering guidance directly to a running AI agent without interrupting its execution.

```bash
drover hint <task-id> "<message>"
```

* **Behavior**: Queues the string message and injects it into the agent's context window at the next available execution checkpoint. This allows developer intervention without killing the worker process.

---

## Worktree management

Each parallel worker executes inside a dedicated Git worktree sandbox to prevent workspace pollution. These commands help manage disk space.

```bash
drover worktree [command]
```

### Subcommands

* **`list`**: Render all currently active worktrees, matching task IDs, and disk space usage.
* **`prune`**: Delete worktree folders allocated to completed, failed, or cancelled tasks.
* **`cleanup`**: Unconditionally delete all local worktree folders to reclaim disk space.

---

## Data sharing and operators

### `drover operator`

Manage developer identities and API keys in multiplayer collaboration contexts.

```bash
drover operator [command]
```

#### Subcommands

* **`create <name>`**: Register a new operator identity and generate an authentication token.
* **`list`**: Render registered operators and their active permissions.
* **`delete <operator-id>`**: Revoke a specific developer's operator token access.
* **`login <name>`**: Log in as a registered operator.

---

### `drover share`

Generate a secure, compressed share token containing the current backlog schema, database tasks, and workspace state.

```bash
drover share
```

* **Usage**: Ideal for hands-on debugging or sending a state snapshot to other developers.

---

### `drover import-share`

Import and restore a collaborative session using a share token.

```bash
drover import-share <token>
```

---

### `drover export`

Export all database tasks and state schemas into a portable file format.

```bash
drover export [flags]
```

#### Flags

| Flag | Short | Default | Description |
|---|---|---|---|
| `--format` | `-f` | `"beads"` | Target format: `beads` (for beads syncing), `json` (portable handoff), or `yaml`. |
| `--output` | `-o` | `""` | Output file path. Defaults to `session-YYYY-MM-DD.drover` for JSON/YAML. |

---

### `drover import`

Import a complete task session from an export file.

```bash
drover import <path>
```

---

### `drover import-jsonl`

Import epics, stories, and task backlogs from standard spreadsheet-generated `.jsonl` sheets.

```bash
drover import-jsonl <path>
```

---

## Experimental and Developer Commands

### `drover dbos-demo`

Run a demonstration of DBOS-based workflow execution.

```bash
drover dbos-demo [flags]
```

* **Purpose**: Simulates state checkpointing, error recovery, built-in retry logic, and queue-based parallel execution on PostgreSQL.
* **Flags**:
  * `--queue`: Use queue-based parallel execution.

---

### `drover flags`

Manage feature flags for experimental capabilities.

```bash
drover flags
```

---

### `drover search`

Perform full-text search across task records.

```bash
drover search <query>
```

---

### `drover backpressure`

Configure adaptive concurrency limits to prevent OOM errors or rate-limits under heavy parallelism.

```bash
drover backpressure
```

---

### `drover proxy`

Manage settings for the local developer LLM API proxy.

```bash
drover proxy
```
