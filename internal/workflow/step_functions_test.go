package workflow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cloud-shuttle/drover-libs/pkg/clock"
	"github.com/cloud-shuttle/drover/internal/config"
	"github.com/cloud-shuttle/drover/internal/db"
	"github.com/cloud-shuttle/drover/internal/executor"
	"github.com/cloud-shuttle/drover/pkg/types"
	"go.opentelemetry.io/otel/trace"
)

// MockAgent implements executor.Agent for testing
type MockAgent struct {
	Result      *executor.ExecutionResult
	ExecuteFunc func(ctx context.Context, worktreePath string, task *types.Task, parentSpan ...trace.Span) *executor.ExecutionResult
	Calls       int
}

func (m *MockAgent) ExecuteWithContext(ctx context.Context, worktreePath string, task *types.Task, parentSpan ...trace.Span) *executor.ExecutionResult {
	m.Calls++
	if m.ExecuteFunc != nil {
		return m.ExecuteFunc(ctx, worktreePath, task, parentSpan...)
	}
	return m.Result
}

func (m *MockAgent) CheckInstalled() error { return nil }
func (m *MockAgent) SetVerbose(bool)       {}

// ============================================================================
// createWorktreeStep
// ============================================================================

func TestCreateWorktreeStep_Success(t *testing.T) {
	gitMgr := NewSuccessGitManager("/tmp/worktree/task-1")

	o := &DBOSOrchestrator{
		clock: clock.RealClock{},
		git:   gitMgr,
	}

	task := TaskInput{TaskID: "task-1", Title: "Test Task"}
	path, err := o.createWorktreeStep(context.Background(), task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "/tmp/worktree/task-1" {
		t.Errorf("expected /tmp/worktree/task-1, got %s", path)
	}
	if len(gitMgr.CreateCalls) != 1 || gitMgr.CreateCalls[0] != "task-1" {
		t.Errorf("expected Create called with task-1, got %v", gitMgr.CreateCalls)
	}
}

func TestCreateWorktreeStep_GitError(t *testing.T) {
	gitMgr := NewFailingGitManager(errors.New("worktree conflict"))

	o := &DBOSOrchestrator{
		clock: clock.RealClock{},
		git:   gitMgr,
	}

	task := TaskInput{TaskID: "task-2", Title: "Failing Task"}
	_, err := o.createWorktreeStep(context.Background(), task)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "creating worktree: worktree conflict" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestCreateWorktreeStep_WithStore(t *testing.T) {
	tmp := t.TempDir()
	store, _ := db.Open(tmp + "/test.db")
	defer store.Close()
	store.InitSchema()

	gitMgr := NewSuccessGitManager("/tmp/worktree/task-3")

	o := &DBOSOrchestrator{
		clock: clock.RealClock{},
		git:   gitMgr,
		store: store,
	}

	task := TaskInput{TaskID: "task-3", Title: "Task With Store"}
	path, err := o.createWorktreeStep(context.Background(), task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "/tmp/worktree/task-3" {
		t.Errorf("expected /tmp/worktree/task-3, got %s", path)
	}
}

// ============================================================================
// commitChangesStep
// ============================================================================

func TestCommitChangesStep_Success(t *testing.T) {
	gitMgr := &MockGitManager{HasChanges: true}

	o := &DBOSOrchestrator{
		clock: clock.RealClock{},
		git:   gitMgr,
	}

	task := TaskInput{TaskID: "task-1", Title: "Test"}
	hasChanges, err := o.commitChangesStep(context.Background(), task, "output")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasChanges {
		t.Error("expected hasChanges=true")
	}
	if len(gitMgr.CommitCalls) != 1 || gitMgr.CommitCalls[0] != "task-1" {
		t.Errorf("expected Commit called with task-1, got %v", gitMgr.CommitCalls)
	}
}

func TestCommitChangesStep_NoChanges(t *testing.T) {
	gitMgr := &MockGitManager{HasChanges: false}

	o := &DBOSOrchestrator{
		clock:   clock.RealClock{},
		git:     gitMgr,
		verbose: true,
	}

	task := TaskInput{TaskID: "task-2", Title: "Test"}
	hasChanges, err := o.commitChangesStep(context.Background(), task, "output text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasChanges {
		t.Error("expected hasChanges=false")
	}
}

func TestCommitChangesStep_CommitError(t *testing.T) {
	gitMgr := &MockGitManager{CommitErr: errors.New("index locked")}

	o := &DBOSOrchestrator{
		clock: clock.RealClock{},
		git:   gitMgr,
	}

	task := TaskInput{TaskID: "task-3", Title: "Test"}
	_, err := o.commitChangesStep(context.Background(), task, "output")
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "committing: index locked" {
		t.Errorf("unexpected error: %v", err)
	}
}

// ============================================================================
// mergeToMainStep
// ============================================================================

func TestMergeToMainStep_Success(t *testing.T) {
	gitMgr := &MockGitManager{}

	o := &DBOSOrchestrator{
		clock: clock.RealClock{},
		git:   gitMgr,
	}

	merged, err := o.mergeToMainStep(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !merged {
		t.Error("expected merged=true")
	}
	if len(gitMgr.MergeCalls) != 1 || gitMgr.MergeCalls[0] != "task-1" {
		t.Errorf("expected MergeToMain called with task-1, got %v", gitMgr.MergeCalls)
	}
	if len(gitMgr.RemoveCalls) != 1 || gitMgr.RemoveCalls[0] != "task-1" {
		t.Errorf("expected Remove called with task-1, got %v", gitMgr.RemoveCalls)
	}
}

func TestMergeToMainStep_MergeError(t *testing.T) {
	gitMgr := &MockGitManager{MergeErr: errors.New("merge conflict")}

	o := &DBOSOrchestrator{
		clock: clock.RealClock{},
		git:   gitMgr,
	}

	_, err := o.mergeToMainStep(context.Background(), "task-2")
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "merging to main: merge conflict" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMergeToMainStep_WithStore(t *testing.T) {
	tmp := t.TempDir()
	store, _ := db.Open(tmp + "/test.db")
	defer store.Close()
	store.InitSchema()

	gitMgr := &MockGitManager{}

	o := &DBOSOrchestrator{
		clock: clock.RealClock{},
		git:   gitMgr,
		store: store,
	}

	merged, err := o.mergeToMainStep(context.Background(), "task-3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !merged {
		t.Error("expected merged=true")
	}
}

// ============================================================================
// executeClaudeStep
// ============================================================================

func TestExecuteClaudeStep_Success(t *testing.T) {
	gitMgr := NewSuccessGitManager("/tmp/wt")
	agent := &MockAgent{
		Result: &executor.ExecutionResult{
			Success:  true,
			Output:   "done",
			Duration: 1 * time.Second,
		},
	}

	o := &DBOSOrchestrator{
		clock:  clock.RealClock{},
		git:    gitMgr,
		agent:  agent,
		config: &config.Config{},
	}

	task := TaskInput{TaskID: "task-1", Title: "Test", Description: "Do things"}
	result, err := o.executeClaudeStep(context.Background(), "/tmp/wt", task, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
	if result.Output != "done" {
		t.Errorf("expected output 'done', got %q", result.Output)
	}
	if agent.Calls != 1 {
		t.Errorf("expected 1 agent call, got %d", agent.Calls)
	}
}

func TestExecuteClaudeStep_AgentFailure(t *testing.T) {
	agent := &MockAgent{
		Result: &executor.ExecutionResult{
			Success: false,
			Error:   errors.New("agent crashed"),
		},
	}

	o := &DBOSOrchestrator{
		clock:  clock.RealClock{},
		git:    &MockGitManager{},
		agent:  agent,
		config: &config.Config{},
	}

	task := TaskInput{TaskID: "task-2", Title: "Failing"}
	_, err := o.executeClaudeStep(context.Background(), "/tmp/wt", task, nil)
	if err == nil {
		t.Fatal("expected error from failed agent")
	}
	if err.Error() != "agent crashed" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestExecuteClaudeStep_WithEpicID(t *testing.T) {
	agent := &MockAgent{
		Result: &executor.ExecutionResult{Success: true, Output: "ok"},
	}

	o := &DBOSOrchestrator{
		clock:  clock.RealClock{},
		git:    &MockGitManager{},
		agent:  agent,
		config: &config.Config{},
	}

	task := TaskInput{TaskID: "task-3", Title: "Epic Task", EpicID: "epic-1"}
	result, err := o.executeClaudeStep(context.Background(), "/tmp/wt", task, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
}

// ============================================================================
// executeSubTasks — full flow with mocked git + agent
// ============================================================================

func TestExecuteSubTasks_FullSuccess(t *testing.T) {
	tmp := t.TempDir()
	store, _ := db.Open(tmp + "/test.db")
	defer store.Close()
	store.InitSchema()

	parent, _ := store.CreateTask("Parent", "", "", 0, nil)
	store.CreateSubTask("Sub 1", "Do sub thing", parent.ID, 0, nil)

	gitMgr := NewSuccessGitManager(tmp + "/worktree")
	agent := &MockAgent{
		Result: &executor.ExecutionResult{
			Success:  true,
			Output:   "sub task done",
			Duration: 500 * time.Millisecond,
		},
	}

	o := &Orchestrator{
		clock:  clock.RealClock{},
		store:  store,
		git:    gitMgr,
		agent:  agent,
		config: &config.Config{},
	}

	result := o.executeSubTasks(0, parent)
	if !result {
		t.Error("expected true for successful sub-task execution")
	}

	// Verify git operations were called
	if len(gitMgr.CreateCalls) != 1 {
		t.Errorf("expected 1 Create call, got %d", len(gitMgr.CreateCalls))
	}
	if len(gitMgr.CommitCalls) != 1 {
		t.Errorf("expected 1 Commit call, got %d", len(gitMgr.CommitCalls))
	}
	if len(gitMgr.MergeCalls) != 1 {
		t.Errorf("expected 1 MergeToMain call, got %d", len(gitMgr.MergeCalls))
	}
	if len(gitMgr.RemoveCalls) != 1 {
		t.Errorf("expected 1 Remove call, got %d", len(gitMgr.RemoveCalls))
	}
	if agent.Calls != 1 {
		t.Errorf("expected 1 agent call, got %d", agent.Calls)
	}
}

func TestExecuteSubTasks_AgentFailure(t *testing.T) {
	tmp := t.TempDir()
	store, _ := db.Open(tmp + "/test.db")
	defer store.Close()
	store.InitSchema()

	parent, _ := store.CreateTask("Parent", "", "", 0, nil)
	store.CreateSubTask("Sub 1", "", parent.ID, 0, nil)

	gitMgr := NewSuccessGitManager(tmp + "/worktree")
	agent := &MockAgent{
		Result: &executor.ExecutionResult{
			Success: false,
			Error:   errors.New("agent failed"),
		},
	}

	o := &Orchestrator{
		clock:  clock.RealClock{},
		store:  store,
		git:    gitMgr,
		agent:  agent,
		config: &config.Config{},
	}

	result := o.executeSubTasks(0, parent)
	if result {
		t.Error("expected false when agent fails")
	}
}

func TestExecuteSubTasks_GitCreateFailure(t *testing.T) {
	tmp := t.TempDir()
	store, _ := db.Open(tmp + "/test.db")
	defer store.Close()
	store.InitSchema()

	parent, _ := store.CreateTask("Parent", "", "", 0, nil)
	store.CreateSubTask("Sub 1", "", parent.ID, 0, nil)

	gitMgr := NewFailingGitManager(errors.New("disk full"))

	o := &Orchestrator{
		clock:  clock.RealClock{},
		store:  store,
		git:    gitMgr,
		config: &config.Config{},
	}

	result := o.executeSubTasks(0, parent)
	if result {
		t.Error("expected false when git create fails")
	}
}

func TestExecuteSubTasks_CommitFailure(t *testing.T) {
	tmp := t.TempDir()
	store, _ := db.Open(tmp + "/test.db")
	defer store.Close()
	store.InitSchema()

	parent, _ := store.CreateTask("Parent", "", "", 0, nil)
	store.CreateSubTask("Sub 1", "", parent.ID, 0, nil)

	gitMgr := &MockGitManager{
		WorktreePath: tmp + "/worktree",
		CommitErr:    errors.New("nothing to commit"),
	}
	agent := &MockAgent{
		Result: &executor.ExecutionResult{Success: true, Output: "done"},
	}

	o := &Orchestrator{
		clock:  clock.RealClock{},
		store:  store,
		git:    gitMgr,
		agent:  agent,
		config: &config.Config{},
	}

	result := o.executeSubTasks(0, parent)
	if result {
		t.Error("expected false when commit fails")
	}
}

func TestExecuteSubTasks_MergeFailure_StillSucceeds(t *testing.T) {
	tmp := t.TempDir()
	store, _ := db.Open(tmp + "/test.db")
	defer store.Close()
	store.InitSchema()

	parent, _ := store.CreateTask("Parent", "", "", 0, nil)
	store.CreateSubTask("Sub 1", "", parent.ID, 0, nil)

	gitMgr := &MockGitManager{
		WorktreePath: tmp + "/worktree",
		HasChanges:   true,
		MergeErr:     errors.New("merge conflict"), // Merge fails but task still completes
	}
	agent := &MockAgent{
		Result: &executor.ExecutionResult{Success: true, Output: "done"},
	}

	o := &Orchestrator{
		clock:  clock.RealClock{},
		store:  store,
		git:    gitMgr,
		agent:  agent,
		config: &config.Config{},
	}

	result := o.executeSubTasks(0, parent)
	if !result {
		t.Error("expected true even when merge fails (non-fatal)")
	}
}

func TestExecuteSubTasks_MultipleSubTasks(t *testing.T) {
	tmp := t.TempDir()
	store, _ := db.Open(tmp + "/test.db")
	defer store.Close()
	store.InitSchema()

	parent, _ := store.CreateTask("Parent", "", "", 0, nil)
	store.CreateSubTask("Sub 1", "", parent.ID, 0, nil)
	store.CreateSubTask("Sub 2", "", parent.ID, 0, nil)
	store.CreateSubTask("Sub 3", "", parent.ID, 0, nil)

	gitMgr := NewSuccessGitManager(tmp + "/worktree")
	agent := &MockAgent{
		Result: &executor.ExecutionResult{Success: true, Output: "ok"},
	}

	o := &Orchestrator{
		clock:  clock.RealClock{},
		store:  store,
		git:    gitMgr,
		agent:  agent,
		config: &config.Config{},
	}

	result := o.executeSubTasks(0, parent)
	if !result {
		t.Error("expected true for all sub-tasks succeeding")
	}
	if agent.Calls != 3 {
		t.Errorf("expected 3 agent calls, got %d", agent.Calls)
	}
	if len(gitMgr.CreateCalls) != 3 {
		t.Errorf("expected 3 Create calls, got %d", len(gitMgr.CreateCalls))
	}
}

// ============================================================================
// PrintResults (DBOSOrchestrator)
// ============================================================================

func TestDBOSPrintResults_MixedResults(t *testing.T) {
	o := &DBOSOrchestrator{clock: clock.RealClock{}}
	results := []TaskResult{
		{Success: true, Output: "done", Duration: 1 * time.Second, HasChanges: true},
		{Success: false, Error: "failed"},
		{Success: true, Output: "ok"},
	}
	// Should not panic
	o.PrintResults(results)
}

func TestDBOSPrintResults_AllSuccess(t *testing.T) {
	o := &DBOSOrchestrator{clock: clock.RealClock{}}
	results := []TaskResult{
		{Success: true},
		{Success: true},
	}
	o.PrintResults(results)
}

func TestDBOSPrintResults_Empty(t *testing.T) {
	o := &DBOSOrchestrator{clock: clock.RealClock{}}
	o.PrintResults(nil)
}

func TestDBOSPrintResults_AllFailed(t *testing.T) {
	o := &DBOSOrchestrator{clock: clock.RealClock{}}
	results := []TaskResult{
		{Success: false, Error: "err1"},
		{Success: false, Error: "err2"},
	}
	o.PrintResults(results)
}

// ============================================================================
// RegisterWorkflows (smoke test)
// ============================================================================

func TestRegisterWorkflows(t *testing.T) {
	mock := NewMockDBOSContext()
	o := &DBOSOrchestrator{
		clock:          clock.RealClock{},
		dbosCtx:        mock,
		dependencyMap:  make(map[string][]string),
		taskRegistry:   make(map[string]TaskInput),
		completedTasks: make(map[string]bool),
	}

	err := o.RegisterWorkflows()
	if err != nil {
		t.Fatalf("RegisterWorkflows error: %v", err)
	}
}

// ============================================================================
// generateWorkflowID
// ============================================================================

func TestGenerateWorkflowID(t *testing.T) {
	mockClock := clock.NewMockClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	o := &DBOSOrchestrator{clock: mockClock}
	id := o.generateWorkflowID()
	if id == "" {
		t.Error("expected non-empty workflow ID")
	}

	// Advance the clock to guarantee different ID
	mockClock.Add(time.Millisecond)
	id2 := o.generateWorkflowID()
	if id == id2 {
		t.Error("expected unique workflow IDs")
	}
}

