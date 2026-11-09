package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/cloud-shuttle/drover/internal/conversation"
	"github.com/cloud-shuttle/drover/internal/db"
	"github.com/cloud-shuttle/drover/pkg/types"
)

// These tests exercise the CRUD surfaces of db.Store that previously had no
// coverage (events, guidance, pause/resume, session shares, operators, plans,
// checkpoints and the conversation store) against a real SQLite database.

// setupMigratedDB opens a test store with the full migrated schema (events,
// plans, session shares, conversations, ... are created by MigrateSchema).
func setupMigratedDB(t *testing.T) *db.Store {
	t.Helper()
	store, _ := setupTestDB(t)
	if err := store.MigrateSchema(); err != nil {
		t.Fatalf("MigrateSchema: %v", err)
	}
	return store
}

func newTask(t *testing.T, store *db.Store, title string) *types.Task {
	t.Helper()
	epic, err := store.CreateEpic("epic "+title, "")
	if err != nil {
		t.Fatalf("CreateEpic: %v", err)
	}
	task, err := store.CreateTask(title, "desc", epic.ID, 1, nil)
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	return task
}

func TestEvents_RecordAndQuery(t *testing.T) {
	store := setupMigratedDB(t)
	defer store.Close()

	t1 := newTask(t, store, "events one")
	t2 := newTask(t, store, "events two")
	now := time.Now().Unix()
	if err := store.RecordEvent("ev-1", "task.started", now, t1.ID, t1.EpicID, `{"a":1}`); err != nil {
		t.Fatalf("RecordEvent: %v", err)
	}
	if err := store.RecordEvent("ev-2", "task.completed", now+1, t1.ID, t1.EpicID, `{}`); err != nil {
		t.Fatalf("RecordEvent: %v", err)
	}
	if err := store.RecordEvent("ev-3", "task.started", now+2, t2.ID, t2.EpicID, `{}`); err != nil {
		t.Fatalf("RecordEvent: %v", err)
	}

	all, err := store.QueryEvents(nil, "", "", 0, 0, 0)
	if err != nil {
		t.Fatalf("QueryEvents(all): %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("QueryEvents(all) = %d events, want 3", len(all))
	}

	started, err := store.QueryEvents([]string{"task.started"}, "", "", 0, 0, 0)
	if err != nil {
		t.Fatalf("QueryEvents(type): %v", err)
	}
	if len(started) != 2 {
		t.Errorf("QueryEvents(type) = %d, want 2", len(started))
	}

	filtered, err := store.QueryEvents(nil, t1.EpicID, t1.ID, now, now+1, 1)
	if err != nil {
		t.Fatalf("QueryEvents(filtered): %v", err)
	}
	if len(filtered) != 1 {
		t.Errorf("QueryEvents(filtered, limit 1) = %d, want 1", len(filtered))
	}
}

func TestTaskLifecycleOperations(t *testing.T) {
	store := setupMigratedDB(t)
	defer store.Close()

	task := newTask(t, store, "lifecycle")

	if err := store.SetTaskVerdict(task.ID, types.TaskVerdictPass, "looks good"); err != nil {
		t.Fatalf("SetTaskVerdict: %v", err)
	}
	if err := store.SetTaskTestConfig(task.ID, "strict", "diff", "go test ./..."); err != nil {
		t.Fatalf("SetTaskTestConfig: %v", err)
	}

	if err := store.PauseTask(task.ID); err == nil {
		t.Errorf("PauseTask on a ready task should fail")
	}
	if err := store.UpdateTaskStatus(task.ID, types.TaskStatusInProgress, ""); err != nil {
		t.Fatalf("UpdateTaskStatus: %v", err)
	}
	if err := store.PauseTask(task.ID); err != nil {
		t.Fatalf("PauseTask: %v", err)
	}
	if st, _ := store.GetTaskStatus(task.ID); st != types.TaskStatusPaused {
		t.Errorf("status after pause = %q, want paused", st)
	}
	if err := store.ResumeTask(task.ID); err != nil {
		t.Fatalf("ResumeTask: %v", err)
	}
	if st, _ := store.GetTaskStatus(task.ID); st == types.TaskStatusPaused {
		t.Errorf("task still paused after resume")
	}
	if err := store.ResumeTask(task.ID); err == nil {
		t.Errorf("ResumeTask on a non-paused task should fail")
	}

	if err := store.CancelTask(task.ID, "no longer needed"); err != nil {
		t.Fatalf("CancelTask: %v", err)
	}
	if st, _ := store.GetTaskStatus(task.ID); st != types.TaskStatusCancelled {
		t.Errorf("status after cancel = %q, want cancelled", st)
	}

	other := newTask(t, store, "resolve")
	if err := store.UpdateTaskStatus(other.ID, types.TaskStatusBlocked, "waiting"); err != nil {
		t.Fatalf("UpdateTaskStatus: %v", err)
	}
	if err := store.ResolveTask(other.ID, "unblocked manually"); err != nil {
		t.Fatalf("ResolveTask: %v", err)
	}
	if st, _ := store.GetTaskStatus(other.ID); st != types.TaskStatusReady {
		t.Errorf("status after resolve = %q, want ready", st)
	}
	if err := store.UpdateTaskStatus(other.ID, types.TaskStatusCompleted, ""); err != nil {
		t.Fatalf("UpdateTaskStatus: %v", err)
	}

	recent, err := store.GetRecentCompletedTasks("", 10, 3600)
	if err != nil {
		t.Fatalf("GetRecentCompletedTasks: %v", err)
	}
	_ = recent
	if _, err := store.GetRecentCompletedTasks(other.EpicID, 10, 0); err != nil {
		t.Fatalf("GetRecentCompletedTasks(epic): %v", err)
	}
}

func TestGuidance_AddDeliverClear(t *testing.T) {
	store := setupMigratedDB(t)
	defer store.Close()

	task := newTask(t, store, "guidance")

	g1, err := store.AddGuidance(task.ID, "try the other API")
	if err != nil {
		t.Fatalf("AddGuidance: %v", err)
	}
	if _, err := store.AddGuidance(task.ID, "and add a test"); err != nil {
		t.Fatalf("AddGuidance: %v", err)
	}

	pending, err := store.GetPendingGuidance(task.ID)
	if err != nil {
		t.Fatalf("GetPendingGuidance: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("pending = %d, want 2", len(pending))
	}

	if err := store.MarkGuidanceDelivered([]string{g1.ID}); err != nil {
		t.Fatalf("MarkGuidanceDelivered: %v", err)
	}
	if err := store.MarkGuidanceDelivered(nil); err != nil {
		t.Fatalf("MarkGuidanceDelivered(empty): %v", err)
	}
	pending, _ = store.GetPendingGuidance(task.ID)
	if len(pending) != 1 {
		t.Errorf("pending after deliver = %d, want 1", len(pending))
	}

	if err := store.ClearGuidance(task.ID); err != nil {
		t.Fatalf("ClearGuidance: %v", err)
	}
	pending, _ = store.GetPendingGuidance(task.ID)
	if len(pending) != 0 {
		t.Errorf("pending after clear = %d, want 0", len(pending))
	}
}

func TestSessionShares(t *testing.T) {
	store := setupMigratedDB(t)
	defer store.Close()

	share, err := store.CreateSessionShare(`{"tasks":[]}`, "alice", 24)
	if err != nil {
		t.Fatalf("CreateSessionShare: %v", err)
	}
	if share.Token == "" {
		t.Fatal("expected a share token")
	}
	if _, err := store.CreateSessionShare(`{}`, "bob", 0); err != nil {
		t.Fatalf("CreateSessionShare(no expiry): %v", err)
	}

	got, err := store.GetSessionShareByToken(share.Token)
	if err != nil {
		t.Fatalf("GetSessionShareByToken: %v", err)
	}
	if got.SessionData != `{"tasks":[]}` || got.CreatedBy != "alice" {
		t.Errorf("unexpected share: %+v", got)
	}

	if err := store.IncrementShareAccess(share.Token); err != nil {
		t.Fatalf("IncrementShareAccess: %v", err)
	}
	got, _ = store.GetSessionShareByToken(share.Token)
	if got.AccessCount != 1 {
		t.Errorf("AccessCount = %d, want 1", got.AccessCount)
	}

	if err := store.DeleteSessionShare(share.Token); err != nil {
		t.Fatalf("DeleteSessionShare: %v", err)
	}
	if _, err := store.GetSessionShareByToken(share.Token); err == nil {
		t.Error("expected error for deleted share")
	}
}

func TestOperators(t *testing.T) {
	store := setupMigratedDB(t)
	defer store.Close()

	op, err := store.CreateOperator("alice")
	if err != nil {
		t.Fatalf("CreateOperator: %v", err)
	}
	if op.APIKey == "" {
		t.Fatal("expected an API key")
	}
	if _, err := store.CreateOperator("bob"); err != nil {
		t.Fatalf("CreateOperator: %v", err)
	}

	byName, err := store.GetOperatorByName("alice")
	if err != nil || byName.ID != op.ID {
		t.Fatalf("GetOperatorByName = %+v, %v", byName, err)
	}
	if byName.LastActive != nil {
		t.Errorf("LastActive should be nil before activity")
	}
	byKey, err := store.GetOperatorByAPIKey(op.APIKey)
	if err != nil || byKey.Name != "alice" {
		t.Fatalf("GetOperatorByAPIKey = %+v, %v", byKey, err)
	}
	if _, err := store.GetOperatorByAPIKey("not-a-key"); err == nil {
		t.Error("expected error for unknown API key")
	}

	if err := store.UpdateOperatorLastActive("alice"); err != nil {
		t.Fatalf("UpdateOperatorLastActive: %v", err)
	}
	byName, _ = store.GetOperatorByName("alice")
	if byName.LastActive == nil {
		t.Error("LastActive should be set after activity")
	}
	byKey, _ = store.GetOperatorByAPIKey(op.APIKey)
	if byKey.LastActive == nil {
		t.Error("LastActive should be set (by key)")
	}

	ops, err := store.ListOperators()
	if err != nil {
		t.Fatalf("ListOperators: %v", err)
	}
	if len(ops) != 2 {
		t.Errorf("ListOperators = %d, want 2", len(ops))
	}

	if err := store.DeleteOperator("bob"); err != nil {
		t.Fatalf("DeleteOperator: %v", err)
	}
	ops, _ = store.ListOperators()
	if len(ops) != 1 {
		t.Errorf("ListOperators after delete = %d, want 1", len(ops))
	}
}

func TestPlans_SaveGetListApproveReject(t *testing.T) {
	store := setupMigratedDB(t)
	defer store.Close()

	task := newTask(t, store, "plan")
	now := time.Now()
	plan := &db.Plan{
		ID:          "plan-1",
		TaskID:      task.ID,
		Title:       "Implement thing",
		Description: "Plan description",
		Steps: []db.PlanStep{
			{Order: 1, Title: "step one", Description: "do it", Files: []string{"a.go"}},
			{Order: 2, Title: "step two", Dependencies: []int{1}},
		},
		FilesToCreate: []db.FileSpec{{Path: "a.go", Operation: "create"}},
		FilesToModify: []db.FileSpec{{Path: "b.go", Operation: "modify"}},
		Dependencies:  []string{"dep"},
		EstimatedTime: 5 * time.Minute,
		Complexity:    "low",
		RiskFactors:   []string{"none"},
		Status:        db.PlanStatusPending,
		Revision:      1,
		CreatedAt:     now,
		UpdatedAt:     now,
		CreatedBy:     "alice",
	}
	if err := store.SavePlan(plan); err != nil {
		t.Fatalf("SavePlan: %v", err)
	}

	got, err := store.GetPlan("plan-1")
	if err != nil {
		t.Fatalf("GetPlan: %v", err)
	}
	if got.Title != plan.Title || len(got.Steps) != 2 || len(got.FilesToCreate) != 1 {
		t.Errorf("round-trip mismatch: %+v", got)
	}

	byTask, err := store.GetPlanByTaskID(task.ID)
	if err != nil || byTask.ID != "plan-1" {
		t.Fatalf("GetPlanByTaskID = %+v, %v", byTask, err)
	}

	if err := store.AddFeedback("plan-1", "please add tests"); err != nil {
		t.Fatalf("AddFeedback: %v", err)
	}
	got, _ = store.GetPlan("plan-1")
	if len(got.Feedback) != 1 {
		t.Errorf("Feedback = %v, want 1 entry", got.Feedback)
	}

	pending, err := store.ListPlans(db.PlanStatusPending)
	if err != nil {
		t.Fatalf("ListPlans(pending): %v", err)
	}
	if len(pending) != 1 {
		t.Errorf("ListPlans(pending) = %d, want 1", len(pending))
	}
	all, err := store.ListPlans("")
	if err != nil {
		t.Fatalf("ListPlans(all): %v", err)
	}
	if len(all) != 1 {
		t.Errorf("ListPlans(all) = %d, want 1", len(all))
	}

	if err := store.ApprovePlan("plan-1", "bob"); err != nil {
		t.Fatalf("ApprovePlan: %v", err)
	}
	got, _ = store.GetPlan("plan-1")
	if got.Status != db.PlanStatusApproved || got.ApprovedBy != "bob" || got.ApprovedAt == nil {
		t.Errorf("after approve: status=%q by=%q at=%v", got.Status, got.ApprovedBy, got.ApprovedAt)
	}

	if err := store.RejectPlan("plan-1", "changed my mind"); err != nil {
		t.Fatalf("RejectPlan: %v", err)
	}
	got, _ = store.GetPlan("plan-1")
	if got.Status != db.PlanStatusRejected || got.RejectionReason != "changed my mind" {
		t.Errorf("after reject: status=%q reason=%q", got.Status, got.RejectionReason)
	}

	if err := store.UpdatePlanStatus("plan-1", db.PlanStatusExecuting, ""); err != nil {
		t.Fatalf("UpdatePlanStatus: %v", err)
	}

	if err := store.DeletePlan("plan-1"); err != nil {
		t.Fatalf("DeletePlan: %v", err)
	}
	if _, err := store.GetPlan("plan-1"); err == nil {
		t.Error("expected error for deleted plan")
	}
}

func TestCheckpoints(t *testing.T) {
	store := setupMigratedDB(t)
	defer store.Close()

	task := newTask(t, store, "checkpoint")
	now := time.Now().Unix()
	cp := &types.TaskCheckpoint{
		TaskID:        task.ID,
		State:         types.TaskStatusInProgress,
		WorkerPID:     4242,
		StartedAt:     now - 100,
		LastHeartbeat: now - 100,
		Attempt:       1,
	}
	if err := store.CreateCheckpoint(cp); err != nil {
		t.Fatalf("CreateCheckpoint: %v", err)
	}

	got, err := store.GetCheckpoint(task.ID)
	if err != nil {
		t.Fatalf("GetCheckpoint: %v", err)
	}
	if got.WorkerPID != 4242 {
		t.Errorf("WorkerPID = %d, want 4242", got.WorkerPID)
	}

	orphans, err := store.FindOrphanedCheckpoints(10)
	if err != nil {
		t.Fatalf("FindOrphanedCheckpoints: %v", err)
	}
	if len(orphans) != 1 {
		t.Errorf("orphans = %d, want 1 (stale heartbeat)", len(orphans))
	}

	if err := store.UpdateCheckpoint(task.ID, "partial output", time.Now().Unix()); err != nil {
		t.Fatalf("UpdateCheckpoint: %v", err)
	}
	got, _ = store.GetCheckpoint(task.ID)
	if got.Output != "partial output" {
		t.Errorf("Output = %q", got.Output)
	}
	orphans, _ = store.FindOrphanedCheckpoints(3600)
	if len(orphans) != 0 {
		t.Errorf("orphans after heartbeat = %d, want 0", len(orphans))
	}

	if err := store.CompleteCheckpoint(task.ID, types.TaskVerdictPass, "done"); err != nil {
		t.Fatalf("CompleteCheckpoint: %v", err)
	}
	if err := store.DeleteCheckpoint(task.ID); err != nil {
		t.Fatalf("DeleteCheckpoint: %v", err)
	}
	if gone, err := store.GetCheckpoint(task.ID); err != nil || gone != nil {
		t.Errorf("GetCheckpoint after delete = %+v, %v; want nil, nil", gone, err)
	}
}

func TestConversationStore(t *testing.T) {
	store := setupMigratedDB(t)
	defer store.Close()
	ctx := context.Background()
	cs := store.ConversationStore()

	task := newTask(t, store, "conversation")

	// No conversation yet: BuildContext returns an empty context.
	empty, err := cs.BuildContext(ctx, task.ID, nil)
	if err != nil {
		t.Fatalf("BuildContext(empty): %v", err)
	}
	if len(empty.Turns) != 0 {
		t.Errorf("empty context has %d turns", len(empty.Turns))
	}

	conv, err := cs.CreateConversation(ctx, task.ID, "/tmp/wt")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}

	turns := []*types.ConversationTurn{
		{ID: "turn-1", ConversationID: conv.ID, TurnNumber: 1, Role: types.ConversationRoleSystem, Content: "system prompt", TokensUsed: 10},
		{ID: "turn-2", ConversationID: conv.ID, TurnNumber: 2, Role: types.ConversationRoleUser, Content: "please refactor the parser", TokensUsed: 20},
		{ID: "turn-3", ConversationID: conv.ID, TurnNumber: 3, Role: types.ConversationRoleAssistant, Content: "refactoring parser now", TokensUsed: 30},
		{ID: "turn-4", ConversationID: conv.ID, TurnNumber: 4, Role: types.ConversationRoleTool, Content: "ran tests", ToolUseID: "tu-1", ToolName: "bash", ToolInput: "go test", ToolResult: "ok"},
	}
	for _, turn := range turns {
		if err := cs.AppendTurn(ctx, turn); err != nil {
			t.Fatalf("AppendTurn(%s): %v", turn.ID, err)
		}
	}

	got, err := cs.GetConversation(ctx, conv.ID)
	if err != nil {
		t.Fatalf("GetConversation: %v", err)
	}
	if got.TurnCount != 4 || got.TotalTokens != 60 {
		t.Errorf("metadata turn_count=%d total_tokens=%d, want 4/60", got.TurnCount, got.TotalTokens)
	}
	byTask, err := cs.GetConversationByTask(ctx, task.ID)
	if err != nil || byTask.ID != conv.ID {
		t.Fatalf("GetConversationByTask = %+v, %v", byTask, err)
	}

	if dbcs, ok := cs.(interface {
		GetTurn(context.Context, string) (*types.ConversationTurn, error)
	}); ok {
		turn, err := dbcs.GetTurn(ctx, "turn-4")
		if err != nil {
			t.Fatalf("GetTurn: %v", err)
		}
		if turn.ToolName != "bash" {
			t.Errorf("GetTurn tool = %q", turn.ToolName)
		}
	}

	recent, err := cs.GetRecentTurns(ctx, conv.ID, 2)
	if err != nil {
		t.Fatalf("GetRecentTurns: %v", err)
	}
	if len(recent) != 2 {
		t.Errorf("GetRecentTurns = %d, want 2", len(recent))
	}
	budget, err := cs.GetTurnsByTokenBudget(ctx, conv.ID, 35)
	if err != nil {
		t.Fatalf("GetTurnsByTokenBudget: %v", err)
	}
	if len(budget) == 0 || len(budget) == 4 {
		t.Errorf("GetTurnsByTokenBudget(35) = %d turns, want a subset", len(budget))
	}
	rng, err := cs.GetTurnsByRange(ctx, conv.ID, 2, 3)
	if err != nil {
		t.Fatalf("GetTurnsByRange: %v", err)
	}
	if len(rng) != 2 {
		t.Errorf("GetTurnsByRange(2,3) = %d, want 2", len(rng))
	}

	if _, err := cs.SearchTurns(ctx, "parser", 10); err != nil {
		t.Fatalf("SearchTurns: %v", err)
	}
	if _, err := cs.SearchTurnsInConversation(ctx, conv.ID, "parser; DROP", 10); err != nil {
		t.Fatalf("SearchTurnsInConversation: %v", err)
	}

	built, err := cs.BuildContext(ctx, task.ID, &conversation.BuildContextOptions{MaxTurns: 3})
	if err != nil {
		t.Fatalf("BuildContext(max turns): %v", err)
	}
	if len(built.Turns) != 3 {
		t.Errorf("BuildContext(MaxTurns 3) = %d turns", len(built.Turns))
	}
	resumed, err := cs.ResumeConversation(ctx, task.ID)
	if err != nil {
		t.Fatalf("ResumeConversation: %v", err)
	}
	if resumed.ConversationID != conv.ID {
		t.Errorf("ResumeConversation id = %q", resumed.ConversationID)
	}

	stats, err := cs.GetStats(ctx, conv.ID)
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats.TotalTurns != 4 {
		t.Errorf("stats.TotalTurns = %d, want 4", stats.TotalTurns)
	}

	if err := cs.UpdateConversationStatus(ctx, conv.ID, types.ConversationStatusPaused); err != nil {
		t.Fatalf("UpdateConversationStatus: %v", err)
	}

	pruned, err := cs.PruneConversation(ctx, conv.ID, &conversation.PruneOptions{KeepLastN: 2})
	if err != nil {
		t.Fatalf("PruneConversation(keep 2): %v", err)
	}
	if pruned == 0 {
		t.Errorf("PruneConversation(keep 2) pruned nothing")
	}

	if err := cs.ArchiveConversation(ctx, conv.ID); err != nil {
		t.Fatalf("ArchiveConversation: %v", err)
	}
	if _, err := cs.PruneConversation(ctx, conv.ID, nil); err != nil {
		t.Fatalf("PruneConversation(all): %v", err)
	}
	if err := cs.DeleteConversation(ctx, conv.ID); err != nil {
		t.Fatalf("DeleteConversation: %v", err)
	}
	if _, err := cs.GetConversation(ctx, conv.ID); err == nil {
		t.Error("expected error for deleted conversation")
	}
}
