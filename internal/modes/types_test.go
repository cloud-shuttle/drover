package modes

import (
	"testing"
	"time"

	"github.com/cloud-shuttle/drover/internal/db"
	"github.com/cloud-shuttle/drover/pkg/types"
)

func TestWorkerModeString(t *testing.T) {
	tests := []struct {
		name     string
		mode     WorkerMode
		expected string
	}{
		{"combined mode", ModeCombined, "combined"},
		{"planning mode", ModePlanning, "planning"},
		{"building mode", ModeBuilding, "building"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mode.String(); got != tt.expected {
				t.Errorf("WorkerMode.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestWorkerModeIsValid(t *testing.T) {
	tests := []struct {
		name     string
		mode     WorkerMode
		expected bool
	}{
		{"combined mode", ModeCombined, true},
		{"planning mode", ModePlanning, true},
		{"building mode", ModeBuilding, true},
		{"invalid mode", WorkerMode("invalid"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mode.IsValid(); got != tt.expected {
				t.Errorf("WorkerMode.IsValid() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg == nil {
		t.Fatal("DefaultConfig() returned nil")
	}

	if cfg.Mode != ModeCombined {
		t.Errorf("DefaultConfig() Mode = %v, want %v", cfg.Mode, ModeCombined)
	}

	if !cfg.Planning.RequireApproval {
		t.Error("DefaultConfig() Planning.RequireApproval should be true")
	}

	if !cfg.Planning.AutoApproveLowComplexity {
		t.Error("DefaultConfig() Planning.AutoApproveLowComplexity should be true")
	}

	if cfg.Planning.MaxStepsPerPlan != 20 {
		t.Errorf("DefaultConfig() Planning.MaxStepsPerPlan = %v, want 20", cfg.Planning.MaxStepsPerPlan)
	}

	if !cfg.Building.ExecuteApprovedOnly {
		t.Error("DefaultConfig() Building.ExecuteApprovedOnly should be true")
	}

	if !cfg.Building.VerifySteps {
		t.Error("DefaultConfig() Building.VerifySteps should be true")
	}

	if !cfg.Refinement.Enabled {
		t.Error("DefaultConfig() Refinement.Enabled should be true")
	}

	if len(cfg.Refinement.Triggers) == 0 {
		t.Error("DefaultConfig() Refinement.Triggers should not be empty")
	}

	if cfg.Refinement.MaxRefinements != 3 {
		t.Errorf("DefaultConfig() Refinement.MaxRefinements = %v, want 3", cfg.Refinement.MaxRefinements)
	}
}

func TestPlanRefinementTriggerString(t *testing.T) {
	tests := []struct {
		trigger  PlanRefinementTrigger
		expected string
	}{
		{TriggerOnFailure, "on_failure"},
		{TriggerOnFeedback, "on_feedback"},
		{TriggerOnComplexity, "on_complexity"},
		{TriggerOnDependency, "on_dependency"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.trigger.String(); got != tt.expected {
				t.Errorf("PlanRefinementTrigger.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestStepResult(t *testing.T) {
	result := StepResult{
		Success:     true,
		Output:      "test output",
		CompletedAt: time.Now(),
		Duration:    100 * time.Millisecond,
	}

	if !result.Success {
		t.Error("StepResult.Success should be true")
	}

	if result.Output != "test output" {
		t.Errorf("StepResult.Output = %v, want 'test output'", result.Output)
	}

	if result.Duration != 100*time.Millisecond {
		t.Errorf("StepResult.Duration = %v, want 100ms", result.Duration)
	}
}

func TestBuildResult(t *testing.T) {
	result := BuildResult{
		Success:        true,
		PlanID:         "plan-123",
		TaskID:         "task-456",
		StepsCompleted: 5,
		TotalSteps:     10,
		Output:         "build output",
		Duration:       5 * time.Second,
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

	if result.Output != "build output" {
		t.Errorf("BuildResult.Output = %v, want 'build output'", result.Output)
	}

	if result.Duration != 5*time.Second {
		t.Errorf("BuildResult.Duration = %v, want 5s", result.Duration)
	}
}

func TestExecutionContext(t *testing.T) {
	plan := &db.Plan{
		ID:     "plan-123",
		TaskID: "task-456",
		Steps: []db.PlanStep{
			{Order: 1, Title: "Step 1"},
			{Order: 2, Title: "Step 2"},
		},
	}

	task := &types.Task{
		ID:          "task-456",
		Title:       "Test Task",
		Description: "Test Description",
	}

	ctx := ExecutionContext{
		Plan:        plan,
		CurrentStep: 0,
		StepResults: make(map[int]StepResult),
		WorkDir:     "/tmp/test",
		Task:        task,
	}

	if ctx.Plan.ID != "plan-123" {
		t.Errorf("ExecutionContext.Plan.ID = %v, want 'plan-123'", ctx.Plan.ID)
	}

	if ctx.CurrentStep != 0 {
		t.Errorf("ExecutionContext.CurrentStep = %v, want 0", ctx.CurrentStep)
	}

	if len(ctx.StepResults) != 0 {
		t.Errorf("ExecutionContext.StepResults length = %v, want 0", len(ctx.StepResults))
	}

	if ctx.WorkDir != "/tmp/test" {
		t.Errorf("ExecutionContext.WorkDir = %v, want '/tmp/test'", ctx.WorkDir)
	}

	if ctx.Task.ID != "task-456" {
		t.Errorf("ExecutionContext.Task.ID = %v, want 'task-456'", ctx.Task.ID)
	}
}

func TestConvertPlanToModes(t *testing.T) {
	plan := &db.Plan{
		ID:     "plan-123",
		TaskID: "task-456",
		Status: db.PlanStatusDraft,
	}

	// These are no-op functions now but test they don't crash
	result := ConvertPlanToModes(plan)
	if result.ID != "plan-123" {
		t.Errorf("ConvertPlanToModes() = %v, want plan-123", result.ID)
	}

	result2 := ConvertPlanToDB(plan)
	if result2.ID != "plan-123" {
		t.Errorf("ConvertPlanToDB() = %v, want plan-123", result2.ID)
	}
}

func TestPlanningConfigDefaults(t *testing.T) {
	cfg := PlanningConfig{}

	// Test zero values
	if cfg.RequireApproval {
		t.Error("PlanningConfig.RequireApproval should be false by default")
	}

	if cfg.AutoApproveLowComplexity {
		t.Error("PlanningConfig.AutoApproveLowComplexity should be false by default")
	}

	if cfg.MaxStepsPerPlan != 0 {
		t.Errorf("PlanningConfig.MaxStepsPerPlan = %v, want 0", cfg.MaxStepsPerPlan)
	}
}

func TestBuildingConfigDefaults(t *testing.T) {
	cfg := BuildingConfig{}

	// Test zero values
	if cfg.ExecuteApprovedOnly {
		t.Error("BuildingConfig.ExecuteApprovedOnly should be false by default")
	}

	if cfg.VerifySteps {
		t.Error("BuildingConfig.VerifySteps should be false by default")
	}
}

func TestRefinementConfigDefaults(t *testing.T) {
	cfg := RefinementConfig{}

	// Test zero values
	if cfg.Enabled {
		t.Error("RefinementConfig.Enabled should be false by default")
	}

	if len(cfg.Triggers) != 0 {
		t.Errorf("RefinementConfig.Triggers length = %v, want 0", len(cfg.Triggers))
	}

	if cfg.MaxRefinements != 0 {
		t.Errorf("RefinementConfig.MaxRefinements = %v, want 0", cfg.MaxRefinements)
	}
}
