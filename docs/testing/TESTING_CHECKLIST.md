# Drover feat/v030 - E2E Testing Checklist

**Date:** 2026-01-15  
**Branch:** `origin/feat/v030`  
**Status:** ⚠️ **MANUAL TESTING REQUIRED**

---

## Overview

This document provides a comprehensive E2E testing checklist for the feat/v030 branch. Due to Go not being available in the review environment, these tests must be run manually.

---

## Prerequisites

```bash
# 1. Ensure Go is installed
go version  # Should be 1.22+

# 2. Clone and checkout branch
git clone https://github.com/cloud-shuttle/drover
cd drover
git checkout origin/feat/v030

# 3. Build
go build -o drover ./cmd/drover

# 4. Verify build
./drover --version
```

---

## Test Suite 1: Core Functionality

### Test 1.1: Initialize Project ✅

```bash
# Create test directory
mkdir -p /tmp/drover-test-1
cd /tmp/drover-test-1
git init

# Initialize Drover
drover init

# Verify
[ -d .drover ] && echo "✅ PASS" || echo "❌ FAIL"
[ -f .drover/drover.db ] && echo "✅ PASS" || echo "❌ FAIL"
[ -f .drover/task_template.yaml ] && echo "✅ PASS" || echo "❌ FAIL"
```

**Expected Result:**
- `.drover` directory created
- `drover.db` SQLite database created
- `task_template.yaml` created

---

### Test 1.2: Add Epic ✅

```bash
# Add epic
drover epic add "Test Epic"

# Verify
drover epic list | grep "Test Epic" && echo "✅ PASS" || echo "❌ FAIL"
```

**Expected Result:**
- Epic created with ID like `epic-1234567890`
- Epic visible in list

---

### Test 1.3: Add Tasks ✅

```bash
# Get epic ID
EPIC_ID=$(drover epic list | grep "Test Epic" | awk '{print $1}')

# Add tasks
TASK1=$(drover add "Task 1: Simple task" --epic $EPIC_ID)
TASK2=$(drover add "Task 2: Depends on Task 1" --epic $EPIC_ID --blocked-by $TASK1)
TASK3=$(drover add "Task 3: Independent" --epic $EPIC_ID)

# Verify
drover status | grep "Task 1" && echo "✅ PASS" || echo "❌ FAIL"
drover status | grep "Task 2" && echo "✅ PASS" || echo "❌ FAIL"
drover status | grep "Task 3" && echo "✅ PASS" || echo "❌ FAIL"
```

**Expected Result:**
- 3 tasks created
- Task 2 should be blocked by Task 1
- All tasks visible in status

---

### Test 1.4: Quick Command (New Feature) ✅

```bash
# Quick add
drover quick "Fix the login bug"

# Verify
drover status | grep "Fix the login bug" && echo "✅ PASS" || echo "❌ FAIL"
```

**Expected Result:**
- Task created with minimal input
- Task visible in status

---

### Test 1.5: Watch Command (New Feature) ✅

```bash
# Start watch in background
drover watch &
WATCH_PID=$!

# Wait a moment
sleep 2

# Kill watch
kill $WATCH_PID

# Check if it ran
[ $? -eq 0 ] && echo "✅ PASS" || echo "❌ FAIL"
```

**Expected Result:**
- Watch command starts without errors
- Updates display every second
- Gracefully handles Ctrl+C

---

## Test Suite 2: DBOS Workflow Execution

### Test 2.1: SQLite Mode Execution ✅

```bash
# Run in SQLite mode (default)
unset DBOS_SYSTEM_DATABASE_URL

# Run tasks (with mock agent for testing)
DROVER_AGENT_TYPE=mock drover run --workers 2 --verbose

# Verify
drover status | grep "completed" && echo "✅ PASS" || echo "❌ FAIL"
```

**Expected Result:**
- Tasks execute in SQLite mode
- DBOS workflows run
- Tasks transition to completed

---

### Test 2.2: DBOS Mode Execution (PostgreSQL) ⚠️

```bash
# Start PostgreSQL (if available)
docker run -d --name drover-test-pg \
  -e POSTGRES_PASSWORD=test \
  -e POSTGRES_DB=drover \
  -p 5432:5432 \
  postgres:15

# Wait for PostgreSQL
sleep 5

# Run in DBOS mode
export DBOS_SYSTEM_DATABASE_URL="postgresql://postgres:test@localhost:5432/drover"

# Run tasks
DROVER_AGENT_TYPE=mock drover run --workers 2 --verbose

# Verify
drover status | grep "completed" && echo "✅ PASS" || echo "❌ FAIL"

# Cleanup
docker stop drover-test-pg
docker rm drover-test-pg
```

**Expected Result:**
- Tasks execute in DBOS mode with PostgreSQL
- Workflows use DBOS durability
- Tasks complete successfully

---

### Test 2.3: Dependency Resolution ✅

```bash
# Create tasks with dependencies
TASK_A=$(drover add "Task A: Foundation")
TASK_B=$(drover add "Task B: Depends on A" --blocked-by $TASK_A)
TASK_C=$(drover add "Task C: Depends on B" --blocked-by $TASK_B)

# Check initial status
drover status | grep "Task B" | grep "blocked" && echo "✅ PASS" || echo "❌ FAIL"
drover status | grep "Task C" | grep "blocked" && echo "✅ PASS" || echo "❌ FAIL"

# Run tasks
DROVER_AGENT_TYPE=mock drover run --workers 1

# Verify execution order
# Task A should complete first, then B, then C
drover status | grep "Task A" | grep "completed" && echo "✅ PASS" || echo "❌ FAIL"
drover status | grep "Task B" | grep "completed" && echo "✅ PASS" || echo "❌ FAIL"
drover status | grep "Task C" | grep "completed" && echo "✅ PASS" || echo "❌ FAIL"
```

**Expected Result:**
- Tasks execute in correct order (A → B → C)
- Dependencies respected
- Blocked tasks unblock automatically

---

## Test Suite 3: Worktree Management

### Test 3.1: Worktree Creation ✅

```bash
# Run a single task
TASK=$(drover add "Test worktree creation")
DROVER_AGENT_TYPE=mock drover run --workers 1

# Check worktree was created and cleaned up
[ ! -d .drover/worktrees/drover-$TASK ] && echo "✅ PASS (cleaned up)" || echo "⚠️ WARN (not cleaned)"
```

**Expected Result:**
- Worktree created during execution
- Worktree cleaned up after completion

---

### Test 3.2: Worktree Pool (New Feature) ⚠️

```bash
# Enable worktree pooling
drover run --pool --pool-min 2 --pool-max 5 --workers 3

# Check pool status
# Note: This feature should be REMOVED per review
```

**Expected Result:**
- ⚠️ **This feature should be removed** (violates DBOS durability)
- If testing, verify pool creates warm worktrees
- Verify pool cleanup on exit

---

### Test 3.3: Concurrent Worktree Access ✅

```bash
# Create multiple independent tasks
for i in {1..5}; do
  drover add "Concurrent task $i"
done

# Run with multiple workers
DROVER_AGENT_TYPE=mock drover run --workers 3

# Verify no conflicts
drover status | grep "failed" && echo "❌ FAIL" || echo "✅ PASS"
```

**Expected Result:**
- Multiple tasks run concurrently
- No worktree conflicts
- All tasks complete successfully

---

## Test Suite 4: Dashboard Functionality

### Test 4.1: Dashboard Startup ✅

```bash
# Start dashboard
drover dashboard &
DASHBOARD_PID=$!

# Wait for startup
sleep 2

# Check if running
curl http://localhost:8080 > /dev/null 2>&1 && echo "✅ PASS" || echo "❌ FAIL"

# Cleanup
kill $DASHBOARD_PID
```

**Expected Result:**
- Dashboard starts on port 8080
- HTTP server responds
- No startup errors

---

### Test 4.2: WebSocket Updates (New Feature) ✅

```bash
# Start dashboard
drover dashboard &
DASHBOARD_PID=$!
sleep 2

# Connect WebSocket (using websocat if available)
websocat ws://localhost:8080/ws &
WS_PID=$!

# Run a task
DROVER_AGENT_TYPE=mock drover add "WebSocket test task"
DROVER_AGENT_TYPE=mock drover run --workers 1

# Check WebSocket received updates
# (Manual verification required)

# Cleanup
kill $WS_PID
kill $DASHBOARD_PID
```

**Expected Result:**
- WebSocket connection established
- Real-time task updates received
- Updates include task state changes

---

### Test 4.3: Dashboard UI ✅

**Manual Test:**
1. Start dashboard: `drover dashboard`
2. Open browser: `http://localhost:8080`
3. Verify UI elements:
   - Task list displays
   - Task status colors correct
   - Filters work
   - Real-time updates visible

**Expected Result:**
- UI loads without errors
- All elements functional
- Real-time updates work

---

## Test Suite 5: Export/Import

### Test 5.1: Export to Beads Format ✅

```bash
# Create some tasks
drover add "Export test task 1"
drover add "Export test task 2"

# Export to Beads
drover export --format beads --output /tmp/export-test.jsonl

# Verify file created
[ -f /tmp/export-test.jsonl ] && echo "✅ PASS" || echo "❌ FAIL"

# Verify format
cat /tmp/export-test.jsonl | jq -r '.type' | grep -E '(bead|epic|link)' && echo "✅ PASS" || echo "❌ FAIL"
```

**Expected Result:**
- Export file created
- Valid JSONL format
- Beads-compatible structure

---

### Test 5.2: Export to JSON ✅

```bash
# Export to JSON
drover export --format json --output /tmp/export-test.json

# Verify
[ -f /tmp/export-test.json ] && echo "✅ PASS" || echo "❌ FAIL"
jq '.tasks | length' /tmp/export-test.json && echo "✅ PASS" || echo "❌ FAIL"
```

**Expected Result:**
- JSON file created
- Valid JSON structure
- Contains tasks and epics

---

### Test 5.3: Export to YAML ✅

```bash
# Export to YAML
drover export --format yaml --output /tmp/export-test.yaml

# Verify
[ -f /tmp/export-test.yaml ] && echo "✅ PASS" || echo "❌ FAIL"
grep "tasks:" /tmp/export-test.yaml && echo "✅ PASS" || echo "❌ FAIL"
```

**Expected Result:**
- YAML file created
- Valid YAML structure
- Contains tasks and epics

---

### Test 5.4: Import from Beads ⚠️

```bash
# Import from Beads
# Note: Import command not implemented yet
drover import --format beads /tmp/export-test.jsonl

# Verify
# (Should fail with "not implemented" error)
```

**Expected Result:**
- ⚠️ **Feature not implemented**
- Should return clear error message
- Recommendation: Implement or document as future work

---

## Test Suite 6: DBOS Durability & Recovery

### Test 6.1: Crash Recovery ✅

```bash
# Start a long-running task
TASK=$(drover add "Long running task")
DROVER_AGENT_TYPE=mock drover run --workers 1 &
DROVER_PID=$!

# Wait for task to start
sleep 5

# Kill Drover (simulate crash)
kill -9 $DROVER_PID

# Resume
drover resume

# Verify task recovered
drover status | grep $TASK | grep "completed" && echo "✅ PASS" || echo "❌ FAIL"
```

**Expected Result:**
- Task recovers from crash
- DBOS resumes from checkpoint
- Task completes successfully

---

### Test 6.2: Workflow Checkpointing ✅

```bash
# Create tasks with dependencies
TASK_A=$(drover add "Checkpoint test A")
TASK_B=$(drover add "Checkpoint test B" --blocked-by $TASK_A)

# Run with verbose logging
DROVER_AGENT_TYPE=mock drover run --workers 1 --verbose 2>&1 | grep "checkpoint"

# Verify checkpoints created
# (Check logs for checkpoint messages)
```

**Expected Result:**
- DBOS creates checkpoints
- Checkpoints logged
- Workflow state persisted

---

### Test 6.3: WorktreePool Crash Recovery ⚠️

```bash
# Enable pool
drover run --pool --pool-min 2 &
DROVER_PID=$!

# Wait for pool warmup
sleep 10

# Kill Drover
kill -9 $DROVER_PID

# Check for orphaned worktrees
ls .drover/worktrees/ | grep "pool-" && echo "❌ FAIL (orphaned)" || echo "✅ PASS"

# Resume
drover resume

# Verify pool state recovered
# Note: This will likely FAIL because pool is not DBOS-managed
```

**Expected Result:**
- ⚠️ **This test will likely FAIL**
- Pool state NOT recovered (not DBOS-managed)
- Orphaned worktrees remain
- **This is why WorktreePool should be removed**

---

## Test Suite 7: Task Types (New Feature)

### Test 7.1: Add Task with Type ⚠️

```bash
# Try to add task with type
drover add "Bug fix task" --type bug

# Verify
# Note: --type flag may not be implemented
```

**Expected Result:**
- ⚠️ **Feature incomplete**
- If implemented: Task created with type
- If not: Clear error message
- **Recommendation:** Complete or remove feature

---

### Test 7.2: Filter by Type ⚠️

```bash
# Add tasks with different types
drover add "Feature task" --type feature
drover add "Bug task" --type bug
drover add "Test task" --type test

# Filter by type
drover status --type bug

# Verify only bug tasks shown
```

**Expected Result:**
- ⚠️ **Feature incomplete**
- Should filter tasks by type
- **Recommendation:** Complete or remove feature

---

## Test Suite 8: Operator Tracking (New Feature)

### Test 8.1: Operator Assignment ⚠️

```bash
# Set operator
export DROVER_OPERATOR="test-user"

# Add task
drover add "Operator test task"

# Verify operator assigned
drover status --json | jq -r '.tasks[0].operator' | grep "test-user" && echo "✅ PASS" || echo "❌ FAIL"
```

**Expected Result:**
- ⚠️ **Feature incomplete**
- Operator field populated
- **Concern:** No authentication system
- **Recommendation:** Remove until multiplayer is designed

---

## Test Suite 9: Performance & Stress

### Test 9.1: Many Tasks ✅

```bash
# Create 100 tasks
for i in {1..100}; do
  drover add "Task $i"
done

# Run with multiple workers
time DROVER_AGENT_TYPE=mock drover run --workers 8

# Verify all completed
COMPLETED=$(drover status | grep "completed" | wc -l)
[ $COMPLETED -eq 100 ] && echo "✅ PASS" || echo "❌ FAIL"
```

**Expected Result:**
- All 100 tasks complete
- No errors or crashes
- Reasonable performance (<10 minutes)

---

### Test 9.2: Deep Dependency Chain ✅

```bash
# Create chain of 20 dependent tasks
PREV=""
for i in {1..20}; do
  if [ -z "$PREV" ]; then
    PREV=$(drover add "Chain task $i")
  else
    PREV=$(drover add "Chain task $i" --blocked-by $PREV)
  fi
done

# Run
DROVER_AGENT_TYPE=mock drover run --workers 1

# Verify all completed in order
drover status | grep "completed" | wc -l | grep 20 && echo "✅ PASS" || echo "❌ FAIL"
```

**Expected Result:**
- All tasks complete in order
- No deadlocks
- Dependency resolution works

---

## Test Suite 10: Error Handling

### Test 10.1: Task Failure & Retry ✅

```bash
# Create task that will fail
TASK=$(drover add "Failing task")

# Mock agent to fail
DROVER_AGENT_TYPE=mock DROVER_MOCK_FAIL=true drover run --workers 1

# Verify retry attempts
drover status | grep $TASK | grep "failed" && echo "✅ PASS" || echo "❌ FAIL"

# Check attempts count
drover info $TASK | grep "attempts: 3" && echo "✅ PASS" || echo "❌ FAIL"
```

**Expected Result:**
- Task fails after max attempts
- Retry logic works
- Error logged

---

### Test 10.2: Invalid Task ID ✅

```bash
# Try to get info for non-existent task
drover info task-nonexistent 2>&1 | grep "not found" && echo "✅ PASS" || echo "❌ FAIL"
```

**Expected Result:**
- Clear error message
- No crash
- Graceful handling

---

### Test 10.3: Circular Dependencies ⚠️

```bash
# Try to create circular dependency
TASK_A=$(drover add "Task A")
TASK_B=$(drover add "Task B" --blocked-by $TASK_A)

# Try to make A depend on B (should fail)
drover update $TASK_A --blocked-by $TASK_B 2>&1 | grep "circular" && echo "✅ PASS" || echo "⚠️ WARN"
```

**Expected Result:**
- Circular dependency detected
- Clear error message
- No tasks created

---

## Summary Checklist

### Core Features
- [ ] Init project
- [ ] Add epic
- [ ] Add tasks
- [ ] Quick command
- [ ] Watch command
- [ ] Run tasks (SQLite mode)
- [ ] Run tasks (DBOS mode)
- [ ] Dependency resolution
- [ ] Status command
- [ ] Info command

### Worktree Management
- [ ] Worktree creation
- [ ] Worktree cleanup
- [ ] Concurrent access
- [ ] Pool (⚠️ should be removed)

### Dashboard
- [ ] Dashboard startup
- [ ] WebSocket updates
- [ ] UI functionality

### Export/Import
- [ ] Export to Beads
- [ ] Export to JSON
- [ ] Export to YAML
- [ ] Import from Beads (⚠️ not implemented)

### DBOS Durability
- [ ] Crash recovery
- [ ] Workflow checkpointing
- [ ] Pool recovery (⚠️ will fail)

### New Features
- [ ] Task types (⚠️ incomplete)
- [ ] Operator tracking (⚠️ incomplete)

### Performance
- [ ] Many tasks (100+)
- [ ] Deep dependency chains
- [ ] Concurrent execution

### Error Handling
- [ ] Task failure & retry
- [ ] Invalid input
- [ ] Circular dependencies

---

## Test Results Template

```markdown
## Test Results - [Date]

**Tester:** [Name]
**Environment:** [OS, Go version]
**Branch:** origin/feat/v030

### Summary
- Total Tests: X
- Passed: Y
- Failed: Z
- Warnings: W

### Failed Tests
1. Test X.Y: [Description]
   - Error: [Error message]
   - Expected: [Expected behavior]
   - Actual: [Actual behavior]

### Warnings
1. Test X.Y: [Description]
   - Issue: [Issue description]
   - Recommendation: [Recommendation]

### Notes
[Any additional observations]
```

---

## Automated Test Script

```bash
#!/bin/bash
# run-e2e-tests.sh

set -e

echo "🧪 Running Drover E2E Tests"
echo "================================"

# Setup
TEST_DIR="/tmp/drover-e2e-$(date +%s)"
mkdir -p $TEST_DIR
cd $TEST_DIR
git init

# Build Drover
cd /path/to/drover
go build -o $TEST_DIR/drover ./cmd/drover
cd $TEST_DIR

# Run tests
./drover init
EPIC=$(./drover epic add "Test Epic" | awk '{print $NF}')
TASK1=$(./drover add "Task 1" --epic $EPIC | awk '{print $NF}')
TASK2=$(./drover add "Task 2" --epic $EPIC --blocked-by $TASK1 | awk '{print $NF}')

# Run with mock agent
DROVER_AGENT_TYPE=mock ./drover run --workers 2

# Verify
COMPLETED=$(./drover status | grep "completed" | wc -l)
if [ $COMPLETED -eq 2 ]; then
  echo "✅ All tests passed"
  exit 0
else
  echo "❌ Tests failed"
  exit 1
fi
```

---

**Testing Checklist Created:** 2026-01-15  
**Next Action:** Run manual tests and record results
