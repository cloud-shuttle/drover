#!/bin/bash
set -e

DROVER="./drover"

# First run the epic creation
source ./load-tasks.sh

echo ""
echo "=== Creating Tasks ==="
echo ""

# Helper function to extract ID from drover output
extract_id() {
    grep -oP '(?<=Created task )[a-z0-9-]+|(?<=Created )[a-z0-9-]+' || echo ""
}

# Helper to add task with dependencies
add_task() {
    local title="$1"
    local epic="${EPIC_IDS[$2]}"
    local desc="$3"
    shift 3
    local blocked_by=("$@")

    local cmd="$DROVER add \"$title\" --epic $epic -d \"$desc\""

    # Add blocked-by flags
    for dep in "${blocked_by[@]}"; do
        cmd="$cmd --blocked-by=$dep"
    done

    OUTPUT=$(eval "$cmd")
    extract_id <<< "$OUTPUT"
}

# ============================================
# E1: Event Streaming System Tasks
# ============================================
echo "Adding E1 tasks..."

TASK_1_1_1=$(add_task "Define event types" "E1" "Create Go structs for TaskStarted, TaskCompleted, TaskFailed, TaskBlocked, TaskUnblocked events with timestamp, task ID, project, worker, and metadata fields.")
echo "  T1.1.1: $TASK_1_1_1"

TASK_1_1_2=$(add_task "Implement event bus" "E1" "Create thread-safe event bus with Subscribe/Publish methods using Go channels. Support multiple subscribers." "$TASK_1_1_1")
echo "  T1.1.2: $TASK_1_1_2"

TASK_1_1_3=$(add_task "Integrate with task state machine" "E1" "Emit events from task state transitions in the DBOS workflow. Events include before/after state, duration, and error info." "$TASK_1_1_2")
echo "  T1.1.3: $TASK_1_1_3"

TASK_1_2_1=$(add_task "Add stream subcommand" "E1" "Create drover stream command with --project filter flag. Connect to event bus and output JSON lines." "$TASK_1_1_3")
echo "  T1.2.1: $TASK_1_2_1"

TASK_1_2_2=$(add_task "Add filtering options" "E1" "Support --project, --worker, --state filters. Add --since flag for historical events from database." "$TASK_1_2_1")
echo "  T1.2.2: $TASK_1_2_2"

TASK_1_2_3=$(add_task "Document streaming format" "E1" "Update README with event types, example output, and jq usage patterns." "$TASK_1_2_2")
echo "  T1.2.3: $TASK_1_2_3"

# ============================================
# E2: Project-Level Configuration Tasks
# ============================================
echo "Adding E2 tasks..."

TASK_2_1_1=$(add_task "Define config struct" "E2" "Create DroverConfig Go struct with fields: Guidelines string, MaxWorkers int, AgentPreferences map, Labels []string, Timeout duration.")
echo "  T2.1.1: $TASK_2_1_1"

TASK_2_1_2=$(add_task "Implement config loading" "E2" "Load .drover.toml from project root, merge with global ~/.drover/config.toml. Project settings override global." "$TASK_2_1_1")
echo "  T2.1.2: $TASK_2_1_2"

TASK_2_1_3=$(add_task "Add config validation" "E2" "Validate config values (positive MaxWorkers, valid duration strings, known agent names)." "$TASK_2_1_2")
echo "  T2.1.3: $TASK_2_1_3"

TASK_2_2_1=$(add_task "Add guidelines to task context" "E2" "When preparing task for worker, include Guidelines from project config in the context passed to Claude Code." "$TASK_2_1_2")
echo "  T2.2.1: $TASK_2_2_1"

TASK_2_2_2=$(add_task "Support guideline templates" "E2" "Allow {{project}}, {{task_type}}, {{labels}} placeholders in guidelines. Expand at runtime." "$TASK_2_2_1")
echo "  T2.2.2: $TASK_2_2_2"

# ============================================
# E3: Context Window Management Tasks
# ============================================
echo "Adding E3 tasks..."

TASK_3_1_1=$(add_task "Add size thresholds to config" "E3" "Add MaxDescriptionSize, MaxDiffSize config options (default 250KB like roborev). Sizes in bytes." "$TASK_2_1_1")
echo "  T3.1.1: $TASK_3_1_1"

TASK_3_1_2=$(add_task "Implement content sizing" "E3" "Calculate byte size of task description, attached files, and diff content. Compare against thresholds." "$TASK_3_1_1")
echo "  T3.1.2: $TASK_3_1_2"

TASK_3_2_1=$(add_task "Create reference substitution" "E3" "Replace large content with structured references: {type: file, path: ...} or {type: commit, sha: ...}." "$TASK_3_1_2")
echo "  T3.2.1: $TASK_3_2_1"

TASK_3_2_2=$(add_task "Add fetch instructions to prompt" "E3" "When using references, add instructions for agent to fetch content via git show, cat, etc." "$TASK_3_2_1")
echo "  T3.2.2: $TASK_3_2_2"

# ============================================
# E4: Structured Task Outcomes Tasks
# ============================================
echo "Adding E4 tasks..."

TASK_4_1_1=$(add_task "Add success criteria field" "E4" "Add SuccessCriteria []string field to Task struct. Store in database. Sync from Beads.")
echo "  T4.1.1: $TASK_4_1_1"

TASK_4_1_2=$(add_task "Parse criteria from task description" "E4" "Extract success criteria from markdown checkboxes (- [ ] criteria) in task description if not explicitly set." "$TASK_4_1_1")
echo "  T4.1.2: $TASK_4_1_2"

TASK_4_2_1=$(add_task "Define verdict types" "E4" "Create Verdict enum: Pass, Fail, Blocked, Unknown. Add Verdict field to TaskResult." "$TASK_4_1_1")
echo "  T4.2.1: $TASK_4_2_1"

TASK_4_2_2=$(add_task "Implement verdict parser" "E4" "Parse agent output for verdict indicators. Look for explicit pass/fail statements, error patterns, blocker mentions." "$TASK_4_2_1")
echo "  T4.2.2: $TASK_4_2_2"

TASK_4_2_3=$(add_task "Add verdict to TUI" "E4" "Display verdict with color coding in TUI: green for Pass, red for Fail, yellow for Blocked." "$TASK_4_2_2")
echo "  T4.2.3: $TASK_4_2_3"

# ============================================
# E5: Enhanced CLI Job Controls Tasks
# ============================================
echo "Adding E5 tasks..."

TASK_5_1_1=$(add_task "Add cancel subcommand" "E5" "Create drover cancel <task-id> command. Validate task exists and is cancellable.")
echo "  T5.1.1: $TASK_5_1_1"

TASK_5_1_2=$(add_task "Implement cancellation" "E5" "Send cancel signal to worker. Update task state to Cancelled. Clean up worktree if applicable." "$TASK_5_1_1")
echo "  T5.1.2: $TASK_5_1_2"

TASK_5_1_3=$(add_task "Handle in-flight DBOS workflows" "E5" "Integrate with DBOS workflow cancellation. Ensure durability guarantees maintained." "$TASK_5_1_2")
echo "  T5.1.3: $TASK_5_1_3"

TASK_5_2_1=$(add_task "Add retry subcommand" "E5" "Create drover retry <task-id> [--force] command. --force allows retrying completed tasks.")
echo "  T5.2.1: $TASK_5_2_1"

TASK_5_2_2=$(add_task "Implement retry logic" "E5" "Reset task state to Pending. Increment retry count. Preserve previous attempt logs." "$TASK_5_2_1")
echo "  T5.2.2: $TASK_5_2_2"

TASK_5_3_1=$(add_task "Add resolve subcommand" "E5" "Create drover resolve <task-id> [--note 'resolution note'] command.")
echo "  T5.3.1: $TASK_5_3_1"

TASK_5_3_2=$(add_task "Implement manual resolution" "E5" "Mark blocker as resolved. Store resolution note. Trigger dependency check for blocked tasks." "$TASK_5_3_1")
echo "  T5.3.2: $TASK_5_3_2"

TASK_5_3_3=$(add_task "Add resolution to TUI" "E5" "Add 'r' keybinding in TUI to resolve selected blocked task with prompt for note." "$TASK_5_3_2")
echo "  T5.3.3: $TASK_5_3_3"

# ============================================
# E6: Task Context Carrying Tasks
# ============================================
echo "Adding E6 tasks..."

TASK_6_1_1=$(add_task "Add context count config" "E6" "Add TaskContextCount int to config (default 3). Controls how many recent task summaries to include." "$TASK_2_1_1")
echo "  T6.1.1: $TASK_6_1_1"

TASK_6_2_1=$(add_task "Query recent completions" "E6" "Query database for last N completed tasks in same project. Include title, outcome, and key decisions." "$TASK_6_1_1")
echo "  T6.2.1: $TASK_6_2_1"

TASK_6_2_2=$(add_task "Format context for prompt" "E6" "Format recent task summaries as structured context block. Include task IDs for reference." "$TASK_6_2_1")
echo "  T6.2.2: $TASK_6_2_2"

TASK_6_2_3=$(add_task "Add context to task prompt" "E6" "Prepend context block to task description when dispatching to worker." "$TASK_6_2_2")
echo "  T6.2.3: $TASK_6_2_3"

# ============================================
# Worktree Pre-warming Tasks
# ============================================
echo "Adding Worktree Pre-warming tasks..."

TASK_001=$(add_task "Design worktree pool data structure" "worktree" "Pool supports configurable size, tracks worktree state (cold/warm/in-use), and handles cleanup")
echo "  task-001: $TASK_001"

TASK_002=$(add_task "Implement pool initialization on drover start" "worktree" "Pool pre-creates N worktrees in background, configurable via --pool-size flag" "$TASK_001")
echo "  task-002: $TASK_002"

TASK_003=$(add_task "Add pool replenishment logic" "worktree" "Pool automatically creates new worktrees when available count drops below threshold" "$TASK_002")
echo "  task-003: $TASK_003"

TASK_004=$(add_task "Add pool cleanup on shutdown" "worktree" "Graceful shutdown removes unused pooled worktrees, preserves in-use ones" "$TASK_002")
echo "  task-004: $TASK_004"

TASK_005=$(add_task "Implement shared node_modules via symlinks" "worktree" "Worktrees share a single node_modules directory, reducing disk usage by 80%+" "$TASK_002")
echo "  task-005: $TASK_005"

TASK_006=$(add_task "Implement Go module cache sharing" "worktree" "GOMODCACHE is shared across worktrees, first install cached for subsequent workers" "$TASK_002")
echo "  task-006: $TASK_006"

# For task-007, we need multiple dependencies - pass them as separate args
TASK_007=$(add_task "Add cache invalidation on dependency changes" "worktree" "Lock file changes trigger cache rebuild, hash-based detection" "$TASK_005" "$TASK_006")
echo "  task-007: $TASK_007"

TASK_008=$(add_task "Implement async git fetch with completion signal" "worktree" "Git fetch runs in background, signals completion via channel")
echo "  task-008: $TASK_008"

TASK_009=$(add_task "Add read-only mode during sync" "worktree" "Claude Code can read files during sync, write operations queued until sync complete" "$TASK_008")
echo "  task-009: $TASK_009"

TASK_010=$(add_task "Add sync status to worker telemetry" "worktree" "Dashboard shows sync status, time-to-ready metrics per worker" "$TASK_008")
echo "  task-010: $TASK_010"

# ============================================
# Enhanced Observability Tasks
# ============================================
echo "Adding Enhanced Observability tasks..."

TASK_011=$(add_task "Add completion rate by task type metric" "observability" "Dashboard shows % of tasks completed successfully, grouped by epic/label")
echo "  task-011: $TASK_011"

TASK_012=$(add_task "Add time-to-completion histogram" "observability" "Histogram shows distribution of task durations, p50/p90/p99 visible" "$TASK_011")
echo "  task-012: $TASK_012"

TASK_013=$(add_task "Add failure pattern analysis" "observability" "Dashboard shows common failure reasons, error message clustering" "$TASK_011")
echo "  task-013: $TASK_013"

TASK_014=$(add_task "Add worker utilization gauge" "observability" "Real-time gauge shows % of workers actively processing tasks" "$TASK_011")
echo "  task-014: $TASK_014"

TASK_015=$(add_task "Add tasks-per-worker throughput metric" "observability" "Chart shows tasks completed per worker over time, identifies bottlenecks" "$TASK_014")
echo "  task-015: $TASK_015"

TASK_016=$(add_task "Add idle time tracking" "observability" "Dashboard shows cumulative idle time, alerts when workers consistently underutilized" "$TASK_014")
echo "  task-016: $TASK_016"

TASK_017=$(add_task "Implement WebSocket event stream" "observability" "Dashboard receives real-time task state changes via WebSocket")
echo "  task-017: $TASK_017"

TASK_018=$(add_task "Add activity feed UI component" "observability" "Scrolling feed shows recent task transitions with timestamps" "$TASK_017")
echo "  task-018: $TASK_018"

TASK_019=$(add_task "Add active prompting counter" "observability" "Dashboard shows count of workers currently prompting Claude Code" "$TASK_017")
echo "  task-019: $TASK_019"

# ============================================
# Agent-Spawned Sub-Tasks
# ============================================
echo "Adding Agent-Spawned Sub-Tasks tasks..."

TASK_020=$(add_task "Design sub-task creation API" "subtasks" "API supports creating sub-tasks with parent reference, automatic dependency")
echo "  task-020: $TASK_020"

TASK_021=$(add_task "Implement MCP tool for sub-task creation" "subtasks" "Claude Code can call create_subtask tool, task appears in Drover queue" "$TASK_020")
echo "  task-021: $TASK_021"

TASK_022=$(add_task "Add sub-task status polling tool" "subtasks" "Claude Code can check status of spawned sub-tasks, await completion" "$TASK_021")
echo "  task-022: $TASK_022"

TASK_023=$(add_task "Add research task type flag" "subtasks" "Tasks can be marked as research-only, no git commits expected" "$TASK_020")
echo "  task-023: $TASK_023"

TASK_024=$(add_task "Implement cross-repo research context" "subtasks" "Research tasks can access multiple repos read-only, aggregate findings" "$TASK_023")
echo "  task-024: $TASK_024"

TASK_025=$(add_task "Add research result storage" "subtasks" "Research findings stored in task metadata, accessible by parent task" "$TASK_024")
echo "  task-025: $TASK_025"

TASK_026=$(add_task "Add decomposition signal mechanism" "subtasks" "Agent can signal task-too-large, returns suggested breakdown" "$TASK_021")
echo "  task-026: $TASK_026"

TASK_027=$(add_task "Implement automatic sub-task creation from breakdown" "subtasks" "Suggested breakdown automatically creates linked sub-tasks" "$TASK_026")
echo "  task-027: $TASK_027"

TASK_028=$(add_task "Add parent task auto-completion on sub-task completion" "subtasks" "Parent task marked complete when all sub-tasks finish" "$TASK_027")
echo "  task-028: $TASK_028"

# ============================================
# Human-in-the-Loop Tasks
# ============================================
echo "Adding Human-in-the-Loop tasks..."

TASK_029=$(add_task "Implement task pause command" "hitl" "drover pause <task-id> gracefully stops Claude Code, preserves worktree state")
echo "  task-029: $TASK_029"

TASK_030=$(add_task "Add manual intervention mode" "hitl" "Paused worktree accessible for manual editing, changes preserved on resume" "$TASK_029")
echo "  task-030: $TASK_030"

TASK_031=$(add_task "Implement task resume with context" "hitl" "drover resume <task-id> continues with updated worktree, optional guidance prompt" "$TASK_030")
echo "  task-031: $TASK_031"

TASK_032=$(add_task "Add guidance queue per task" "hitl" "Tasks maintain a queue of pending guidance messages")
echo "  task-032: $TASK_032"

TASK_033=$(add_task "Implement drover hint command" "hitl" "drover hint <task-id> 'try X approach' adds guidance to queue" "$TASK_032")
echo "  task-033: $TASK_033"

TASK_034=$(add_task "Inject guidance at Claude Code prompt boundary" "hitl" "Guidance injected at natural pause points, Claude Code sees updated context" "$TASK_033")
echo "  task-034: $TASK_034"

# Task 35 has two dependencies - pass both
TASK_035=$(add_task "Add pause/resume buttons to task cards" "hitl" "Dashboard shows pause/resume buttons, status updates in real-time" "$TASK_029" "$TASK_017")
echo "  task-035: $TASK_035"

# Task 36 has two dependencies
TASK_036=$(add_task "Add guidance input field" "hitl" "Text input on task card sends guidance via drover hint" "$TASK_033" "$TASK_035")
echo "  task-036: $TASK_036"

TASK_037=$(add_task "Add worktree file browser" "hitl" "Dashboard shows worktree files, read-only view during execution" "$TASK_035")
echo "  task-037: $TASK_037"

# ============================================
# Session Handoff Tasks
# ============================================
echo "Adding Session Handoff tasks..."

TASK_038=$(add_task "Add session export command" "multiplayer" "drover export exports full session state to portable format")
echo "  task-038: $TASK_038"

TASK_039=$(add_task "Add session import command" "multiplayer" "drover import resumes session from exported state on different machine" "$TASK_038")
echo "  task-039: $TASK_039"

TASK_040=$(add_task "Add session URL sharing" "multiplayer" "Dashboard generates shareable URL for live session viewing" "$TASK_017")
echo "  task-040: $TASK_040"

TASK_041=$(add_task "Add operator field to task model" "multiplayer" "Tasks track creating operator, git commits attributed correctly")
echo "  task-041: $TASK_041"

TASK_042=$(add_task "Update TUI/dashboard to show operator" "multiplayer" "Task cards show operator name/avatar, filter by operator available" "$TASK_041")
echo "  task-042: $TASK_042"

TASK_043=$(add_task "Add operator authentication" "multiplayer" "Operators authenticate via GitHub, commits use their identity" "$TASK_041")
echo "  task-043: $TASK_043"

# ============================================
# CLI Ergonomics Tasks
# ============================================
echo "Adding CLI Ergonomics tasks..."

TASK_044=$(add_task "Add drover quick command" "cli" "drover quick 'fix the login bug' creates task with minimal input")
echo "  task-044: $TASK_044"

TASK_045=$(add_task "Add AI-assisted task enrichment" "cli" "Quick tasks auto-enriched with suggested dependencies, labels, epic assignment" "$TASK_044")
echo "  task-045: $TASK_045"

TASK_046=$(add_task "Add voice input support" "cli" "drover voice captures speech-to-text task description" "$TASK_044")
echo "  task-046: $TASK_046"

TASK_047=$(add_task "Add drover status --oneline" "cli" "Single line showing: X running, Y queued, Z completed, W blocked")
echo "  task-047: $TASK_047"

TASK_048=$(add_task "Add drover watch command" "cli" "Auto-refreshing status display without full TUI" "$TASK_047")
echo "  task-048: $TASK_048"

TASK_049=$(add_task "Add shell prompt integration" "cli" "Drover status snippet for PS1/starship prompt integration" "$TASK_047")
echo "  task-049: $TASK_049"

echo ""
echo "=== All tasks loaded! ==="
echo ""
echo "Summary:"
echo "- E1 (Event Streaming): 6 tasks"
echo "- E2 (Project Config): 5 tasks"
echo "- E3 (Context Window): 4 tasks"
echo "- E4 (Structured Outcomes): 5 tasks"
echo "- E5 (CLI Controls): 8 tasks"
echo "- E6 (Context Carrying): 4 tasks"
echo "- Worktree Pre-warming: 10 tasks"
echo "- Enhanced Observability: 9 tasks"
echo "- Agent-Spawned Sub-Tasks: 9 tasks"
echo "- Human-in-the-Loop: 9 tasks"
echo "- Session Handoff: 6 tasks"
echo "- CLI Ergonomics: 6 tasks"
echo ""
echo "Total: 81 tasks loaded"
echo ""
echo "Run './drover status' to see all tasks"
echo "Run './drover run --workers 4' to start working on them"
