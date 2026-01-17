package modes

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/cloud-shuttle/drover/internal/db"
	"github.com/cloud-shuttle/drover/pkg/types"
	"go.opentelemetry.io/otel/trace"
)

// mockBuildingAgent is a mock implementation of BuildingAgent
type mockBuildingAgent struct {
	result *BuildResult
	err    error
}

func (m *mockBuildingAgent) ExecutePlan(ctx context.Context, plan *db.Plan, task *types.Task, worktreePath string, span ...trace.Span) *BuildResult {
	if m.err != nil {
		return &BuildResult{
			Success:      false,
			Error:        m.err,
			FailureReason: m.err.Error(),
		}
	}
	if m.result != nil {
		return m.result
	}
	// Return a default successful result
	return &BuildResult{
		Success:        true,
		StepsCompleted: 5,
		TotalSteps:     5,
		Output:         "Build completed successfully",
		Duration:       1 * time.Second,
	}
}

func TestNewBuilder(t *testing.T) {
	cfg := DefaultConfig()
	agent := &mockBuildingAgent{}
	store := newMockPlanStore()

	builder := NewBuilder(cfg, agent, store)

	if builder == nil {
		t.Fatal("NewBuilder() returned nil")
	}

	if builder.config != cfg {
		t.Error("NewBuilder() config not set correctly")
	}

	if builder.agent != agent {
		t.Error("NewBuilder() agent not set correctly")
	}

	if builder.store != store {
		t.Error("NewBuilder() store not set correctly")
	}
}

func TestBuilderSetVerbose(t *testing.T) {
	builder := NewBuilder(DefaultConfig(), &mockBuildingAgent{}, newMockPlanStore())

	builder.SetVerbose(true)
	if !builder.verbose {
		t.Error("SetVerbose(true) did not set verbose to true")
	}

	builder.SetVerbose(false)
	if builder.verbose {
		t.Error("SetVerbose(false) did not set verbose to false")
	}
}

func TestBuilderExecutePlanSuccess(t *testing.T) {
	cfg := DefaultConfig()
	agent := &mockBuildingAgent{
		result: &BuildResult{
			Success:        true,
			StepsCompleted: 3,
			Output:         "All steps completed",
			Duration:       2 * time.Second,
		},
	}
	store := newMockPlanStore()
	builder := NewBuilder(cfg, agent, store)

	// Create a plan
	plan := &db.Plan{
		ID:     "plan-123",
		TaskID: "task-456",
		Status: db.PlanStatusApproved,
		Steps: []db.PlanStep{
			{Order: 1, Title: "Step 1"},
			{Order: 2, Title: "Step 2"},
			{Order: 3, Title: "Step 3"},
		},
	}
	store.plans[plan.ID] = plan

	task := &types.Task{
		ID:    "task-456",
		Title: "Test Task",
	}

	result := builder.ExecutePlan(context.Background(), plan, task, "/tmp/test")

	if !result.Success {
		t.Errorf("ExecutePlan() Success = false, want true")
	}

	if result.PlanID != "plan-123" {
		t.Errorf("ExecutePlan() PlanID = %v, want 'plan-123'", result.PlanID)
	}

	if result.TaskID != "task-456" {
		t.Errorf("ExecutePlan() TaskID = %v, want 'task-456'", result.TaskID)
	}

	if result.StepsCompleted != 3 {
		t.Errorf("ExecutePlan() StepsCompleted = %v, want 3", result.StepsCompleted)
	}

	if result.TotalSteps != 3 {
		t.Errorf("ExecutePlan() TotalSteps = %v, want 3", result.TotalSteps)
	}

	if plan.Status != db.PlanStatusCompleted {
		t.Errorf("ExecutePlan() plan status = %v, want %v", plan.Status, db.PlanStatusCompleted)
	}
}

func TestBuilderExecutePlanFailure(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Refinement.Enabled = false
	agent := &mockBuildingAgent{
		result: &BuildResult{
			Success:        false,
			StepsCompleted: 1,
			Error:          &testError{"build failed"},
			FailureReason:  "Build error occurred",
		},
	}
	store := newMockPlanStore()
	builder := NewBuilder(cfg, agent, store)

	// Create a plan
	plan := &db.Plan{
		ID:     "plan-789",
		TaskID: "task-999",
		Status: db.PlanStatusApproved,
		Steps: []db.PlanStep{
			{Order: 1, Title: "Step 1"},
			{Order: 2, Title: "Step 2"},
		},
	}
	store.plans[plan.ID] = plan

	task := &types.Task{
		ID:    "task-999",
		Title: "Test Task",
	}

	result := builder.ExecutePlan(context.Background(), plan, task, "/tmp/test")

	if result.Success {
		t.Error("ExecutePlan() Success = true, want false")
	}

	if plan.Status != db.PlanStatusFailed {
		t.Errorf("ExecutePlan() plan status = %v, want %v", plan.Status, db.PlanStatusFailed)
	}
}

func TestBuilderExecutePlanNeedsRefinement(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Refinement.Enabled = true
	cfg.Refinement.Triggers = []PlanRefinementTrigger{TriggerOnFailure}
	agent := &mockBuildingAgent{
		result: &BuildResult{
			Success:         false,
			StepsCompleted:  1,
			NeedsRefinement: true,
			FailureReason:   "Step 2 failed with dependency error",
		},
	}
	store := newMockPlanStore()
	builder := NewBuilder(cfg, agent, store)

	// Create a plan
	plan := &db.Plan{
		ID:       "plan-abc",
		TaskID:   "task-def",
		Status:   db.PlanStatusApproved,
		Revision: 1,
		Steps: []db.PlanStep{
			{Order: 1, Title: "Step 1"},
			{Order: 2, Title: "Step 2"},
		},
	}
	store.plans[plan.ID] = plan

	task := &types.Task{
		ID:    "task-def",
		Title: "Test Task",
	}

	result := builder.ExecutePlan(context.Background(), plan, task, "/tmp/test")

	if result.Success {
		t.Error("ExecutePlan() Success = true, want false")
	}

	if plan.Status != db.PlanStatusRejected {
		t.Errorf("ExecutePlan() plan status = %v, want %v", plan.Status, db.PlanStatusRejected)
	}
}

func TestCheckPlanReady(t *testing.T) {
	tests := []struct {
		name      string
		approved  bool
		hasSteps  bool
		wantError bool
	}{
		{
			name:      "approved plan with steps",
			approved:  true,
			hasSteps:  true,
			wantError: false,
		},
		{
			name:      "not approved plan",
			approved:  false,
			hasSteps:  true,
			wantError: true,
		},
		{
			name:      "approved plan with no steps",
			approved:  true,
			hasSteps:  false,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Building.ExecuteApprovedOnly = true
			builder := NewBuilder(cfg, &mockBuildingAgent{}, newMockPlanStore())

			plan := &db.Plan{
				ID:     "plan-test",
				Status: db.PlanStatusDraft,
				Steps:  []db.PlanStep{},
			}

			if tt.approved {
				plan.Status = db.PlanStatusApproved
			}

			if tt.hasSteps {
				plan.Steps = []db.PlanStep{{Order: 1, Title: "Step 1"}}
			}

			err := builder.CheckPlanReady(plan)
			if (err != nil) != tt.wantError {
				t.Errorf("CheckPlanReady() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestShouldRefine(t *testing.T) {
	tests := []struct {
		name          string
		enabled       bool
		triggers      []PlanRefinementTrigger
		planRevision  int
		maxRefinements int
		needsRefine   bool
		planStatus    db.PlanStatus
		wantRefine    bool
	}{
		{
			name:          "refinement enabled with on_failure trigger",
			enabled:       true,
			triggers:      []PlanRefinementTrigger{TriggerOnFailure},
			planRevision:  1,
			maxRefinements: 3,
			needsRefine:   true,
			planStatus:    db.PlanStatusFailed,
			wantRefine:    true,
		},
		{
			name:          "refinement disabled",
			enabled:       false,
			triggers:      []PlanRefinementTrigger{TriggerOnFailure},
			planRevision:  1,
			maxRefinements: 3,
			needsRefine:   true,
			planStatus:    db.PlanStatusFailed,
			wantRefine:    false,
		},
		{
			name:          "max refinements exceeded",
			enabled:       true,
			triggers:      []PlanRefinementTrigger{TriggerOnFailure},
			planRevision:  3,
			maxRefinements: 3,
			needsRefine:   true,
			planStatus:    db.PlanStatusFailed,
			wantRefine:    false,
		},
		{
			name:          "no matching trigger",
			enabled:       true,
			triggers:      []PlanRefinementTrigger{TriggerOnFeedback},
			planRevision:  1,
			maxRefinements: 3,
			needsRefine:   true,
			planStatus:    db.PlanStatusFailed,
			wantRefine:    false,
		},
		{
			name:          "plan does not need refinement",
			enabled:       true,
			triggers:      []PlanRefinementTrigger{TriggerOnFailure},
			planRevision:  1,
			maxRefinements: 3,
			needsRefine:   false,
			planStatus:    db.PlanStatusCompleted,
			wantRefine:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Refinement.Enabled = tt.enabled
			cfg.Refinement.Triggers = tt.triggers
			cfg.Refinement.MaxRefinements = tt.maxRefinements

			builder := NewBuilder(cfg, &mockBuildingAgent{}, newMockPlanStore())

			plan := &db.Plan{
				ID:       "plan-test",
				Revision: tt.planRevision,
				Status:   tt.planStatus,
			}

			result := &BuildResult{
				NeedsRefinement: tt.needsRefine,
			}

			got := builder.ShouldRefine(plan, result)
			if got != tt.wantRefine {
				t.Errorf("ShouldRefine() = %v, want %v", got, tt.wantRefine)
			}
		})
	}
}

func TestRequestRefinement(t *testing.T) {
	cfg := DefaultConfig()
	store := newMockPlanStore()
	builder := NewBuilder(cfg, &mockBuildingAgent{}, store)

	// Create original plan
	originalPlan := &db.Plan{
		ID:          "plan-123",
		TaskID:      "task-456",
		Title:       "Original Plan",
		Description: "Original description",
		Status:      db.PlanStatusFailed,
		Revision:    1,
		Steps: []db.PlanStep{
			{Order: 1, Title: "Step 1"},
		},
		FilesToCreate: []db.FileSpec{
			{Path: "test.go", Operation: "create", Reason: "Test"},
		},
		FilesToModify: []db.FileSpec{
			{Path: "main.go", Operation: "modify", Reason: "Update"},
		},
		RiskFactors: []string{"Risk 1"},
		Feedback:    []string{"Initial feedback"},
	}

	result := &BuildResult{
		FailureReason: "Step 1 failed: dependency not found",
	}

	refinedPlan, err := builder.RequestRefinement(context.Background(), originalPlan, result, "worker-1")
	if err != nil {
		t.Fatalf("RequestRefinement() error = %v", err)
	}

	if refinedPlan == nil {
		t.Fatal("RequestRefinement() returned nil plan")
	}

	// Verify refined plan properties
	if refinedPlan.ParentPlanID != "plan-123" {
		t.Errorf("RequestRefinement() ParentPlanID = %v, want 'plan-123'", refinedPlan.ParentPlanID)
	}

	if refinedPlan.TaskID != "task-456" {
		t.Errorf("RequestRefinement() TaskID = %v, want 'task-456'", refinedPlan.TaskID)
	}

	if refinedPlan.Title != "Original Plan" {
		t.Errorf("RequestRefinement() Title = %v, want 'Original Plan'", refinedPlan.Title)
	}

	if refinedPlan.Revision != 2 {
		t.Errorf("RequestRefinement() Revision = %v, want 2", refinedPlan.Revision)
	}

	if refinedPlan.CreatedBy != "worker-1" {
		t.Errorf("RequestRefinement() CreatedBy = %v, want 'worker-1'", refinedPlan.CreatedBy)
	}

	// Verify feedback was added
	hasFailureFeedback := false
	for _, f := range refinedPlan.Feedback {
		if strings.Contains(f, "Previous attempt (revision 1) failed") {
			hasFailureFeedback = true
			break
		}
	}
	if !hasFailureFeedback {
		t.Error("RequestRefinement() should include failure feedback")
	}

	// Verify original feedback is preserved
	hasOriginalFeedback := false
	for _, f := range refinedPlan.Feedback {
		if f == "Initial feedback" {
			hasOriginalFeedback = true
			break
		}
	}
	if !hasOriginalFeedback {
		t.Error("RequestRefinement() should preserve original feedback")
	}

	// Verify files and risks are copied
	if len(refinedPlan.FilesToCreate) != 1 {
		t.Errorf("RequestRefinement() FilesToCreate length = %v, want 1", len(refinedPlan.FilesToCreate))
	}

	if len(refinedPlan.FilesToModify) != 1 {
		t.Errorf("RequestRefinement() FilesToModify length = %v, want 1", len(refinedPlan.FilesToModify))
	}

	if len(refinedPlan.RiskFactors) != 1 {
		t.Errorf("RequestRefinement() RiskFactors length = %v, want 1", len(refinedPlan.RiskFactors))
	}
}

func TestBuildExecutionPrompt(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Building.PromptTemplate = DefaultBuildingPrompt()
	builder := NewBuilder(cfg, &mockBuildingAgent{}, newMockPlanStore())

	plan := &db.Plan{
		Title:       "Test Plan",
		Description: "Test plan description",
		Steps: []db.PlanStep{
			{
				Order:       1,
				Title:       "Create file",
				Description: "Create main.go",
				Files:       []string{"main.go"},
				Verification: "File exists",
			},
		},
	}

	task := &types.Task{
		ID:          "task-123",
		Title:       "Test Task",
		Description: "Test task description",
	}

	prompt := builder.BuildExecutionPrompt(plan, task)

	// Verify prompt contains key elements
	if !strings.Contains(prompt, "Test Task") {
		t.Error("BuildExecutionPrompt() should contain task title")
	}

	if !strings.Contains(prompt, "Test task description") {
		t.Error("BuildExecutionPrompt() should contain task description")
	}

	if !strings.Contains(prompt, "Test plan description") {
		t.Error("BuildExecutionPrompt() should contain plan overview")
	}

	if !strings.Contains(prompt, "Create file") {
		t.Error("BuildExecutionPrompt() should contain step title")
	}

	if !strings.Contains(prompt, "main.go") {
		t.Error("BuildExecutionPrompt() should contain step files")
	}
}

func TestFormatStringSlice(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected string
	}{
		{
			name:     "empty slice",
			input:    []string{},
			expected: "none",
		},
		{
			name:     "single item",
			input:    []string{"main.go"},
			expected: "'main.go'",
		},
		{
			name:     "multiple items",
			input:    []string{"main.go", "utils.go", "config.go"},
			expected: "'main.go', 'utils.go', 'config.go'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatStringSlice(tt.input)
			if result != tt.expected {
				t.Errorf("formatStringSlice() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// testError is a simple error type for testing
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

func TestBuildResultFields(t *testing.T) {

	result := BuildResult{
		Success:         true,
		PlanID:          "plan-123",
		TaskID:          "task-456",
		StepsCompleted:  5,
		TotalSteps:      10,
		Output:          "Build output",
		Duration:        5 * time.Second,
		NeedsRefinement: false,
		FailureReason:   "",
	}

	if !result.Success {
		t.Error("BuildResult.Success should be true")
	}

	if result.PlanID != "plan-123" {
		t.Errorf("BuildResult.PlanID = %v, want 'plan-123'", result.PlanID)
	}

	if result.TaskID != "task-456" {
		t.Errorf("BuildResult.TaskID = %v, want 'task-456'", result.TaskID)
	}

	if result.StepsCompleted != 5 {
		t.Errorf("BuildResult.StepsCompleted = %v, want 5", result.StepsCompleted)
	}

	if result.TotalSteps != 10 {
		t.Errorf("BuildResult.TotalSteps = %v, want 10", result.TotalSteps)
	}
}
