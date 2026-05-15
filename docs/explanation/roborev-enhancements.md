# Drover Enhancement Design Document

**Version:** 1.0
**Date:** 2026-01-14
**Status:** Draft
**Author:** Generated from roborev analysis

---

## Executive Summary

This document outlines six major enhancements to Drover, inspired by analysis of the [roborev](https://github.com/wesm/roborev) project. These features will improve observability, configurability, and developer experience while maintaining Drover's core strengths in durable workflow orchestration and parallel AI agent execution.

### Key Enhancements

1. **Event Streaming System** - Real-time JSONL event output for integrations
2. **Project-Level Configuration** - `.drover.toml` support for per-project settings
3. **Context Window Management** - Intelligent handling of large content
4. **Structured Task Outcomes** - Pass/Fail verdict extraction from agent output
5. **Enhanced CLI Job Controls** - Cancel, retry, and resolve commands
6. **Task Context Carrying** - Memory of recent task decisions

### Effort Estimate

| Epic | Stories | Tasks | Estimated Effort |
|------|---------|-------|------------------|
| E1 - Event Streaming | 2 | 6 | 1-2 weeks |
| E2 - Project Configuration | 2 | 5 | 1 week |
| E3 - Context Window | 2 | 4 | 1 week |
| E4 - Structured Outcomes | 2 | 5 | 1-2 weeks |
| E5 - CLI Controls | 3 | 6 | 1 week |
| E6 - Context Carrying | 2 | 4 | 1 week |
| **Total** | **13** | **30** | **6-8 weeks** |

---

## Dependency Graph

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           PARALLEL EXECUTION TRACKS                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Track A: E1 Event Streaming (standalone)                                   │
│  ════════════════════════════════════════                                   │
│  S1.1 Core Event Bus                                                        │
│    └─► T1.1.1 Define event types                                            │
│          └─► T1.1.2 Implement event bus                                     │
│                └─► T1.1.3 Integrate with task state machine                 │
│  S1.2 Stream Command                                                        │
│    └─► T1.2.1 Add stream subcommand ◄── (depends on T1.1.3)                │
│          └─► T1.2.2 Add filtering options                                   │
│                └─► T1.2.3 Document streaming format                         │
│                                                                             │
│  Track B: E2 Project Configuration ──► E6 Context Carrying                  │
│  ═══════════════════════════════════════════════════════                    │
│  S2.1 Configuration Schema                                                  │
│    └─► T2.1.1 Define config struct ◄── (E3, E6 depend on this)             │
│          └─► T2.1.2 Implement config loading                                │
│                └─► T2.1.3 Add config validation                             │
│  S2.2 Task Guidelines Integration                                           │
│    └─► T2.2.1 Add guidelines to task context                                │
│          └─► T2.2.2 Support guideline templates                             │
│                        │                                                    │
│                        ▼                                                    │
│  S6.1 Context Configuration (E6)                                            │
│    └─► T6.1.1 Add context count config                                      │
│  S6.2 Context Injection                                                     │
│    └─► T6.2.1 Query recent completions                                      │
│          └─► T6.2.2 Format context for prompt                               │
│                └─► T6.2.3 Add context to task prompt                        │
│                                                                             │
│  Track C: E3 Context Window Management                                      │
│  ═════════════════════════════════════                                      │
│  S3.1 Content Size Detection                                                │
│    └─► T3.1.1 Add size thresholds to config ◄── (depends on T2.1.1)        │
│          └─► T3.1.2 Implement content sizing                                │
│  S3.2 Reference-Based Fallback                                              │
│    └─► T3.2.1 Create reference substitution                                 │
│          └─► T3.2.2 Add fetch instructions to prompt                        │
│                                                                             │
│  Track D: E4 Structured Task Outcomes (standalone)                          │
│  ═════════════════════════════════════════════════                          │
│  S4.1 Success Criteria Definition                                           │
│    └─► T4.1.1 Add success criteria field                                    │
│          └─► T4.1.2 Parse criteria from task description                    │
│  S4.2 Verdict Extraction                                                    │
│    └─► T4.2.1 Define verdict types                                          │
│          └─► T4.2.2 Implement verdict parser                                │
│                └─► T4.2.3 Add verdict to TUI                                │
│                                                                             │
│  Track E: E5 Enhanced CLI Controls (standalone)                             │
│  ══════════════════════════════════════════════                             │
│  S5.1 Cancel Command                                                        │
│    └─► T5.1.1 ─► T5.1.2 ─► T5.1.3                                          │
│  S5.2 Retry Command                                                         │
│    └─► T5.2.1 ─► T5.2.2                                                     │
│  S5.3 Resolve Command                                                       │
│    └─► T5.3.1 ──► T5.3.2 ──► T5.3.3                                        │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘

CRITICAL PATH: E2 (T2.1.1) must complete before E3 and E6 can start
```

---

## Epic 1: Event Streaming System

### Overview

Add real-time JSONL event streaming for task lifecycle events, enabling external integrations, notifications, and custom tooling. This mirrors roborev's `roborev stream` functionality.

### Motivation

- Enable Slack/Discord notifications for task completions
- Support custom dashboards and monitoring
- Allow integration with external CI/CD systems
- Provide audit trail for task execution

### Design

#### Event Types

```go
type EventType string

const (
    EventTaskStarted   EventType = "task.started"
    EventTaskCompleted EventType = "task.completed"
    EventTaskFailed    EventType = "task.failed"
    EventTaskBlocked   EventType = "task.blocked"
    EventTaskUnblocked EventType = "task.unblocked"
)

type TaskEvent struct {
    Type      EventType         `json:"type"`
    Timestamp time.Time         `json:"ts"`
    TaskID    string            `json:"task_id"`
    Project   string            `json:"project"`
    Title     string            `json:"title"`
    Worker    string            `json:"worker,omitempty"`
    State     string            `json:"state,omitempty"`
    PrevState string            `json:"prev_state,omitempty"`
    Duration  *time.Duration    `json:"duration_ms,omitempty"`
    Error     string            `json:"error,omitempty"`
    Verdict   string            `json:"verdict,omitempty"`
    Metadata  map[string]string `json:"metadata,omitempty"`
}
```

#### Event Bus Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        Event Bus                            │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  Publisher (Task State Machine)                      │   │
│  │  - Emits events on state transitions                 │   │
│  │  - Includes context: task, worker, duration          │   │
│  └─────────────────────┬───────────────────────────────┘   │
│                        │                                    │
│                        ▼                                    │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  Channel Hub (fan-out)                               │   │
│  │  - Thread-safe subscription management               │   │
│  │  - Buffered channels per subscriber                  │   │
│  │  - Graceful handling of slow consumers               │   │
│  └─────────────────────┬───────────────────────────────┘   │
│                        │                                    │
│         ┌──────────────┼──────────────┐                    │
│         ▼              ▼              ▼                    │
│  ┌───────────┐  ┌───────────┐  ┌───────────┐              │
│  │ Stream    │  │ TUI       │  │ Database  │              │
│  │ Command   │  │ Updates   │  │ Logger    │              │
│  └───────────┘  └───────────┘  └───────────┘              │
└─────────────────────────────────────────────────────────────┘
```

#### CLI Interface

```bash
# Stream all events
drover stream

# Filter by project
drover stream --project my-project

# Filter by event type
drover stream --state completed,failed

# Include historical events
drover stream --since 2024-01-01T00:00:00Z

# Combine with jq
drover stream | jq -c 'select(.type == "task.completed")'
```

#### Example Output

```jsonl
{"type":"task.started","ts":"2026-01-14T10:00:00Z","task_id":"T123","project":"drover","title":"Implement event bus","worker":"worker-1"}
{"type":"task.completed","ts":"2026-01-14T10:15:30Z","task_id":"T123","project":"drover","title":"Implement event bus","worker":"worker-1","duration_ms":930000,"verdict":"pass"}
{"type":"task.blocked","ts":"2026-01-14T10:16:00Z","task_id":"T124","project":"drover","title":"Add filtering","blocked_by":"T125"}
```

### Stories and Tasks

#### S1.1: Core Event Bus

| Task | Description | Success Criteria | Dependencies |
|------|-------------|------------------|--------------|
| T1.1.1 | Define event types | Event types defined with JSON tags; serialization tests pass | - |
| T1.1.2 | Implement event bus | Concurrent publish/subscribe works; no race conditions under `-race` flag | T1.1.1 |
| T1.1.3 | Integrate with task state machine | All state transitions emit corresponding events; integration tests verify | T1.1.2 |

#### S1.2: Stream Command

| Task | Description | Success Criteria | Dependencies |
|------|-------------|------------------|--------------|
| T1.2.1 | Add stream subcommand | Command runs; outputs events to stdout in JSONL format | T1.1.3 |
| T1.2.2 | Add filtering options | Filters work correctly; historical replay functions | T1.2.1 |
| T1.2.3 | Document streaming format | Documentation complete with examples | T1.2.2 |

---

## Epic 2: Project-Level Configuration

### Overview

Support `.drover.toml` per-project configuration for task guidelines, worker constraints, and agent preferences. This mirrors roborev's per-repo configuration approach.

### Motivation

- Different projects have different coding standards
- Some projects need more/fewer parallel workers
- Agent prompts should reflect project context
- Teams can customize behavior without global changes

### Design

#### Configuration Schema

```toml
# .drover.toml - Project-level configuration

# Agent configuration
agent = "claude-code"
max_workers = 4
task_timeout = "30m"

# Context settings (see Epic 6)
task_context_count = 5

# Size thresholds (see Epic 3)
max_description_size = "250KB"
max_diff_size = "250KB"

# Project-specific guidelines injected into agent prompts
guidelines = """
This is a Go project using DBOS for durability.
- Follow Go idioms and conventions
- Use structured logging with slog
- All public functions must have doc comments
- Prefer composition over inheritance
- Database migrations are not needed yet
"""

# Labels to apply to all tasks
default_labels = ["drover", "go"]

# Agent preferences
[agent_preferences]
model = "claude-sonnet-4-20250514"
temperature = 0.7
```

#### Configuration Hierarchy

```
Priority (highest to lowest):
1. CLI flags (--max-workers 8)
2. Environment variables (DROVER_MAX_WORKERS=8)
3. Project config (.drover.toml in project root)
4. Global config (~/.drover/config.toml)
5. Built-in defaults
```

#### Config Loading Implementation

```go
type DroverConfig struct {
    Agent             string            `toml:"agent"`
    MaxWorkers        int               `toml:"max_workers"`
    TaskTimeout       Duration          `toml:"task_timeout"`
    TaskContextCount  int               `toml:"task_context_count"`
    MaxDescriptionSize ByteSize         `toml:"max_description_size"`
    MaxDiffSize       ByteSize          `toml:"max_diff_size"`
    Guidelines        string            `toml:"guidelines"`
    DefaultLabels     []string          `toml:"default_labels"`
    AgentPreferences  map[string]any    `toml:"agent_preferences"`
}

func LoadConfig(projectRoot string) (*DroverConfig, error) {
    config := DefaultConfig()

    // Load global config
    if globalPath := GlobalConfigPath(); fileExists(globalPath) {
        if err := mergeConfig(config, globalPath); err != nil {
            return nil, err
        }
    }

    // Load project config (overrides global)
    projectPath := filepath.Join(projectRoot, ".drover.toml")
    if fileExists(projectPath) {
        if err := mergeConfig(config, projectPath); err != nil {
            return nil, err
        }
    }

    // Apply environment variables
    applyEnvOverrides(config)

    return config, config.Validate()
}
```

### Stories and Tasks

#### S2.1: Configuration Schema

| Task | Description | Success Criteria | Dependencies |
|------|-------------|------------------|--------------|
| T2.1.1 | Define config struct | Struct parses sample TOML files correctly | - |
| T2.1.2 | Implement config loading | Hierarchical config loading works; precedence correct | T2.1.1 |
| T2.1.3 | Add config validation | Invalid configs produce clear error messages | T2.1.2 |

#### S2.2: Task Guidelines Integration

| Task | Description | Success Criteria | Dependencies |
|------|-------------|------------------|--------------|
| T2.2.1 | Add guidelines to task context | Task prompts include guidelines text | T2.1.2 |
| T2.2.2 | Support guideline templates | Template variables replaced correctly | T2.2.1 |

---

## Epic 3: Context Window Management

### Overview

Intelligently manage large content to prevent context overflow, passing references instead of content when appropriate. This mirrors roborev's 250KB threshold approach.

### Motivation

- Large diffs can exceed context windows
- Attached files may be too large to inline
- Agents can fetch content themselves when needed
- Prevents silent truncation or failures

### Design

#### Size Detection Flow

```
┌─────────────────────────────────────────────────────────────┐
│                    Task Preparation                         │
│                                                             │
│  Task Content                                               │
│  ├── Description (markdown)                                 │
│  ├── Attached Files []                                      │
│  └── Git Diff (if applicable)                               │
│                                                             │
│                        │                                    │
│                        ▼                                    │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  Size Check                                          │   │
│  │  - description_size = len(description)               │   │
│  │  - files_size = sum(len(file) for file in files)    │   │
│  │  - diff_size = len(diff)                            │   │
│  └─────────────────────┬───────────────────────────────┘   │
│                        │                                    │
│         ┌──────────────┴──────────────┐                    │
│         ▼                             ▼                    │
│  ┌─────────────┐              ┌─────────────┐              │
│  │ Under       │              │ Over        │              │
│  │ Threshold   │              │ Threshold   │              │
│  │             │              │             │              │
│  │ Include     │              │ Replace     │              │
│  │ inline      │              │ with refs   │              │
│  └─────────────┘              └─────────────┘              │
└─────────────────────────────────────────────────────────────┘
```

#### Reference Format

```go
type ContentReference struct {
    Type    string `json:"type"`    // "file", "commit", "diff"
    Path    string `json:"path,omitempty"`
    SHA     string `json:"sha,omitempty"`
    Size    int64  `json:"size_bytes"`
    Command string `json:"fetch_command"`
}

// Example references
refs := []ContentReference{
    {
        Type:    "file",
        Path:    "internal/worker/executor.go",
        Size:    45000,
        Command: "cat internal/worker/executor.go",
    },
    {
        Type:    "diff",
        SHA:     "abc123",
        Size:    300000,
        Command: "git show abc123",
    },
}
```

#### Prompt Augmentation

When references are used instead of content:

```markdown
## Large Content Notice

The following content exceeds size thresholds and is provided as references.
You can fetch the content using the provided commands.

### References

| Type | Path/SHA | Size | Fetch Command |
|------|----------|------|---------------|
| file | internal/worker/executor.go | 45KB | `cat internal/worker/executor.go` |
| diff | abc123 | 300KB | `git show abc123` |

Please fetch the content you need to complete this task.
```

### Stories and Tasks

#### S3.1: Content Size Detection

| Task | Description | Success Criteria | Dependencies |
|------|-------------|------------------|--------------|
| T3.1.1 | Add size thresholds to config | Config options parsed; defaults applied | T2.1.1 |
| T3.1.2 | Implement content sizing | Size calculation accurate; threshold comparison works | T3.1.1 |

#### S3.2: Reference-Based Fallback

| Task | Description | Success Criteria | Dependencies |
|------|-------------|------------------|--------------|
| T3.2.1 | Create reference substitution | References generated correctly; JSON parseable | T3.1.2 |
| T3.2.2 | Add fetch instructions to prompt | Agents successfully fetch referenced content | T3.2.1 |

---

## Epic 4: Structured Task Outcomes

### Overview

Parse agent responses to extract structured completion status, improving blocker detection and reporting. This mirrors roborev's Pass/Fail verdict system.

### Motivation

- Quick visual triage in TUI
- Better blocker detection
- Automated success validation
- Metrics and reporting

### Design

#### Verdict Types

```go
type Verdict string

const (
    VerdictPass    Verdict = "pass"
    VerdictFail    Verdict = "fail"
    VerdictBlocked Verdict = "blocked"
    VerdictUnknown Verdict = "unknown"
)

type TaskResult struct {
    TaskID          string    `json:"task_id"`
    Verdict         Verdict   `json:"verdict"`
    Summary         string    `json:"summary"`
    CriteriaMet     []string  `json:"criteria_met"`
    CriteriaUnmet   []string  `json:"criteria_unmet"`
    BlockerReason   string    `json:"blocker_reason,omitempty"`
    AgentOutput     string    `json:"agent_output"`
    ParseConfidence float64   `json:"parse_confidence"`
}
```

#### Success Criteria Sources

```
Priority (highest to lowest):
1. Explicit criteria in task spec
2. Parsed from markdown checkboxes in description
3. Inferred from task type/labels
```

#### Verdict Parsing Logic

```go
func ParseVerdict(output string, criteria []string) TaskResult {
    result := TaskResult{
        AgentOutput: output,
        Verdict:     VerdictUnknown,
    }

    // Check for explicit verdict statements
    if matches := verdictRegex.FindStringSubmatch(output); len(matches) > 0 {
        result.Verdict = normalizeVerdict(matches[1])
        result.ParseConfidence = 0.9
        return result
    }

    // Check for blocker indicators
    blockerPatterns := []string{
        `blocked by`,
        `waiting for`,
        `cannot proceed`,
        `dependency not met`,
    }
    for _, pattern := range blockerPatterns {
        if strings.Contains(strings.ToLower(output), pattern) {
            result.Verdict = VerdictBlocked
            result.BlockerReason = extractBlockerReason(output, pattern)
            result.ParseConfidence = 0.8
            return result
        }
    }

    // Check for failure indicators
    failurePatterns := []string{
        `failed`,
        `error:`,
        `could not`,
        `unable to`,
    }
    // ... similar logic

    // Check criteria satisfaction
    if len(criteria) > 0 {
        result.CriteriaMet, result.CriteriaUnmet = checkCriteria(output, criteria)
        if len(result.CriteriaUnmet) == 0 {
            result.Verdict = VerdictPass
        } else {
            result.Verdict = VerdictFail
        }
        result.ParseConfidence = 0.7
    }

    return result
}
```

#### TUI Display

```
┌─────────────────────────────────────────────────────────────────────┐
│ Drover Task Queue                                    [h]elp [q]uit  │
├─────────────────────────────────────────────────────────────────────┤
│ ID     │ Title                          │ Status    │ Verdict      │
├────────┼────────────────────────────────┼───────────┼──────────────┤
│ T123   │ Implement event bus            │ Completed │ ✓ Pass       │  <- Green
│ T124   │ Add filtering options          │ Completed │ ✗ Fail       │  <- Red
│ T125   │ Database migration             │ Blocked   │ ⚠ Blocked    │  <- Yellow
│ T126   │ Update documentation           │ Running   │ -            │  <- Gray
│ T127   │ Integration tests              │ Pending   │ -            │
└─────────────────────────────────────────────────────────────────────┘
```

### Stories and Tasks

#### S4.1: Success Criteria Definition

| Task | Description | Success Criteria | Dependencies |
|------|-------------|------------------|--------------|
| T4.1.1 | Add success criteria field | Field persisted; round-trips through database | - |
| T4.1.2 | Parse criteria from task description | Checkboxes parsed to criteria array | T4.1.1 |

#### S4.2: Verdict Extraction

| Task | Description | Success Criteria | Dependencies |
|------|-------------|------------------|--------------|
| T4.2.1 | Define verdict types | Verdict types defined; serialization works | T4.1.1 |
| T4.2.2 | Implement verdict parser | Parser extracts correct verdict from sample outputs | T4.2.1 |
| T4.2.3 | Add verdict to TUI | Verdicts visible in TUI with correct colors | T4.2.2 |

---

## Epic 5: Enhanced CLI Job Controls

### Overview

Add CLI commands for fine-grained job control: cancel, retry, and manual blocker resolution. This mirrors roborev's `address` command and adds additional controls.

### Motivation

- Stop runaway tasks
- Retry transient failures
- Manually resolve blockers
- Better operational control

### Design

#### Command Interface

```bash
# Cancel a running or queued task
drover cancel <task-id>
drover cancel T123

# Retry a failed or cancelled task
drover retry <task-id>
drover retry T123 --force  # Retry even if completed

# Manually resolve a blocked task
drover resolve <task-id>
drover resolve T123 --note "Fixed manually in separate PR"
```

#### Cancel Implementation

```go
func (c *Coordinator) CancelTask(taskID string) error {
    task, err := c.store.GetTask(taskID)
    if err != nil {
        return fmt.Errorf("task not found: %w", err)
    }

    // Validate task is cancellable
    if !task.IsCancellable() {
        return fmt.Errorf("task %s in state %s cannot be cancelled",
            taskID, task.State)
    }

    // If running, signal worker to stop
    if task.State == StateRunning {
        if err := c.signalWorkerCancel(task.WorkerID); err != nil {
            log.Warn("failed to signal worker", "error", err)
        }
    }

    // Update state
    task.State = StateCancelled
    task.CancelledAt = time.Now()

    // Clean up worktree if applicable
    if task.WorktreePath != "" {
        go c.cleanupWorktree(task.WorktreePath)
    }

    // Persist and emit event
    if err := c.store.UpdateTask(task); err != nil {
        return err
    }

    c.eventBus.Publish(TaskEvent{
        Type:   EventTaskCancelled,
        TaskID: taskID,
    })

    return nil
}
```

#### DBOS Workflow Integration

```go
// Cancel must integrate with DBOS to prevent workflow resumption
func (c *Coordinator) CancelDBOSWorkflow(taskID string) error {
    // Mark in DBOS that this workflow should not resume
    return c.dbos.CancelWorkflow(context.Background(), taskID)
}
```

#### Retry State Machine

```
                    ┌─────────────┐
                    │   Failed    │
                    └──────┬──────┘
                           │ retry
                           ▼
┌─────────────┐      ┌─────────────┐      ┌─────────────┐
│  Cancelled  │─────►│   Pending   │─────►│   Running   │
└─────────────┘      └─────────────┘      └─────────────┘
       │ retry             ▲                     │
       └───────────────────┘                     │
                                                 ▼
                                          ┌─────────────┐
        retry --force                     │  Completed  │
        ─────────────────────────────────►└─────────────┘
```

### Stories and Tasks

#### S5.1: Cancel Command

| Task | Description | Success Criteria | Dependencies |
|------|-------------|------------------|--------------|
| T5.1.1 | Add cancel subcommand | Command parses task ID; validation errors clear | - |
| T5.1.2 | Implement cancellation | Running tasks stop; state updated; resources cleaned | T5.1.1 |
| T5.1.3 | Handle in-flight DBOS workflows | Cancelled workflows don't resume on restart | T5.1.2 |

#### S5.2: Retry Command

| Task | Description | Success Criteria | Dependencies |
|------|-------------|------------------|--------------|
| T5.2.1 | Add retry subcommand | Command parses options correctly | - |
| T5.2.2 | Implement retry logic | Task requeued; attempt history preserved | T5.2.1 |

#### S5.3: Resolve Command

| Task | Description | Success Criteria | Dependencies |
|------|-------------|------------------|--------------|
| T5.3.1 | Add resolve subcommand | Command accepts task ID and optional note | - |
| T5.3.2 | Implement manual resolution | Blocked tasks become runnable after resolution | T5.3.1 |
| T5.3.3 | Add resolution to TUI | TUI resolution workflow functions correctly | T5.3.2 |

---

## Epic 6: Task Context Carrying

### Overview

Include context from recent completed tasks to give agents memory of recent decisions and patterns. This mirrors roborev's `review_context_count` feature.

### Motivation

- Agents make more consistent decisions
- Patterns from recent work inform current task
- Reduces repeated questions
- Better project coherence

### Design

#### Context Query

```go
func (s *Store) GetRecentCompletedTasks(projectID string, limit int) ([]TaskSummary, error) {
    query := `
        SELECT id, title, verdict, summary, completed_at
        FROM tasks
        WHERE project_id = ?
          AND state = 'completed'
          AND verdict IN ('pass', 'fail')
        ORDER BY completed_at DESC
        LIMIT ?
    `
    // ...
}

type TaskSummary struct {
    ID          string    `json:"id"`
    Title       string    `json:"title"`
    Verdict     Verdict   `json:"verdict"`
    Summary     string    `json:"summary"`
    CompletedAt time.Time `json:"completed_at"`
}
```

#### Context Format

```markdown
## Recent Task Context

The following tasks were recently completed in this project.
Use this context to inform your approach.

### T122: Implement worker pool (Pass)
*Completed 2 hours ago*
> Implemented worker pool with configurable size. Used sync.Pool
> for efficiency. Added graceful shutdown with context cancellation.

### T121: Add database migrations (Pass)
*Completed 5 hours ago*
> Created migration system using golang-migrate. Migrations stored
> in migrations/ directory. Applied up/down pattern.

### T120: Fix race condition in scheduler (Pass)
*Completed yesterday*
> Root cause was unsynchronized map access. Fixed with sync.RWMutex.
> Added -race flag to CI pipeline.

---

## Current Task

[Original task description follows...]
```

#### Prompt Construction

```go
func (w *Worker) PreparePrompt(task *Task, config *DroverConfig) string {
    var sb strings.Builder

    // Add project guidelines (from Epic 2)
    if config.Guidelines != "" {
        sb.WriteString("## Project Guidelines\n\n")
        sb.WriteString(config.Guidelines)
        sb.WriteString("\n\n---\n\n")
    }

    // Add recent task context (this epic)
    if config.TaskContextCount > 0 {
        recent, _ := w.store.GetRecentCompletedTasks(
            task.ProjectID,
            config.TaskContextCount,
        )
        if len(recent) > 0 {
            sb.WriteString("## Recent Task Context\n\n")
            for _, r := range recent {
                sb.WriteString(formatTaskSummary(r))
            }
            sb.WriteString("\n---\n\n")
        }
    }

    // Add current task
    sb.WriteString("## Current Task\n\n")
    sb.WriteString(task.Description)

    return sb.String()
}
```

### Stories and Tasks

#### S6.1: Context Configuration

| Task | Description | Success Criteria | Dependencies |
|------|-------------|------------------|--------------|
| T6.1.1 | Add context count config | Config option works; validates positive integer | T2.1.1 |

#### S6.2: Context Injection

| Task | Description | Success Criteria | Dependencies |
|------|-------------|------------------|--------------|
| T6.2.1 | Query recent completions | Query returns correct tasks; performance acceptable | T6.1.1 |
| T6.2.2 | Format context for prompt | Context block well-formatted; not too verbose | T6.2.1 |
| T6.2.3 | Add context to task prompt | Workers receive augmented prompts; context visible | T6.2.2 |

---

## Implementation Phases

### Phase 1: Foundation (Weeks 1-2)
- E2: Project Configuration (required by E3 and E6)
- E5: CLI Controls (standalone, high value)

### Phase 2: Observability (Weeks 3-4)
- E1: Event Streaming
- E4: Structured Outcomes

### Phase 3: Intelligence (Weeks 5-6)
- E3: Context Window Management
- E6: Task Context Carrying

### Phase 4: Polish (Week 7-8)
- Integration testing
- Documentation
- Performance tuning

---

## Testing Strategy

### Unit Tests
- Config parsing and validation
- Event serialization
- Verdict parsing logic
- Size threshold calculations

### Integration Tests
- Event bus with multiple subscribers
- Config hierarchy precedence
- CLI command execution
- DBOS workflow cancellation

### End-to-End Tests
- Full task lifecycle with events
- Context injection verification
- Large content handling
- Manual resolution flow

---

## Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Event latency | < 100ms | Time from state change to stream output |
| Config load time | < 50ms | Time to load and merge configs |
| Verdict accuracy | > 90% | Manual review of parsed verdicts |
| Context relevance | > 80% | Agent feedback on context usefulness |
| Cancel reliability | 100% | Tasks never resume after cancel |

---

## Appendix: Full Task Inventory

| ID | Type | Title | Dependencies |
|----|------|-------|--------------|
| E1 | Epic | Event Streaming System | - |
| S1.1 | Story | Core Event Bus | - |
| T1.1.1 | Task | Define event types | - |
| T1.1.2 | Task | Implement event bus | T1.1.1 |
| T1.1.3 | Task | Integrate with task state machine | T1.1.2 |
| S1.2 | Story | Stream Command | S1.1 |
| T1.2.1 | Task | Add stream subcommand | T1.1.3 |
| T1.2.2 | Task | Add filtering options | T1.2.1 |
| T1.2.3 | Task | Document streaming format | T1.2.2 |
| E2 | Epic | Project-Level Configuration | - |
| S2.1 | Story | Configuration Schema | - |
| T2.1.1 | Task | Define config struct | - |
| T2.1.2 | Task | Implement config loading | T2.1.1 |
| T2.1.3 | Task | Add config validation | T2.1.2 |
| S2.2 | Story | Task Guidelines Integration | S2.1 |
| T2.2.1 | Task | Add guidelines to task context | T2.1.2 |
| T2.2.2 | Task | Support guideline templates | T2.2.1 |
| E3 | Epic | Context Window Management | - |
| S3.1 | Story | Content Size Detection | - |
| T3.1.1 | Task | Add size thresholds to config | T2.1.1 |
| T3.1.2 | Task | Implement content sizing | T3.1.1 |
| S3.2 | Story | Reference-Based Fallback | S3.1 |
| T3.2.1 | Task | Create reference substitution | T3.1.2 |
| T3.2.2 | Task | Add fetch instructions to prompt | T3.2.1 |
| E4 | Epic | Structured Task Outcomes | - |
| S4.1 | Story | Success Criteria Definition | - |
| T4.1.1 | Task | Add success criteria field | - |
| T4.1.2 | Task | Parse criteria from task description | T4.1.1 |
| S4.2 | Story | Verdict Extraction | S4.1 |
| T4.2.1 | Task | Define verdict types | T4.1.1 |
| T4.2.2 | Task | Implement verdict parser | T4.2.1 |
| T4.2.3 | Task | Add verdict to TUI | T4.2.2 |
| E5 | Epic | Enhanced CLI Job Controls | - |
| S5.1 | Story | Cancel Command | - |
| T5.1.1 | Task | Add cancel subcommand | - |
| T5.1.2 | Task | Implement cancellation | T5.1.1 |
| T5.1.3 | Task | Handle in-flight DBOS workflows | T5.1.2 |
| S5.2 | Story | Retry Command | - |
| T5.2.1 | Task | Add retry subcommand | - |
| T5.2.2 | Task | Implement retry logic | T5.2.1 |
| S5.3 | Story | Resolve Command | - |
| T5.3.1 | Task | Add resolve subcommand | - |
| T5.3.2 | Task | Implement manual resolution | T5.3.1 |
| T5.3.3 | Task | Add resolution to TUI | T5.3.2 |
| E6 | Epic | Task Context Carrying | E2 |
| S6.1 | Story | Context Configuration | S2.1 |
| T6.1.1 | Task | Add context count config | T2.1.1 |
| S6.2 | Story | Context Injection | S6.1 |
| T6.2.1 | Task | Query recent completions | T6.1.1 |
| T6.2.2 | Task | Format context for prompt | T6.2.1 |
| T6.2.3 | Task | Add context to task prompt | T6.2.2 |
