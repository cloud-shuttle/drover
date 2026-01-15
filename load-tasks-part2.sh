#!/bin/bash
set -e

DROVER="./drover"

# First run the epic creation
source ./load-tasks.sh

echo ""
echo "=== Creating Tasks ==="
echo ""

# ============================================
# E1: Event Streaming System Tasks
# ============================================
echo "Adding E1 tasks..."

OUTPUT=$($DROVER add "Define event types" --epic ${EPIC_IDS["E1"]} -d "Create Go structs for TaskStarted, TaskCompleted, TaskFailed, TaskBlocked, TaskUnblocked events with timestamp, task ID, project, worker, and metadata fields.")
TASK_1_1_1=$(echo "$OUTPUT" | extract_id)
echo "  T1.1.1: $TASK_1_1_1"

OUTPUT=$($DROVER add "Implement event bus" --epic ${EPIC_IDS["E1"]} -d "Create thread-safe event bus with Subscribe/Publish methods using Go channels. Support multiple subscribers." --blocked-by=$TASK_1_1_1)
TASK_1_1_2=$(echo "$OUTPUT" | extract_id)
echo "  T1.1.2: $TASK_1_1_2"

OUTPUT=$($DROVER add "Integrate with task state machine" --epic ${EPIC_IDS["E1"]} -d "Emit events from task state transitions in the DBOS workflow. Events include before/after state, duration, and error info." --blocked-by $TASK_1_1_2)
TASK_1_1_3=$(echo "$OUTPUT" | extract_id)
echo "  T1.1.3: $TASK_1_1_3"

OUTPUT=$($DROVER add "Add stream subcommand" --epic ${EPIC_IDS["E1"]} -d "Create drover stream command with --project filter flag. Connect to event bus and output JSON lines." --blocked-by $TASK_1_1_3)
TASK_1_2_1=$(echo "$OUTPUT" | extract_id)
echo "  T1.2.1: $TASK_1_2_1"

OUTPUT=$($DROVER add "Add filtering options" --epic ${EPIC_IDS["E1"]} -d "Support --project, --worker, --state filters. Add --since flag for historical events from database." --blocked-by $TASK_1_2_1)
TASK_1_2_2=$(echo "$OUTPUT" | extract_id)
echo "  T1.2.2: $TASK_1_2_2"

OUTPUT=$($DROVER add "Document streaming format" --epic ${EPIC_IDS["E1"]} -d "Update README with event types, example output, and jq usage patterns." --blocked-by $TASK_1_2_2)
TASK_1_2_3=$(echo "$OUTPUT" | extract_id)
echo "  T1.2.3: $TASK_1_2_3"

# ============================================
# E2: Project-Level Configuration Tasks
# ============================================
echo "Adding E2 tasks..."

OUTPUT=$($DROVER add "Define config struct" --epic ${EPIC_IDS["E2"]} -d "Create DroverConfig Go struct with fields: Guidelines string, MaxWorkers int, AgentPreferences map, Labels []string, Timeout duration.")
TASK_2_1_1=$(echo "$OUTPUT" | extract_id)
echo "  T2.1.1: $TASK_2_1_1"

OUTPUT=$($DROVER add "Implement config loading" --epic ${EPIC_IDS["E2"]} -d "Load .drover.toml from project root, merge with global ~/.drover/config.toml. Project settings override global." --blocked-by $TASK_2_1_1)
TASK_2_1_2=$(echo "$OUTPUT" | extract_id)
echo "  T2.1.2: $TASK_2_1_2"

OUTPUT=$($DROVER add "Add config validation" --epic ${EPIC_IDS["E2"]} -d "Validate config values (positive MaxWorkers, valid duration strings, known agent names)." --blocked-by $TASK_2_1_2)
TASK_2_1_3=$(echo "$OUTPUT" | extract_id)
echo "  T2.1.3: $TASK_2_1_3"

OUTPUT=$($DROVER add "Add guidelines to task context" --epic ${EPIC_IDS["E2"]} -d "When preparing task for worker, include Guidelines from project config in the context passed to Claude Code." --blocked-by $TASK_2_1_2)
TASK_2_2_1=$(echo "$OUTPUT" | extract_id)
echo "  T2.2.1: $TASK_2_2_1"

OUTPUT=$($DROVER add "Support guideline templates" --epic ${EPIC_IDS["E2"]} -d "Allow {{project}}, {{task_type}}, {{labels}} placeholders in guidelines. Expand at runtime." --blocked-by $TASK_2_2_1)
TASK_2_2_2=$(echo "$OUTPUT" | extract_id)
echo "  T2.2.2: $TASK_2_2_2"

# ============================================
# E3: Context Window Management Tasks
# ============================================
echo "Adding E3 tasks..."

OUTPUT=$($DROVER add "Add size thresholds to config" --epic ${EPIC_IDS["E3"]} -d "Add MaxDescriptionSize, MaxDiffSize config options (default 250KB like roborev). Sizes in bytes." --blocked-by $TASK_2_1_1)
TASK_3_1_1=$(echo "$OUTPUT" | extract_id)
echo "  T3.1.1: $TASK_3_1_1"

OUTPUT=$($DROVER add "Implement content sizing" --epic ${EPIC_IDS["E3"]} -d "Calculate byte size of task description, attached files, and diff content. Compare against thresholds." --blocked-by $TASK_3_1_1)
TASK_3_1_2=$(echo "$OUTPUT" | extract_id)
echo "  T3.1.2: $TASK_3_1_2"

OUTPUT=$($DROVER add "Create reference substitution" --epic ${EPIC_IDS["E3"]} -d "Replace large content with structured references: {type: file, path: ...} or {type: commit, sha: ...}." --blocked-by $TASK_3_1_2)
TASK_3_2_1=$(echo "$OUTPUT" | extract_id)
echo "  T3.2.1: $TASK_3_2_1"

OUTPUT=$($DROVER add "Add fetch instructions to prompt" --epic ${EPIC_IDS["E3"]} -d "When using references, add instructions for agent to fetch content via git show, cat, etc." --blocked-by $TASK_3_2_1)
TASK_3_2_2=$(echo "$OUTPUT" | extract_id)
echo "  T3.2.2: $TASK_3_2_2"

# ============================================
# E4: Structured Task Outcomes Tasks
# ============================================
echo "Adding E4 tasks..."

OUTPUT=$($DROVER add "Add success criteria field" --epic ${EPIC_IDS["E4"]} -d "Add SuccessCriteria []string field to Task struct. Store in database. Sync from Beads.")
TASK_4_1_1=$(echo "$OUTPUT" | extract_id)
echo "  T4.1.1: $TASK_4_1_1"

OUTPUT=$($DROVER add "Parse criteria from task description" --epic ${EPIC_IDS["E4"]} -d "Extract success criteria from markdown checkboxes (- [ ] criteria) in task description if not explicitly set." --blocked-by $TASK_4_1_1)
TASK_4_1_2=$(echo "$OUTPUT" | extract_id)
echo "  T4.1.2: $TASK_4_1_2"

OUTPUT=$($DROVER add "Define verdict types" --epic ${EPIC_IDS["E4"]} -d "Create Verdict enum: Pass, Fail, Blocked, Unknown. Add Verdict field to TaskResult." --blocked-by $TASK_4_1_1)
TASK_4_2_1=$(echo "$OUTPUT" | extract_id)
echo "  T4.2.1: $TASK_4_2_1"

OUTPUT=$($DROVER add "Implement verdict parser" --epic ${EPIC_IDS["E4"]} -d "Parse agent output for verdict indicators. Look for explicit pass/fail statements, error patterns, blocker mentions." --blocked-by $TASK_4_2_1)
TASK_4_2_2=$(echo "$OUTPUT" | extract_id)
echo "  T4.2.2: $TASK_4_2_2"

OUTPUT=$($DROVER add "Add verdict to TUI" --epic ${EPIC_IDS["E4"]} -d "Display verdict with color coding in TUI: green for Pass, red for Fail, yellow for Blocked." --blocked-by $TASK_4_2_2)
TASK_4_2_3=$(echo "$OUTPUT" | extract_id)
echo "  T4.2.3: $TASK_4_2_3"

# ============================================
# E5: Enhanced CLI Job Controls Tasks
# ============================================
echo "Adding E5 tasks..."

OUTPUT=$($DROVER add "Add cancel subcommand" --epic ${EPIC_IDS["E5"]} -d "Create drover cancel <task-id> command. Validate task exists and is cancellable.")
TASK_5_1_1=$(echo "$OUTPUT" | extract_id)
echo "  T5.1.1: $TASK_5_1_1"

OUTPUT=$($DROVER add "Implement cancellation" --epic ${EPIC_IDS["E5"]} -d "Send cancel signal to worker. Update task state to Cancelled. Clean up worktree if applicable." --blocked-by $TASK_5_1_1)
TASK_5_1_2=$(echo "$OUTPUT" | extract_id)
echo "  T5.1.2: $TASK_5_1_2"

OUTPUT=$($DROVER add "Handle in-flight DBOS workflows" --epic ${EPIC_IDS["E5"]} -d "Integrate with DBOS workflow cancellation. Ensure durability guarantees maintained." --blocked-by $TASK_5_1_2)
TASK_5_1_3=$(echo "$OUTPUT" | extract_id)
echo "  T5.1.3: $TASK_5_1_3"

OUTPUT=$($DROVER add "Add retry subcommand" --epic ${EPIC_IDS["E5"]} -d "Create drover retry <task-id> [--force] command. --force allows retrying completed tasks.")
TASK_5_2_1=$(echo "$OUTPUT" | extract_id)
echo "  T5.2.1: $TASK_5_2_1"

OUTPUT=$($DROVER add "Implement retry logic" --epic ${EPIC_IDS["E5"]} -d "Reset task state to Pending. Increment retry count. Preserve previous attempt logs." --blocked-by $TASK_5_2_1)
TASK_5_2_2=$(echo "$OUTPUT" | extract_id)
echo "  T5.2.2: $TASK_5_2_2"

OUTPUT=$($DROVER add "Add resolve subcommand" --epic ${EPIC_IDS["E5"]} -d "Create drover resolve <task-id> [--note 'resolution note'] command.")
TASK_5_3_1=$(echo "$OUTPUT" | extract_id)
echo "  T5.3.1: $TASK_5_3_1"

OUTPUT=$($DROVER add "Implement manual resolution" --epic ${EPIC_IDS["E5"]} -d "Mark blocker as resolved. Store resolution note. Trigger dependency check for blocked tasks." --blocked-by $TASK_5_3_1)
TASK_5_3_2=$(echo "$OUTPUT" | extract_id)
echo "  T5.3.2: $TASK_5_3_2"

OUTPUT=$($DROVER add "Add resolution to TUI" --epic ${EPIC_IDS["E5"]} -d "Add 'r' keybinding in TUI to resolve selected blocked task with prompt for note." --blocked-by $TASK_5_3_2)
TASK_5_3_3=$(echo "$OUTPUT" | extract_id)
echo "  T5.3.3: $TASK_5_3_3"

# ============================================
# E6: Task Context Carrying Tasks
# ============================================
echo "Adding E6 tasks..."

OUTPUT=$($DROVER add "Add context count config" --epic ${EPIC_IDS["E6"]} -d "Add TaskContextCount int to config (default 3). Controls how many recent task summaries to include." --blocked-by $TASK_2_1_1)
TASK_6_1_1=$(echo "$OUTPUT" | extract_id)
echo "  T6.1.1: $TASK_6_1_1"

OUTPUT=$($DROVER add "Query recent completions" --epic ${EPIC_IDS["E6"]} -d "Query database for last N completed tasks in same project. Include title, outcome, and key decisions." --blocked-by $TASK_6_1_1)
TASK_6_2_1=$(echo "$OUTPUT" | extract_id)
echo "  T6.2.1: $TASK_6_2_1"

OUTPUT=$($DROVER add "Format context for prompt" --epic ${EPIC_IDS["E6"]} -d "Format recent task summaries as structured context block. Include task IDs for reference." --blocked-by $TASK_6_2_1)
TASK_6_2_2=$(echo "$OUTPUT" | extract_id)
echo "  T6.2.2: $TASK_6_2_2"

OUTPUT=$($DROVER add "Add context to task prompt" --epic ${EPIC_IDS["E6"]} -d "Prepend context block to task description when dispatching to worker." --blocked-by $TASK_6_2_2)
TASK_6_2_3=$(echo "$OUTPUT" | extract_id)
echo "  T6.2.3: $TASK_6_2_3"

echo ""
echo "=== Roborev tasks loaded ==="
echo ""
echo "=== Now loading Worktree/Dashboard tasks ==="
echo ""

# ============================================
# Worktree Pre-warming Tasks
# ============================================
echo "Adding Worktree Pre-warming tasks..."

OUTPUT=$($DROVER add "Design worktree pool data structure" --epic ${EPIC_IDS["worktree"]} -d "Pool supports configurable size, tracks worktree state (cold/warm/in-use), and handles cleanup")
TASK_001=$(echo "$OUTPUT" | extract_id)
echo "  task-001: $TASK_001"

OUTPUT=$($DROVER add "Implement pool initialization on drover start" --epic ${EPIC_IDS["worktree"]} -d "Pool pre-creates N worktrees in background, configurable via --pool-size flag" --blocked-by $TASK_001)
TASK_002=$(echo "$OUTPUT" | extract_id)
echo "  task-002: $TASK_002"

OUTPUT=$($DROVER add "Add pool replenishment logic" --epic ${EPIC_IDS["worktree"]} -d "Pool automatically creates new worktrees when available count drops below threshold" --blocked-by $TASK_002)
TASK_003=$(echo "$OUTPUT" | extract_id)
echo "  task-003: $TASK_003"

OUTPUT=$($DROVER add "Add pool cleanup on shutdown" --epic ${EPIC_IDS["worktree"]} -d "Graceful shutdown removes unused pooled worktrees, preserves in-use ones" --blocked-by $TASK_002)
TASK_004=$(echo "$OUTPUT" | extract_id)
echo "  task-004: $TASK_004"

OUTPUT=$($DROVER add "Implement shared node_modules via symlinks" --epic ${EPIC_IDS["worktree"]} -d "Worktrees share a single node_modules directory, reducing disk usage by 80%+" --blocked-by $TASK_002)
TASK_005=$(echo "$OUTPUT" | extract_id)
echo "  task-005: $TASK_005"

OUTPUT=$($DROVER add "Implement Go module cache sharing" --epic ${EPIC_IDS["worktree"]} -d "GOMODCACHE is shared across worktrees, first install cached for subsequent workers" --blocked-by $TASK_002)
TASK_006=$(echo "$OUTPUT" | extract_id)
echo "  task-006: $TASK_006"

OUTPUT=$($DROVER add "Add cache invalidation on dependency changes" --epic ${EPIC_IDS["worktree"]} -d "Lock file changes trigger cache rebuild, hash-based detection" --blocked-by $TASK_005,$TASK_006)
TASK_007=$(echo "$OUTPUT" | extract_id)
echo "  task-007: $TASK_007"

OUTPUT=$($DROVER add "Implement async git fetch with completion signal" --epic ${EPIC_IDS["worktree"]} -d "Git fetch runs in background, signals completion via channel")
TASK_008=$(echo "$OUTPUT" | extract_id)
echo "  task-008: $TASK_008"

OUTPUT=$($DROVER add "Add read-only mode during sync" --epic ${EPIC_IDS["worktree"]} -d "Claude Code can read files during sync, write operations queued until sync complete" --blocked-by $TASK_008)
TASK_009=$(echo "$OUTPUT" | extract_id)
echo "  task-009: $TASK_009"

OUTPUT=$($DROVER add "Add sync status to worker telemetry" --epic ${EPIC_IDS["worktree"]} -d "Dashboard shows sync status, time-to-ready metrics per worker" --blocked-by $TASK_008)
TASK_010=$(echo "$OUTPUT" | extract_id)
echo "  task-010: $TASK_010"

# ============================================
# Enhanced Observability Tasks
# ============================================
echo "Adding Enhanced Observability tasks..."

OUTPUT=$($DROVER add "Add completion rate by task type metric" --epic ${EPIC_IDS["observability"]} -d "Dashboard shows % of tasks completed successfully, grouped by epic/label")
TASK_011=$(echo "$OUTPUT" | extract_id)
echo "  task-011: $TASK_011"

OUTPUT=$($DROVER add "Add time-to-completion histogram" --epic ${EPIC_IDS["observability"]} -d "Histogram shows distribution of task durations, p50/p90/p99 visible" --blocked-by $TASK_011)
TASK_012=$(echo "$OUTPUT" | extract_id)
echo "  task-012: $TASK_012"

OUTPUT=$($DROVER add "Add failure pattern analysis" --epic ${EPIC_IDS["observability"]} -d "Dashboard shows common failure reasons, error message clustering" --blocked-by $TASK_011)
TASK_013=$(echo "$OUTPUT" | extract_id)
echo "  task-013: $TASK_013"

OUTPUT=$($DROVER add "Add worker utilization gauge" --epic ${EPIC_IDS["observability"]} -d "Real-time gauge shows % of workers actively processing tasks" --blocked-by $TASK_011)
TASK_014=$(echo "$OUTPUT" | extract_id)
echo "  task-014: $TASK_014"

OUTPUT=$($DROVER add "Add tasks-per-worker throughput metric" --epic ${EPIC_IDS["observability"]} -d "Chart shows tasks completed per worker over time, identifies bottlenecks" --blocked-by $TASK_014)
TASK_015=$(echo "$OUTPUT" | extract_id)
echo "  task-015: $TASK_015"

OUTPUT=$($DROVER add "Add idle time tracking" --epic ${EPIC_IDS["observability"]} -d "Dashboard shows cumulative idle time, alerts when workers consistently underutilized" --blocked-by $TASK_014)
TASK_016=$(echo "$OUTPUT" | extract_id)
echo "  task-016: $TASK_016"

OUTPUT=$($DROVER add "Implement WebSocket event stream" --epic ${EPIC_IDS["observability"]} -d "Dashboard receives real-time task state changes via WebSocket")
TASK_017=$(echo "$OUTPUT" | extract_id)
echo "  task-017: $TASK_017"

OUTPUT=$($DROVER add "Add activity feed UI component" --epic ${EPIC_IDS["observability"]} -d "Scrolling feed shows recent task transitions with timestamps" --blocked-by $TASK_017)
TASK_018=$(echo "$OUTPUT" | extract_id)
echo "  task-018: $TASK_018"

OUTPUT=$($DROVER add "Add active prompting counter" --epic ${EPIC_IDS["observability"]} -d "Dashboard shows count of workers currently prompting Claude Code" --blocked-by $TASK_017)
TASK_019=$(echo "$OUTPUT" | extract_id)
echo "  task-019: $TASK_019"

# ============================================
# Agent-Spawned Sub-Tasks
# ============================================
echo "Adding Agent-Spawned Sub-Tasks tasks..."

OUTPUT=$($DROVER add "Design sub-task creation API" --epic ${EPIC_IDS["subtasks"]} -d "API supports creating sub-tasks with parent reference, automatic dependency")
TASK_020=$(echo "$OUTPUT" | extract_id)
echo "  task-020: $TASK_020"

OUTPUT=$($DROVER add "Implement MCP tool for sub-task creation" --epic ${EPIC_IDS["subtasks"]} -d "Claude Code can call create_subtask tool, task appears in Drover queue" --blocked-by $TASK_020)
TASK_021=$(echo "$OUTPUT" | extract_id)
echo "  task-021: $TASK_021"

OUTPUT=$($DROVER add "Add sub-task status polling tool" --epic ${EPIC_IDS["subtasks"]} -d "Claude Code can check status of spawned sub-tasks, await completion" --blocked-by $TASK_021)
TASK_022=$(echo "$OUTPUT" | extract_id)
echo "  task-022: $TASK_022"

OUTPUT=$($DROVER add "Add research task type flag" --epic ${EPIC_IDS["subtasks"]} -d "Tasks can be marked as research-only, no git commits expected" --blocked-by $TASK_020)
TASK_023=$(echo "$OUTPUT" | extract_id)
echo "  task-023: $TASK_023"

OUTPUT=$($DROVER add "Implement cross-repo research context" --epic ${EPIC_IDS["subtasks"]} -d "Research tasks can access multiple repos read-only, aggregate findings" --blocked-by $TASK_023)
TASK_024=$(echo "$OUTPUT" | extract_id)
echo "  task-024: $TASK_024"

OUTPUT=$($DROVER add "Add research result storage" --epic ${EPIC_IDS["subtasks"]} -d "Research findings stored in task metadata, accessible by parent task" --blocked-by $TASK_024)
TASK_025=$(echo "$OUTPUT" | extract_id)
echo "  task-025: $TASK_025"

OUTPUT=$($DROVER add "Add decomposition signal mechanism" --epic ${EPIC_IDS["subtasks"]} -d "Agent can signal task-too-large, returns suggested breakdown" --blocked-by $TASK_021)
TASK_026=$(echo "$OUTPUT" | extract_id)
echo "  task-026: $TASK_026"

OUTPUT=$($DROVER add "Implement automatic sub-task creation from breakdown" --epic ${EPIC_IDS["subtasks"]} -d "Suggested breakdown automatically creates linked sub-tasks" --blocked-by $TASK_026)
TASK_027=$(echo "$OUTPUT" | extract_id)
echo "  task-027: $TASK_027"

OUTPUT=$($DROVER add "Add parent task auto-completion on sub-task completion" --epic ${EPIC_IDS["subtasks"]} -d "Parent task marked complete when all sub-tasks finish" --blocked-by $TASK_027)
TASK_028=$(echo "$OUTPUT" | extract_id)
echo "  task-028: $TASK_028"

# ============================================
# Human-in-the-Loop Tasks
# ============================================
echo "Adding Human-in-the-Loop tasks..."

OUTPUT=$($DROVER add "Implement task pause command" --epic ${EPIC_IDS["hitl"]} -d "drover pause <task-id> gracefully stops Claude Code, preserves worktree state")
TASK_029=$(echo "$OUTPUT" | extract_id)
echo "  task-029: $TASK_029"

OUTPUT=$($DROVER add "Add manual intervention mode" --epic ${EPIC_IDS["hitl"]} -d "Paused worktree accessible for manual editing, changes preserved on resume" --blocked-by $TASK_029)
TASK_030=$(echo "$OUTPUT" | extract_id)
echo "  task-030: $TASK_030"

OUTPUT=$($DROVER add "Implement task resume with context" --epic ${EPIC_IDS["hitl"]} -d "drover resume <task-id> continues with updated worktree, optional guidance prompt" --blocked-by $TASK_030)
TASK_031=$(echo "$OUTPUT" | extract_id)
echo "  task-031: $TASK_031"

OUTPUT=$($DROVER add "Add guidance queue per task" --epic ${EPIC_IDS["hitl"]} -d "Tasks maintain a queue of pending guidance messages")
TASK_032=$(echo "$OUTPUT" | extract_id)
echo "  task-032: $TASK_032"

OUTPUT=$($DROVER add "Implement drover hint command" --epic ${EPIC_IDS["hitl"]} -d "drover hint <task-id> 'try X approach' adds guidance to queue" --blocked-by $TASK_032)
TASK_033=$(echo "$OUTPUT" | extract_id)
echo "  task-033: $TASK_033"

OUTPUT=$($DROVER add "Inject guidance at Claude Code prompt boundary" --epic ${EPIC_IDS["hitl"]} -d "Guidance injected at natural pause points, Claude Code sees updated context" --blocked-by $TASK_033)
TASK_034=$(echo "$OUTPUT" | extract_id)
echo "  task-034: $TASK_034"

OUTPUT=$($DROVER add "Add pause/resume buttons to task cards" --epic ${EPIC_IDS["hitl"]} -d "Dashboard shows pause/resume buttons, status updates in real-time" --blocked-by $TASK_029,$TASK_017)
TASK_035=$(echo "$OUTPUT" | extract_id)
echo "  task-035: $TASK_035"

OUTPUT=$($DROVER add "Add guidance input field" --epic ${EPIC_IDS["hitl"]} -d "Text input on task card sends guidance via drover hint" --blocked-by $TASK_033,$TASK_035)
TASK_036=$(echo "$OUTPUT" | extract_id)
echo "  task-036: $TASK_036"

OUTPUT=$($DROVER add "Add worktree file browser" --epic ${EPIC_IDS["hitl"]} -d "Dashboard shows worktree files, read-only view during execution" --blocked-by $TASK_035)
TASK_037=$(echo "$OUTPUT" | extract_id)
echo "  task-037: $TASK_037"

# ============================================
# Session Handoff Tasks
# ============================================
echo "Adding Session Handoff tasks..."

OUTPUT=$($DROVER add "Add session export command" --epic ${EPIC_IDS["multiplayer"]} -d "drover export exports full session state to portable format")
TASK_038=$(echo "$OUTPUT" | extract_id)
echo "  task-038: $TASK_038"

OUTPUT=$($DROVER add "Add session import command" --epic ${EPIC_IDS["multiplayer"]} -d "drover import resumes session from exported state on different machine" --blocked-by $TASK_038)
TASK_039=$(echo "$OUTPUT" | extract_id)
echo "  task-039: $TASK_039"

OUTPUT=$($DROVER add "Add session URL sharing" --epic ${EPIC_IDS["multiplayer"]} -d "Dashboard generates shareable URL for live session viewing" --blocked-by $TASK_017)
TASK_040=$(echo "$OUTPUT" | extract_id)
echo "  task-040: $TASK_040"

OUTPUT=$($DROVER add "Add operator field to task model" --epic ${EPIC_IDS["multiplayer"]} -d "Tasks track creating operator, git commits attributed correctly")
TASK_041=$(echo "$OUTPUT" | extract_id)
echo "  task-041: $TASK_041"

OUTPUT=$($DROVER add "Update TUI/dashboard to show operator" --epic ${EPIC_IDS["multiplayer"]} -d "Task cards show operator name/avatar, filter by operator available" --blocked-by $TASK_041)
TASK_042=$(echo "$OUTPUT" | extract_id)
echo "  task-042: $TASK_042"

OUTPUT=$($DROVER add "Add operator authentication" --epic ${EPIC_IDS["multiplayer"]} -d "Operators authenticate via GitHub, commits use their identity" --blocked-by $TASK_041)
TASK_043=$(echo "$OUTPUT" | extract_id)
echo "  task-043: $TASK_043"

# ============================================
# CLI Ergonomics Tasks
# ============================================
echo "Adding CLI Ergonomics tasks..."

OUTPUT=$($DROVER add "Add drover quick command" --epic ${EPIC_IDS["cli"]} -d "drover quick 'fix the login bug' creates task with minimal input")
TASK_044=$(echo "$OUTPUT" | extract_id)
echo "  task-044: $TASK_044"

OUTPUT=$($DROVER add "Add AI-assisted task enrichment" --epic ${EPIC_IDS["cli"]} -d "Quick tasks auto-enriched with suggested dependencies, labels, epic assignment" --blocked-by $TASK_044)
TASK_045=$(echo "$OUTPUT" | extract_id)
echo "  task-045: $TASK_045"

OUTPUT=$($DROVER add "Add voice input support" --epic ${EPIC_IDS["cli"]} -d "drover voice captures speech-to-text task description" --blocked-by $TASK_044)
TASK_046=$(echo "$OUTPUT" | extract_id)
echo "  task-046: $TASK_046"

OUTPUT=$($DROVER add "Add drover status --oneline" --epic ${EPIC_IDS["cli"]} -d "Single line showing: X running, Y queued, Z completed, W blocked")
TASK_047=$(echo "$OUTPUT" | extract_id)
echo "  task-047: $TASK_047"

OUTPUT=$($DROVER add "Add drover watch command" --epic ${EPIC_IDS["cli"]} -d "Auto-refreshing status display without full TUI" --blocked-by $TASK_047)
TASK_048=$(echo "$OUTPUT" | extract_id)
echo "  task-048: $TASK_048"

OUTPUT=$($DROVER add "Add shell prompt integration" --epic ${EPIC_IDS["cli"]} -d "Drover status snippet for PS1/starship prompt integration" --blocked-by $TASK_047)
TASK_049=$(echo "$OUTPUT" | extract_id)
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
