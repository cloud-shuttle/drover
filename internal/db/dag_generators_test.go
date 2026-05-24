// Package db_test contains property-based tests for the task DAG scheduler.
// This file defines: rapid generators, an in-memory model (oracle), and test helpers.
package db_test

import (
	"fmt"
	"os"
	"path/filepath"

	"pgregory.net/rapid"

	"github.com/cloud-shuttle/drover/internal/db"
	"github.com/cloud-shuttle/drover/pkg/types"
)

// ---------------------------------------------------------------------------
// Generator types
// ---------------------------------------------------------------------------

// taskSpec describes one node in the generated task DAG.
type taskSpec struct {
	index            int    // position in the DAG slice (used for acyclicity guarantee)
	title            string
	priority         int
	blockedByIndices []int // indices into the same dagSpec.tasks slice; always < index
}

// dagSpec is a fully-generated, guaranteed-acyclic dependency graph.
type dagSpec struct {
	tasks []taskSpec
}

// ---------------------------------------------------------------------------
// Rapid generators
// ---------------------------------------------------------------------------

// genDAG generates a random valid acyclic task graph with 1–maxNodes nodes.
//
// Acyclicity is guaranteed structurally: task[i] may only depend on task[j]
// where j < i (index-ordered DAG). Each task gets 0–3 unique blockers chosen
// from earlier tasks.
func genDAG(t *rapid.T, maxNodes int) dagSpec {
	n := rapid.IntRange(1, maxNodes).Draw(t, "n")
	tasks := make([]taskSpec, n)

	for i := 0; i < n; i++ {
		tasks[i] = taskSpec{
			index:    i,
			title:    fmt.Sprintf("task-%d", i),
			priority: rapid.IntRange(0, 100).Draw(t, fmt.Sprintf("priority_%d", i)),
		}
		if i > 0 {
			// Pick 0–3 unique predecessors
			maxDeps := i
			if maxDeps > 3 {
				maxDeps = 3
			}
			numDeps := rapid.IntRange(0, maxDeps).Draw(t, fmt.Sprintf("numdeps_%d", i))
			seen := make(map[int]bool)
			for d := 0; d < numDeps; d++ {
				dep := rapid.IntRange(0, i-1).Draw(t, fmt.Sprintf("dep_%d_%d", i, d))
				if !seen[dep] {
					tasks[i].blockedByIndices = append(tasks[i].blockedByIndices, dep)
					seen[dep] = true
				}
			}
		}
	}
	return dagSpec{tasks: tasks}
}

// genDistinctPriorityDAG generates a DAG where every task has a unique priority.
// This makes the expected claim order fully deterministic (no FIFO tie-breaking
// between same-priority tasks needed).
func genDistinctPriorityDAG(t *rapid.T, maxNodes int) dagSpec {
	n := rapid.IntRange(1, maxNodes).Draw(t, "n")

	// Shuffle 0…n-1 as unique priorities
	perm := rapid.SliceOfN(rapid.IntRange(0, n-1), n, n).Draw(t, "priority_perm")
	// Deduplicate via map (rapid may repeat values in the slice)
	seen := make(map[int]bool)
	priorities := make([]int, 0, n)
	for _, p := range perm {
		if !seen[p] {
			seen[p] = true
			priorities = append(priorities, p)
		}
		if len(priorities) == n {
			break
		}
	}
	// Fill remaining if dedup reduced length
	for v := 0; len(priorities) < n; v++ {
		if !seen[v] {
			seen[v] = true
			priorities = append(priorities, v)
		}
	}

	tasks := make([]taskSpec, n)
	for i := 0; i < n; i++ {
		tasks[i] = taskSpec{
			index:    i,
			title:    fmt.Sprintf("task-%d", i),
			priority: priorities[i],
		}
		if i > 0 {
			maxDeps := i
			if maxDeps > 3 {
				maxDeps = 3
			}
			numDeps := rapid.IntRange(0, maxDeps).Draw(t, fmt.Sprintf("numdeps_%d", i))
			seenDeps := make(map[int]bool)
			for d := 0; d < numDeps; d++ {
				dep := rapid.IntRange(0, i-1).Draw(t, fmt.Sprintf("dep_%d_%d", i, d))
				if !seenDeps[dep] {
					tasks[i].blockedByIndices = append(tasks[i].blockedByIndices, dep)
					seenDeps[dep] = true
				}
			}
		}
	}
	return dagSpec{tasks: tasks}
}

// genWorkerCount generates a number of concurrent workers in [1, 4].
func genWorkerCount(t *rapid.T) int {
	return rapid.IntRange(1, 4).Draw(t, "workers")
}

// ---------------------------------------------------------------------------
// Test DB helper
// ---------------------------------------------------------------------------

// setupPropDB opens a fresh in-memory SQLite store for one property trial.
// It accepts *rapid.T so it can use rapid's per-trial Cleanup.
func setupPropDB(t *rapid.T) *db.Store {
	tmpDir, err := os.MkdirTemp("", "drover-prop-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	if err := store.InitSchema(); err != nil {
		store.Close()
		t.Fatalf("InitSchema: %v", err)
	}
	return store
}

// insertDAGSpec inserts a dagSpec into the store and returns the slice of
// real task IDs in the same order as spec.tasks.
//
// Accepts *rapid.T so it can use rapid.T.Fatalf inside rapid.Check callbacks.
func insertDAGSpec(t *rapid.T, store *db.Store, spec dagSpec) []string {
	ids := make([]string, len(spec.tasks))
	for i, ts := range spec.tasks {
		var blockedBy []string
		for _, bIdx := range ts.blockedByIndices {
			blockedBy = append(blockedBy, ids[bIdx])
		}
		task, err := store.CreateTask(ts.title, "", "", ts.priority, blockedBy)
		if err != nil {
			t.Fatalf("insertDAGSpec[%d]: CreateTask: %v", i, err)
		}
		ids[i] = task.ID
	}
	return ids
}

// ---------------------------------------------------------------------------
// In-memory DAG model (oracle)
// ---------------------------------------------------------------------------

// dagModel is a pure in-memory implementation of the scheduler state machine.
// It is used as the ground-truth oracle in model-based property tests.
type dagModel struct {
	tasks       map[string]*modelTask
	insertOrder []string // records insertion sequence for FIFO tie-breaking
}

type modelTask struct {
	id        string
	priority  int
	status    types.TaskStatus
	blockedBy map[string]bool // set of blocker IDs
	insertSeq int             // 0-based insertion position
}

func newDAGModel() *dagModel {
	return &dagModel{tasks: make(map[string]*modelTask)}
}

// addTask registers a task in the model, setting its initial status.
func (m *dagModel) addTask(id string, priority int, blockedByIDs []string) {
	mt := &modelTask{
		id:        id,
		priority:  priority,
		blockedBy: make(map[string]bool),
		insertSeq: len(m.insertOrder),
	}
	for _, bid := range blockedByIDs {
		mt.blockedBy[bid] = true
	}
	if len(mt.blockedBy) == 0 {
		mt.status = types.TaskStatusReady
	} else {
		mt.status = types.TaskStatusBlocked
	}
	m.tasks[id] = mt
	m.insertOrder = append(m.insertOrder, id)
}

// nextReady returns the task ID that ClaimTask should pick next:
// highest priority among ready tasks, FIFO (insertSeq) as tie-breaker.
// Returns "" if no ready tasks exist.
func (m *dagModel) nextReady() string {
	best := ""
	bestPri := -1
	bestSeq := int(^uint(0) >> 1) // max int
	for _, id := range m.insertOrder {
		mt := m.tasks[id]
		if mt.status != types.TaskStatusReady {
			continue
		}
		if mt.priority > bestPri || (mt.priority == bestPri && mt.insertSeq < bestSeq) {
			best = id
			bestPri = mt.priority
			bestSeq = mt.insertSeq
		}
	}
	return best
}

// claim transitions the next ready task to claimed and returns its ID.
func (m *dagModel) claim() string {
	id := m.nextReady()
	if id == "" {
		return ""
	}
	m.tasks[id].status = types.TaskStatusClaimed
	return id
}

// complete marks a task completed and cascades unblocking to dependents.
// Returns the set of task IDs that became ready as a result.
func (m *dagModel) complete(id string) map[string]bool {
	m.tasks[id].status = types.TaskStatusCompleted
	unblocked := make(map[string]bool)

	for _, mt := range m.tasks {
		if !mt.blockedBy[id] {
			continue
		}
		if mt.status != types.TaskStatusBlocked {
			continue
		}
		allDone := true
		for blockerID := range mt.blockedBy {
			if m.tasks[blockerID].status != types.TaskStatusCompleted {
				allDone = false
				break
			}
		}
		if allDone {
			mt.status = types.TaskStatusReady
			unblocked[mt.id] = true
		}
	}
	return unblocked
}

// blockedIDs returns all task IDs currently in the blocked state.
func (m *dagModel) blockedIDs() []string {
	var out []string
	for id, mt := range m.tasks {
		if mt.status == types.TaskStatusBlocked {
			out = append(out, id)
		}
	}
	return out
}

// readyIDs returns all task IDs currently in the ready state.
func (m *dagModel) readyIDs() []string {
	var out []string
	for id, mt := range m.tasks {
		if mt.status == types.TaskStatusReady {
			out = append(out, id)
		}
	}
	return out
}

// allCompleted returns true if every task has status completed.
func (m *dagModel) allCompleted() bool {
	for _, mt := range m.tasks {
		if mt.status != types.TaskStatusCompleted {
			return false
		}
	}
	return true
}

// taskStatus returns the model's current status for a task.
func (m *dagModel) taskStatus(id string) types.TaskStatus {
	return m.tasks[id].status
}

// ---------------------------------------------------------------------------
// Shared drain helper
// ---------------------------------------------------------------------------

// drainDAG runs a claim→complete loop until no more tasks can be claimed.
// It returns the sequence of task IDs that were claimed, in order.
func drainDAG(t *rapid.T, store *db.Store) []string {
	var claimed []string
	for {
		task, err := store.ClaimTask("worker-drain")
		if err != nil {
			t.Fatalf("drainDAG: ClaimTask: %v", err)
		}
		if task == nil {
			break
		}
		if err := store.CompleteTask(task.ID); err != nil {
			t.Fatalf("drainDAG: CompleteTask(%s): %v", task.ID, err)
		}
		claimed = append(claimed, task.ID)
	}
	return claimed
}
