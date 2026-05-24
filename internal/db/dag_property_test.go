// Package db_test contains property-based tests for the task DAG scheduler.
// This file contains the seven property tests that verify scheduling invariants.
//
// Properties tested:
//
//	P1  TestProp_NoBlockedTaskClaimable      — ClaimTask never returns a still-blocked task
//	P2  TestProp_TopologicalOrderRespected   — tasks are claimed only after all their deps complete
//	P3  TestProp_CompletionCascadeIsExact    — CompleteTask unblocks exactly the right dependents
//	P4  TestProp_PriorityOrdering            — highest-priority ready task is always claimed first
//	P5  TestProp_IdempotentCompletion        — completing an already-completed task is a safe no-op
//	P6  TestProp_FullDAGDrainCompletesAll    — draining a DAG via claim→complete reaches all-completed
//	P7  TestProp_ConcurrentClaimNoDoubleAssign — concurrent workers never double-claim the same task
package db_test

import (
	"fmt"
	"sync"
	"testing"

	"pgregory.net/rapid"

	"github.com/cloud-shuttle/drover/pkg/types"
)

const (
	propMaxNodes = 20 // max tasks in a generated DAG
	// Trial count is controlled via the -rapid.checks flag (default 100)
	// or the RAPID_CHECKS environment variable.
)

// ---------------------------------------------------------------------------
// P1 — No blocked task is ever claimable
// ---------------------------------------------------------------------------

// TestProp_NoBlockedTaskClaimable asserts that ClaimTask never returns a task
// that still has at least one non-completed dependency in the database.
//
// This is the primary safety invariant of the scheduler: a task whose blockers
// are not yet done must never be handed to a worker.
func TestProp_NoBlockedTaskClaimable(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		store := setupPropDB(t)
		defer store.Close()

		spec := genDAG(t, propMaxNodes)
		ids := insertDAGSpec(t, store, spec)

		// Build a quick lookup: real taskID → blocker real taskIDs
		blockerMap := make(map[string][]string) // id → blocker ids
		for i, ts := range spec.tasks {
			for _, bIdx := range ts.blockedByIndices {
				blockerMap[ids[i]] = append(blockerMap[ids[i]], ids[bIdx])
			}
		}

		completedIDs := make(map[string]bool)

		for {
			task, err := store.ClaimTask("worker-p1")
			if err != nil {
				t.Fatalf("ClaimTask: %v", err)
			}
			if task == nil {
				break
			}

			// P1 assertion: every declared blocker of this task must be completed.
			for _, blockerID := range blockerMap[task.ID] {
				status, err := store.GetTaskStatus(blockerID)
				if err != nil {
					t.Fatalf("GetTaskStatus(%s): %v", blockerID, err)
				}
				if status != types.TaskStatusCompleted {
					t.Fatalf(
						"VIOLATION P1: claimed task %q has non-completed blocker %q (status=%s)",
						task.ID, blockerID, status,
					)
				}
			}

			if err := store.CompleteTask(task.ID); err != nil {
				t.Fatalf("CompleteTask(%s): %v", task.ID, err)
			}
			completedIDs[task.ID] = true
		}
	})
}

// ---------------------------------------------------------------------------
// P2 — Topological order respected (model-based)
// ---------------------------------------------------------------------------

// TestProp_TopologicalOrderRespected uses the in-memory dagModel as an oracle.
//
// At every step, the model predicts which task should be claimed next.
// We compare the model's prediction against what the DB actually claims.
// If the DB claims a task the model says is still blocked, the invariant is broken.
func TestProp_TopologicalOrderRespected(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		store := setupPropDB(t)
		defer store.Close()

		spec := genDAG(t, propMaxNodes)
		ids := insertDAGSpec(t, store, spec)

		model := newDAGModel()
		for i, ts := range spec.tasks {
			var blockedByIDs []string
			for _, bIdx := range ts.blockedByIndices {
				blockedByIDs = append(blockedByIDs, ids[bIdx])
			}
			model.addTask(ids[i], ts.priority, blockedByIDs)
		}

		for {
			task, err := store.ClaimTask("worker-p2")
			if err != nil {
				t.Fatalf("ClaimTask: %v", err)
			}
			if task == nil {
				break
			}

			// P2 assertion: the model must agree that this task is now ready.
			modelStatus := model.taskStatus(task.ID)
			if modelStatus != types.TaskStatusReady {
				t.Fatalf(
					"VIOLATION P2: DB claimed %q but model says it is %s (not ready)",
					task.ID, modelStatus,
				)
			}

			// Advance both the model and the DB together.
			model.claim()
			model.complete(task.ID)
			if err := store.CompleteTask(task.ID); err != nil {
				t.Fatalf("CompleteTask(%s): %v", task.ID, err)
			}
		}

		// After draining, model must also be fully drained.
		if model.nextReady() != "" {
			t.Fatalf("VIOLATION P2: model still has ready tasks after DB returned nil")
		}
	})
}

// ---------------------------------------------------------------------------
// P3 — Completion cascade is exact
// ---------------------------------------------------------------------------

// TestProp_CompletionCascadeIsExact verifies that CompleteTask unblocks
// exactly the tasks whose last remaining blocker was just completed —
// no more, no fewer.
//
// Strategy:
//  1. Insert the DAG and drain until we have at least one blocked task.
//  2. Identify the next task to complete and ask the model which dependents
//     should become ready as a result.
//  3. Complete the task in the DB and compare DB state against model prediction.
func TestProp_CompletionCascadeIsExact(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		store := setupPropDB(t)
		defer store.Close()

		spec := genDAG(t, propMaxNodes)
		ids := insertDAGSpec(t, store, spec)

		model := newDAGModel()
		for i, ts := range spec.tasks {
			var blockedByIDs []string
			for _, bIdx := range ts.blockedByIndices {
				blockedByIDs = append(blockedByIDs, ids[bIdx])
			}
			model.addTask(ids[i], ts.priority, blockedByIDs)
		}

		for {
			task, err := store.ClaimTask("worker-p3")
			if err != nil {
				t.Fatalf("ClaimTask: %v", err)
			}
			if task == nil {
				break
			}
			model.claim()

			// Ask the model: which tasks will become ready after this completion?
			expectedUnblocked := model.complete(task.ID)

			// Now complete in the DB.
			if err := store.CompleteTask(task.ID); err != nil {
				t.Fatalf("CompleteTask(%s): %v", task.ID, err)
			}

			// P3 assertion: every task the model said should unblock must now
			// be ready in the DB, and no task the model did NOT predict should
			// have silently become ready.
			for unblockedID := range expectedUnblocked {
				status, err := store.GetTaskStatus(unblockedID)
				if err != nil {
					t.Fatalf("GetTaskStatus(%s): %v", unblockedID, err)
				}
				if status != types.TaskStatusReady {
					t.Fatalf(
						"VIOLATION P3 (missing unblock): completing %q should have set %q to ready, got %s",
						task.ID, unblockedID, status,
					)
				}
			}

			// Check that tasks the model keeps blocked are indeed still blocked.
			for _, blockedID := range model.blockedIDs() {
				status, err := store.GetTaskStatus(blockedID)
				if err != nil {
					t.Fatalf("GetTaskStatus(%s): %v", blockedID, err)
				}
				if status == types.TaskStatusReady {
					t.Fatalf(
						"VIOLATION P3 (spurious unblock): completing %q should NOT have set %q to ready",
						task.ID, blockedID,
					)
				}
			}
		}
	})
}

// ---------------------------------------------------------------------------
// P4 — Priority ordering
// ---------------------------------------------------------------------------

// TestProp_PriorityOrdering asserts that ClaimTask always picks the
// highest-priority ready task.
//
// Uses genDistinctPriorityDAG so that there are no same-priority ties,
// making the expected winner fully deterministic regardless of insertion order.
func TestProp_PriorityOrdering(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		store := setupPropDB(t)
		defer store.Close()

		spec := genDistinctPriorityDAG(t, propMaxNodes)
		ids := insertDAGSpec(t, store, spec)

		model := newDAGModel()
		for i, ts := range spec.tasks {
			var blockedByIDs []string
			for _, bIdx := range ts.blockedByIndices {
				blockedByIDs = append(blockedByIDs, ids[bIdx])
			}
			model.addTask(ids[i], ts.priority, blockedByIDs)
		}

		for {
			expectedID := model.nextReady()

			task, err := store.ClaimTask("worker-p4")
			if err != nil {
				t.Fatalf("ClaimTask: %v", err)
			}

			if task == nil && expectedID == "" {
				break // both agree: nothing left
			}
			if task == nil && expectedID != "" {
				t.Fatalf("VIOLATION P4: DB returned nil but model expects %q to be ready", expectedID)
			}
			if task != nil && expectedID == "" {
				t.Fatalf("VIOLATION P4: DB returned %q but model has no ready tasks", task.ID)
			}

			// P4 assertion: DB must have claimed the same task the model predicted.
			if task.ID != expectedID {
				modelTask := model.tasks[task.ID]
				expectedTask := model.tasks[expectedID]
				t.Fatalf(
					"VIOLATION P4: DB claimed %q (priority=%d) but highest-priority ready was %q (priority=%d)",
					task.ID, modelTask.priority,
					expectedID, expectedTask.priority,
				)
			}

			model.claim()
			model.complete(task.ID)
			if err := store.CompleteTask(task.ID); err != nil {
				t.Fatalf("CompleteTask(%s): %v", task.ID, err)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// P5 — Idempotent completion
// ---------------------------------------------------------------------------

// TestProp_IdempotentCompletion verifies that calling CompleteTask on an
// already-completed task does not corrupt state.
//
// After a full drain, we pick a random completed task and complete it again.
// The project status must remain all-completed.
func TestProp_IdempotentCompletion(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		store := setupPropDB(t)
		defer store.Close()

		spec := genDAG(t, propMaxNodes)
		ids := insertDAGSpec(t, store, spec)

		// Full drain
		drainDAG(t, store)

		// Pick any task and complete it again
		targetIdx := rapid.IntRange(0, len(ids)-1).Draw(t, "target_idx")
		targetID := ids[targetIdx]

		// P5: re-completing must not error.
		if err := store.CompleteTask(targetID); err != nil {
			t.Fatalf("VIOLATION P5: CompleteTask on already-completed task %q returned error: %v", targetID, err)
		}

		// P5: all tasks must still be completed (no corruption).
		status, err := store.GetProjectStatus()
		if err != nil {
			t.Fatalf("GetProjectStatus: %v", err)
		}
		if status.Ready != 0 || status.Blocked != 0 || status.Claimed != 0 || status.InProgress != 0 {
			t.Fatalf(
				"VIOLATION P5: after idempotent complete, project is not fully-completed: %+v",
				status,
			)
		}
		if status.Completed != len(ids) {
			t.Fatalf(
				"VIOLATION P5: expected %d completed tasks, got %d",
				len(ids), status.Completed,
			)
		}
	})
}

// ---------------------------------------------------------------------------
// P6 — Full DAG drain reaches all-completed
// ---------------------------------------------------------------------------

// TestProp_FullDAGDrainCompletesAll verifies that the claim→complete loop
// eventually reaches a state where every task in the DAG is completed and
// nothing is left blocked, ready, or claimed.
//
// This is the end-to-end liveness property: a valid DAG must always be
// fully drainable.
func TestProp_FullDAGDrainCompletesAll(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		store := setupPropDB(t)
		defer store.Close()

		spec := genDAG(t, propMaxNodes)
		ids := insertDAGSpec(t, store, spec)

		drainDAG(t, store)

		status, err := store.GetProjectStatus()
		if err != nil {
			t.Fatalf("GetProjectStatus: %v", err)
		}

		// P6: nothing left in any non-terminal state.
		if status.Ready != 0 {
			t.Fatalf("VIOLATION P6: %d tasks still ready after full drain", status.Ready)
		}
		if status.Blocked != 0 {
			t.Fatalf("VIOLATION P6: %d tasks still blocked after full drain", status.Blocked)
		}
		if status.Claimed != 0 {
			t.Fatalf("VIOLATION P6: %d tasks still claimed after full drain", status.Claimed)
		}
		if status.InProgress != 0 {
			t.Fatalf("VIOLATION P6: %d tasks still in_progress after full drain", status.InProgress)
		}
		if status.Completed != len(ids) {
			t.Fatalf(
				"VIOLATION P6: expected all %d tasks completed, got %d completed",
				len(ids), status.Completed,
			)
		}
	})
}

// ---------------------------------------------------------------------------
// P7 — Concurrent claim: no double assignment
// ---------------------------------------------------------------------------

// TestProp_ConcurrentClaimNoDoubleAssign verifies that under concurrent workers,
// every task is claimed at most once.
//
// N workers simultaneously loop calling ClaimTask. The union of all claimed
// task IDs must have no duplicates, and each claimed ID must belong to the DAG.
// Workers are capped at 3 to stay within SQLite's WAL-mode safe concurrency range.
func TestProp_ConcurrentClaimNoDoubleAssign(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		store := setupPropDB(t)
		defer store.Close()

		// Use tasks with NO dependencies so all are immediately claimable,
		// maximising the window for concurrent races.
		n := rapid.IntRange(1, propMaxNodes).Draw(t, "n")
		ids := make([]string, n)
		for i := 0; i < n; i++ {
			task, err := store.CreateTask(
				fmt.Sprintf("conc-task-%d", i), "", "", 10, nil,
			)
			if err != nil {
				t.Fatalf("CreateTask[%d]: %v", i, err)
			}
			ids[i] = task.ID
		}

		knownIDs := make(map[string]bool, n)
		for _, id := range ids {
			knownIDs[id] = true
		}

		numWorkers := genWorkerCount(t)
		if numWorkers > 3 {
			numWorkers = 3 // cap for SQLite WAL safety
		}

		var (
			mu          sync.Mutex
			claimedOnce = make(map[string]int) // taskID → claim count
		)

		var wg sync.WaitGroup
		for w := 0; w < numWorkers; w++ {
			workerID := fmt.Sprintf("worker-p7-%d", w)
			wg.Add(1)
			go func(wid string) {
				defer wg.Done()
				for {
					task, err := store.ClaimTask(wid)
					if err != nil {
						// SQLite may return "database is locked" under contention.
						// This is acceptable — the worker simply stops.
						return
					}
					if task == nil {
						return
					}
					mu.Lock()
					claimedOnce[task.ID]++
					mu.Unlock()
					// Complete immediately so dependents (if any) unblock.
					_ = store.CompleteTask(task.ID)
				}
			}(workerID)
		}
		wg.Wait()

		// P7: no task claimed more than once.
		for taskID, count := range claimedOnce {
			if count > 1 {
				t.Fatalf(
					"VIOLATION P7: task %q was claimed %d times (expected at most 1)",
					taskID, count,
				)
			}
		}

		// P7: no unknown task IDs appeared.
		for taskID := range claimedOnce {
			if !knownIDs[taskID] {
				t.Fatalf(
					"VIOLATION P7: claimed unknown task %q not in the generated DAG",
					taskID,
				)
			}
		}
	})
}
