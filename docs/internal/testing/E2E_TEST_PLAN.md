---
title: End-to-End Testing Plan for Drover Features
description: End-to-End Testing Plan for Drover Features
product: drover-orchestrator
audience: platform-operator
doc_type: how-to
topics:
  - agent-jobs
surface: repo-docs
---
# End-to-End Testing Plan for Drover Features

**Date:** 2026-01-14  
**Status:** Test Plan (Features Not Yet Implemented)  
**Scope:** Comprehensive E2E testing scenarios for all planned features

---

## Overview

This document outlines end-to-end test scenarios for the features described in:
- `design/roborev-enhancements.md` (E1-E6)
- `design/worktree-prewarming-dashboard.md` (Epic 1-6)

**Note:** These features are currently in design phase. Tests should be implemented alongside feature development.

---

## Test Environment Setup

### Prerequisites

```bash
# 1. Fresh Drover initialization
cd /tmp/drover-test
rm -rf .drover
drover init

# 2. Test repository setup
git init
echo "# Test Project" > README.md
git add README.md
git commit -m "Initial commit"

# 3. Agent availability
which claude-code  # or codex, amp, opencode
export DROVER_AGENT_TYPE=claude  # or codex, amp, opencode

# 4. DBOS setup (optional for production tests)
export DBOS_SYSTEM_DATABASE_URL=postgresql://localhost/drover_test
```

### Test Data

Create test epics and tasks using `load-tasks.sh` and `load-tasks-part2.sh`:

```bash
# Load all test tasks
./load-tasks.sh
./load-tasks-part2.sh

# Verify tasks loaded
drover status --tree | head -20
```

---

## Epic 1: Event Streaming System (E1)

### Test E1.1: Event Bus Basic Functionality

**Objective:** Verify events are emitted and can be consumed

**Steps:**
1. Start event stream in background:
   ```bash
   drover stream > /tmp/events.jsonl &
   STREAM_PID=$!
   ```

2. Execute a single task:
   ```bash
   drover run --workers 1 --epic E1
   ```

3. Stop stream and verify events:
   ```bash
   kill $STREAM_PID
   cat /tmp/events.jsonl | jq -c '.type' | sort -u
   # Expected: ["task.started", "task.completed"]
   ```

**Success Criteria:**
- [ ] Events emitted for task.started
- [ ] Events emitted for task.completed
- [ ] Events include task_id, title, worker fields
- [ ] Events are valid JSONL

### Test E1.2: Event Filtering

**Objective:** Verify filtering options work correctly

**Steps:**
```bash
# Filter by epic
drover stream --epic E1 | jq '.epic_id' | sort -u
# Expected: Only E1 tasks

# Filter by state
drover stream --state completed | jq '.type' | sort -u
# Expected: Only "task.completed" events

# Filter by worker
drover stream --worker worker-1 | jq '.worker' | sort -u
# Expected: Only "worker-1"
```

**Success Criteria:**
- [ ] Epic filter works
- [ ] State filter works
- [ ] Worker filter works
- [ ] Multiple filters can be combined

### Test E1.3: Historical Event Replay

**Objective:** Verify historical events can be replayed

**Steps:**
```bash
# Run some tasks
drover run --workers 2

# Wait for completion
sleep 10

# Replay events from last hour
drover stream --since $(date -u -d '1 hour ago' +%Y-%m-%dT%H:%M:%SZ) | wc -l
# Expected: Count > 0
```

**Success Criteria:**
- [ ] Historical events retrieved from database
- [ ] Timestamps are correct
- [ ] Events are in chronological order

### Test E1.4: Event Bus Integration with DBOS

**Objective:** Verify events are emitted at DBOS step boundaries

**Steps:**
1. Enable verbose logging:
   ```bash
   export DROVER_VERBOSE=true
   ```

2. Run task and capture events:
   ```bash
   drover stream > /tmp/dbos-events.jsonl &
   drover run --workers 1
   ```

3. Verify event timing matches DBOS steps:
   ```bash
   # Check that events align with DBOS checkpoint logs
   grep "DBOS step" drover.log | wc -l
   grep "task.started" /tmp/dbos-events.jsonl | wc -l
   # Should be similar counts
   ```

**Success Criteria:**
- [ ] Events emitted at DBOS step boundaries
- [ ] No events lost during crash recovery
- [ ] Events persist across restarts

---

## Epic 2: Project-Level Configuration (E2)

### Test E2.1: Config Loading Hierarchy

**Objective:** Verify config precedence (CLI > Env > Project > Global > Defaults)

**Steps:**
```bash
# 1. Create project config
cat > .drover.toml <<EOF
max_workers = 8
agent = "claude"
EOF

# 2. Set environment variable (should override project)
export DROVER_MAX_WORKERS=4

# 3. Run with CLI flag (should override env)
drover run --workers 2

# 4. Verify actual workers used
# Check logs or dashboard for worker count
```

**Success Criteria:**
- [ ] CLI flags take precedence
- [ ] Environment variables override project config
- [ ] Project config overrides global config
- [ ] Defaults used when nothing specified

### Test E2.2: Guidelines Injection

**Objective:** Verify project guidelines appear in agent prompts

**Steps:**
```bash
# 1. Create config with guidelines
cat > .drover.toml <<EOF
guidelines = """
This is a Go project.
- Use structured logging
- Follow Go idioms
- All public functions need doc comments
"""
EOF

# 2. Create a task
drover add "Test task" --epic E2

# 3. Run with verbose logging
drover run --workers 1 --verbose 2>&1 | grep -i "guidelines"
# Expected: Guidelines appear in logs
```

**Success Criteria:**
- [ ] Guidelines appear in task context
- [ ] Guidelines passed to agent
- [ ] Template variables expanded (if used)

### Test E2.3: Config Validation

**Objective:** Verify invalid configs are rejected

**Steps:**
```bash
# Test 1: Negative workers
cat > .drover.toml <<EOF
max_workers = -1
EOF
drover run  # Should fail

# Test 2: Invalid agent type
cat > .drover.toml <<EOF
agent = "invalid-agent"
EOF
drover run  # Should fail

# Test 3: Invalid duration
cat > .drover.toml <<EOF
task_timeout = "not-a-duration"
EOF
drover run  # Should fail
```

**Success Criteria:**
- [ ] Invalid configs rejected with clear errors
- [ ] Validation errors mention specific fields
- [ ] Valid configs accepted

---

## Epic 3: Context Window Management (E3)

### Test E3.1: Size Detection

**Objective:** Verify large content is detected

**Steps:**
```bash
# Create task with large description (>250KB)
LARGE_DESC=$(python3 -c "print('x' * 300000)")
drover add "Large task" --epic E3 -d "$LARGE_DESC"

# Run task
drover run --workers 1 --epic E3 --verbose 2>&1 | grep -i "size\|threshold\|reference"
# Expected: Size detection messages
```

**Success Criteria:**
- [ ] Large content detected
- [ ] Thresholds configurable
- [ ] Size calculation accurate

### Test E3.2: Reference Substitution

**Objective:** Verify large content replaced with references

**Steps:**
```bash
# Create task with large attached file
echo "$(python3 -c "print('x' * 300000)")" > large-file.txt
drover add "Task with large file" --epic E3 --attach large-file.txt

# Run and check prompt
drover run --workers 1 --epic E3 --verbose 2>&1 | grep -i "reference\|fetch"
# Expected: References instead of content
```

**Success Criteria:**
- [ ] Large content replaced with references
- [ ] References include fetch commands
- [ ] Agent can fetch referenced content

---

## Epic 4: Structured Task Outcomes (E4)

### Test E4.1: Verdict Extraction

**Objective:** Verify verdicts extracted from agent output

**Steps:**
```bash
# Create task with explicit success criteria
drover add "Test task" --epic E4 -d "
Task description with criteria:
- [ ] Feature works
- [ ] Tests pass
- [ ] Documentation updated
"

# Run task
drover run --workers 1 --epic E4

# Check verdict
drover status | grep "Test task"
# Expected: Verdict displayed (pass/fail/blocked)
```

**Success Criteria:**
- [ ] Verdict extracted correctly
- [ ] Pass verdict for successful tasks
- [ ] Fail verdict for failed tasks
- [ ] Blocked verdict for blocked tasks

### Test E4.2: Verdict in TUI

**Objective:** Verify verdicts displayed in TUI

**Steps:**
```bash
# Run multiple tasks with different outcomes
drover run --workers 4

# Open TUI
drover status
# Expected: Verdict column with color coding
```

**Success Criteria:**
- [ ] Verdict column visible
- [ ] Color coding: green (pass), red (fail), yellow (blocked)
- [ ] Verdict updates in real-time

---

## Epic 5: Enhanced CLI Job Controls (E5)

### Test E5.1: Cancel Running Task

**Objective:** Verify task cancellation works

**Steps:**
```bash
# Start long-running task
drover run --workers 1 &
RUN_PID=$!

# Wait for task to start
sleep 5

# Get running task ID
TASK_ID=$(drover status --oneline | grep running | head -1 | awk '{print $1}')

# Cancel task
drover cancel $TASK_ID

# Verify cancellation
drover status | grep $TASK_ID
# Expected: Status = "cancelled"

# Verify workflow doesn't resume
kill $RUN_PID
drover resume
drover status | grep $TASK_ID
# Expected: Still cancelled, not resumed
```

**Success Criteria:**
- [ ] Task cancelled successfully
- [ ] State updated to "cancelled"
- [ ] Worktree cleaned up (if applicable)
- [ ] DBOS workflow doesn't resume cancelled task

### Test E5.2: Retry Failed Task

**Objective:** Verify task retry works

**Steps:**
```bash
# Create and run task that will fail
drover add "Failing task" --epic E5 -d "This task will fail"
drover run --workers 1 --epic E5

# Verify failure
drover status | grep "Failing task"
# Expected: Status = "failed"

# Retry task
drover retry <task-id>

# Verify retry
drover status | grep "Failing task"
# Expected: Status = "ready" or "claimed", attempts incremented
```

**Success Criteria:**
- [ ] Task retried successfully
- [ ] Attempt count incremented
- [ ] Previous attempt logs preserved
- [ ] Task re-enters queue

### Test E5.3: Resolve Blocked Task

**Objective:** Verify manual resolution works

**Steps:**
```bash
# Create blocked task
drover add "Blocked task" --epic E5 --blocked-by nonexistent-task

# Verify blocked
drover status | grep "Blocked task"
# Expected: Status = "blocked"

# Resolve manually
drover resolve <task-id> --note "Fixed manually in separate PR"

# Verify resolution
drover status | grep "Blocked task"
# Expected: Status = "ready"
```

**Success Criteria:**
- [ ] Task resolved successfully
- [ ] Resolution note stored
- [ ] Task becomes ready
- [ ] Dependent tasks unblocked

---

## Epic 6: Task Context Carrying (E6)

### Test E6.1: Context Injection

**Objective:** Verify recent task context included in prompts

**Steps:**
```bash
# Complete several tasks
drover run --workers 2 --epic E6

# Create new task
drover add "New task" --epic E6

# Run with verbose logging
drover run --workers 1 --epic E6 --verbose 2>&1 | grep -i "recent\|context"
# Expected: Recent task summaries in logs
```

**Success Criteria:**
- [ ] Recent tasks included in context
- [ ] Context count configurable
- [ ] Context formatted correctly
- [ ] Context doesn't exceed size limits

---

## Epic: Worktree Pre-warming (Epic 1)

### Test W1.1: Pool Initialization

**Objective:** Verify worktree pool created on startup

**Steps:**
```bash
# Start with pool size 4
drover run --pool-size 4 --workers 2 &

# Wait for pool initialization
sleep 5

# Check pool status
ls -la .drover/worktrees/pool-*
# Expected: 4 worktree directories

# Verify tasks start quickly
time drover run --workers 1
# Expected: First task starts in <3s (vs 15-60s without pool)
```

**Success Criteria:**
- [ ] Pool initialized before first task
- [ ] Pool size matches config
- [ ] Tasks start faster with pool
- [ ] Pool state tracked in database

### Test W1.2: Dependency Cache Sharing

**Objective:** Verify shared dependency caches work

**Steps:**
```bash
# Create Node.js project
echo '{"name":"test","dependencies":{"lodash":"^4.17.21"}}' > package.json

# Run multiple tasks requiring npm install
drover run --pool-size 4 --workers 4

# Check disk usage
du -sh .drover/worktrees/pool-*/node_modules
# Expected: Symlinks, not full copies (80%+ disk savings)
```

**Success Criteria:**
- [ ] node_modules symlinked
- [ ] GOMODCACHE shared
- [ ] Disk usage reduced significantly
- [ ] Cache invalidation works on lock file changes

---

## Epic: Enhanced Observability (Epic 2)

### Test O2.1: Success Rate Metrics

**Objective:** Verify completion rate tracking

**Steps:**
```bash
# Run multiple tasks
drover run --workers 4

# Check dashboard metrics
curl http://localhost:8080/api/metrics | jq '.success_rate'
# Expected: Percentage of completed tasks
```

**Success Criteria:**
- [ ] Success rate calculated correctly
- [ ] Metrics grouped by epic/label
- [ ] Historical trends visible
- [ ] Metrics update in real-time

### Test O2.2: Worker Utilization

**Objective:** Verify worker efficiency metrics

**Steps:**
```bash
# Run with 4 workers
drover run --workers 4

# Check utilization
curl http://localhost:8080/api/metrics | jq '.worker_utilization'
# Expected: Percentage of workers actively processing
```

**Success Criteria:**
- [ ] Utilization calculated correctly
- [ ] Idle time tracked
- [ ] Throughput metrics available
- [ ] Dashboard shows utilization chart

---

## Integration Tests

### Test INT.1: DBOS Recovery

**Objective:** Verify workflows recover after crash

**Steps:**
```bash
# Start execution
drover run --workers 4 &

# Kill process mid-execution
sleep 10
kill -9 $!

# Resume execution
drover resume

# Verify tasks completed
drover status
# Expected: All tasks completed, no duplicates
```

**Success Criteria:**
- [ ] Workflows resume from checkpoint
- [ ] No duplicate execution
- [ ] State consistent after recovery
- [ ] Events emitted correctly after recovery

### Test INT.2: Beads Sync

**Objective:** Verify bidirectional sync with Beads

**Steps:**
```bash
# Create task in Drover
drover add "Synced task" --epic E1

# Verify in Beads format
cat .beads/beads.jsonl | jq 'select(.id == "task-xxx")'
# Expected: Task appears in beads.jsonl

# Modify in Beads (if bd CLI available)
bd close task-xxx --reason completed

# Import to Drover
drover sync --from-beads

# Verify status updated
drover status | grep "Synced task"
# Expected: Status = "completed"
```

**Success Criteria:**
- [ ] Tasks sync to Beads format
- [ ] Beads changes sync back to Drover
- [ ] Hierarchical IDs preserved
- [ ] Status mappings correct

### Test INT.3: Multi-Agent Support

**Objective:** Verify pluggable agent interface works

**Steps:**
```bash
# Test Claude
export DROVER_AGENT_TYPE=claude
drover run --workers 1

# Test Codex
export DROVER_AGENT_TYPE=codex
drover run --workers 1

# Test Amp
export DROVER_AGENT_TYPE=amp
drover run --workers 1

# Test OpenCode
export DROVER_AGENT_TYPE=opencode
drover run --workers 1
```

**Success Criteria:**
- [ ] All agent types work
- [ ] Agent selection via config
- [ ] Agent-specific features work
- [ ] Error handling consistent

---

## Performance Tests

### Test PERF.1: Parallel Execution Throughput

**Objective:** Measure tasks completed per hour

**Steps:**
```bash
# Create 100 tasks
for i in {1..100}; do
  drover add "Task $i" --epic E1
done

# Run with 8 workers
time drover run --workers 8

# Calculate throughput
# Expected: >50 tasks/hour with 8 workers
```

**Success Criteria:**
- [ ] Throughput scales with workers
- [ ] No significant overhead from DBOS
- [ ] Worktree pool improves throughput
- [ ] Metrics show performance data

### Test PERF.2: Memory Usage

**Objective:** Verify memory usage is reasonable

**Steps:**
```bash
# Monitor memory during execution
/usr/bin/time -v drover run --workers 4 2>&1 | grep "Maximum resident"
# Expected: <2GB for 4 workers
```

**Success Criteria:**
- [ ] Memory usage reasonable
- [ ] No memory leaks
- [ ] Memory scales linearly with workers
- [ ] Worktree pool doesn't increase memory significantly

---

## Test Execution Checklist

### Pre-Test Setup
- [ ] Fresh Drover initialization
- [ ] Test repository created
- [ ] Agent available and configured
- [ ] Test tasks loaded
- [ ] Database clean

### Test Execution
- [ ] Run all E1 tests (Event Streaming)
- [ ] Run all E2 tests (Project Config)
- [ ] Run all E3 tests (Context Window)
- [ ] Run all E4 tests (Structured Outcomes)
- [ ] Run all E5 tests (CLI Controls)
- [ ] Run all E6 tests (Context Carrying)
- [ ] Run all Epic 1 tests (Worktree Pool)
- [ ] Run all Epic 2 tests (Observability)
- [ ] Run integration tests
- [ ] Run performance tests

### Post-Test Cleanup
- [ ] Clean up test worktrees
- [ ] Reset database
- [ ] Remove test files
- [ ] Document test results

---

## Test Results Template

```markdown
## Test Results - [Date]

### Environment
- Drover Version: [version]
- Go Version: [version]
- Agent Type: [claude/codex/amp/opencode]
- Workers: [number]
- DBOS Mode: [SQLite/PostgreSQL]

### Test Summary
- Total Tests: [number]
- Passed: [number]
- Failed: [number]
- Skipped: [number]

### Failed Tests
1. [Test Name] - [Reason]
2. [Test Name] - [Reason]

### Performance Metrics
- Average Task Duration: [time]
- Throughput: [tasks/hour]
- Memory Usage: [MB]
- CPU Usage: [%]

### Issues Found
1. [Issue description]
2. [Issue description]

### Recommendations
1. [Recommendation]
2. [Recommendation]
```

---

*Test plan created: 2026-01-14*  
*Update as features are implemented*
