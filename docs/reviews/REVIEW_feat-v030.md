# Review: `feat/v030` (New planning docs + task loader scripts)

**Review date:** 2026-01-15  
**Compared against:** `v0.2.0` (`12e6981`) → `HEAD` (`403ccbc`)

## Summary of changes

### What changed vs `v0.2.0`

- **Docs (planning/design)**:
  - `design/roborev-enhancements.md`: roadmap inspired by roborev (event streaming, config, context sizing, outcomes, CLI controls, context carrying).
  - `design/worktree-prewarming-dashboard.md`: roadmap inspired by Ramp Inspect (pre-warmed worktrees, dashboard improvements, HITL, subtasks, multiplayer, CLI UX).
- **Task-loading scripts**:
  - `load-tasks.sh`, `load-tasks-part2.sh`: create epics and seed ~81 planning tasks with dependencies.
- **Repo hygiene**:
  - `.gitignore`: now ignores local SQLite DBs (including `.drover/drover.db`).
  - `LICENSE`: added.
- **Tracked DB file**:
  - `.drover/drover.db` changed (binary). This is likely accidental / undesirable to version-control (see “Risks & recommendations”).

## End-to-end testing

### Automated tests (Go)

Executed on host using Homebrew Go (`/opt/homebrew/bin/go`):

- ✅ `go test ./...` passes

Notes:
- While doing the review, two pre-existing test failures were uncovered and fixed:
  - `cmd/drover` build break (missing `runDashboard`)
  - `internal/git` cleanup test failing (worktrees not being removed)

### Script-driven E2E: init → create epics → create tasks → status

Ran in a clean temporary project directory:

- ✅ `drover init`
- ✅ `bash ./load-tasks.sh` (with `SKIP_VALIDATION=1`)
- ✅ `drover status --tree` shows tasks + dependency-blocked state

Observations:
- The loader is intentionally creating “planning placeholder” tasks; the CLI’s task-quality validation rejects some of them if validation is enabled. The loader now defaults to `--skip-validation` unless `SKIP_VALIDATION=0`.

## Design conformance analysis (DBOS + Beads)

### DBOS (durable workflows)

**What’s aligned with the design:**
- The system’s main intent—durable execution and crash recovery via DBOS—is reflected in `internal/workflow/dbos_workflow.go` by wrapping side-effectful operations as DBOS steps (`dbos.RunAsStep`).
- Work is structured as “per-task” execution (`ExecuteTaskWorkflow`) which matches the design’s “ExecuteTaskWorkflow (×N)” concept.

**Where the implementation diverges from the design docs:**
- The design in `design/sequence.md` describes a durable orchestrator loop that:
  - queries “ready” tasks from the DB,
  - claims them atomically,
  - enqueues per-task workflows,
  - and performs a durable sleep between cycles.
  
  The current DBOS implementation largely expects tasks to be provided as input (POC-style), and its dependency enqueueing (`OnTaskComplete`) is explicitly incomplete (“For this POC, we just enqueue it.”).

**Recommendation to stay true to the DBOS design:**
- Promote the design’s orchestrator loop into the DBOS path (query/claim/enqueue within a durable workflow) and implement full dependency-unblock logic (only enqueue when *all* blockers complete).
- Audit DBOS step idempotency boundaries (worktree creation, commits, merges) so retries don’t create duplicate side effects.

### Beads (hierarchy + sync)

**What’s aligned with Beads-inspired design intent:**
- Hierarchical task IDs are supported and used in the Beads importer (`task-123.1` ⇒ parent `task-123`).

**Where the implementation diverges from typical Beads usage:**
- Beads import exists (`internal/beads/sync.go`) but is not integrated into CLI/workflows (no call sites). Export is only invoked from the legacy (non-DBOS) orchestrator when `AutoSyncBeads` is enabled.
- Export currently **rewrites** `.beads/beads.jsonl` from current state. If you intend Beads’ append-only “event log” semantics, this is a mismatch.
- Status mapping is lossy:
  - Drover `failed` becomes Beads `closed` (without setting `reason`), and `blocked/claimed` become `open`.

**Recommendation to stay true to Beads:**
- Decide whether `.beads/beads.jsonl` is treated as an append-only log or a snapshot. If log, switch export to append-only records and include `closed_at` + `reason`.
- Wire import/export into the DBOS execution path (not only the legacy orchestrator) so the “DBOS is default” story remains consistent.

## Risks & recommendations

- **Committed `.drover/drover.db`**: even if ignored now, it remains a tracked binary in git history and will constantly churn. Recommendation: remove it from version control and rely on `drover init` + loader scripts to seed state.
- **Loader portability**: the loader previously relied on GNU `grep -P` and bash associative arrays (not available on macOS default bash). It has been updated to avoid those pitfalls and to support targeting an arbitrary project dir via `PROJECT_DIR`.

## How to run the loader (recommended)

From repo root (creates epics + tasks in your target project):

```bash
PROJECT_DIR=/path/to/your/project \
DROVER=/path/to/drover \
SKIP_VALIDATION=1 \
bash ./load-tasks.sh
```

