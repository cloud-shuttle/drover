package modes

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/cloud-shuttle/drover/internal/db"
	"github.com/cloud-shuttle/drover/pkg/types"
)

// mockPlanningAgent is a mock implementation of PlanningAgent
type mockPlanningAgent struct {
	plan *db.Plan
	err  error
}

func (m *mockPlanningAgent) GeneratePlan(ctx context.Context, task *types.Task, prompt string) (*db.Plan, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.plan != nil {
		return m.plan, nil
	}
	// Return a default plan
	return &db.Plan{
		Title:       "Test Plan",
		Description: "Test Description",
		Complexity:  "low",
		Steps: []db.PlanStep{
			{Order: 1, Title: "Step 1", Description: "First step"},
		},
		FilesToCreate: []db.FileSpec{},
		FilesToModify: []db.FileSpec{},
		RiskFactors:   []string{},
	}, nil
}

// mockPlanStore is a mock implementation of PlanStore
type mockPlanStore struct {
	plans   map[string]*db.Plan
	saved   []*db.Plan
	updated map[string]db.PlanStatus
}

func newMockPlanStore() *mockPlanStore {
	return &mockPlanStore{
		plans:   make(map[string]*db.Plan),
		saved:   make([]*db.Plan, 0),
		updated: make(map[string]db.PlanStatus),
	}
}

func (m *mockPlanStore) SavePlan(plan *db.Plan) error {
	m.plans[plan.ID] = plan
	m.saved = append(m.saved, plan)
	return nil
}

func (m *mockPlanStore) GetPlan(planID string) (*db.Plan, error) {
	return m.plans[planID], nil
}

func (m *mockPlanStore) GetPlanByTaskID(taskID string) (*db.Plan, error) {
	for _, p := range m.plans {
		if p.TaskID == taskID {
			return p, nil
		}
	}
	return nil, nil
}

func (m *mockPlanStore) ListPlans(status db.PlanStatus) ([]*db.Plan, error) {
	var result []*db.Plan
	for _, p := range m.plans {
		if status == "" || p.Status == status {
			result = append(result, p)
		}
	}
	return result, nil
}

func (m *mockPlanStore) UpdatePlanStatus(planID string, status db.PlanStatus, reason string) error {
	if p, ok := m.plans[planID]; ok {
		p.Status = status
		m.updated[planID] = status
	}
	return nil
}

func (m *mockPlanStore) AddFeedback(planID string, feedback string) error {
	if p, ok := m.plans[planID]; ok {
		p.Feedback = append(p.Feedback, feedback)
	}
	return nil
}

func (m *mockPlanStore) ApprovePlan(planID string, approver string) error {
	if p, ok := m.plans[planID]; ok {
		p.Status = db.PlanStatusApproved
	}
	return nil
}

func (m *mockPlanStore) RejectPlan(planID string, feedback string) error {
	if p, ok := m.plans[planID]; ok {
		p.Status = db.PlanStatusRejected
		p.Feedback = append(p.Feedback, feedback)
	}
	return nil
}

func TestNewPlanner(t *testing.T) {
	cfg := DefaultConfig()
	agent := &mockPlanningAgent{}
	store := newMockPlanStore()

	planner := NewPlanner(cfg, agent, store)

	if planner == nil {
		t.Fatal("NewPlanner() returned nil")
	}

	if planner.config != cfg {
		t.Error("NewPlanner() config not set correctly")
	}

	if planner.agent != agent {
		t.Error("NewPlanner() agent not set correctly")
	}

	if planner.store != store {
		t.Error("NewPlanner() store not set correctly")
	}
}

func TestPlannerSetVerbose(t *testing.T) {
	planner := NewPlanner(DefaultConfig(), &mockPlanningAgent{}, newMockPlanStore())

	planner.SetVerbose(true)
	if !planner.verbose {
		t.Error("SetVerbose(true) did not set verbose to true")
	}

	planner.SetVerbose(false)
	if planner.verbose {
		t.Error("SetVerbose(false) did not set verbose to false")
	}
}

func TestPlannerCreatePlan(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Planning.RequireApproval = true
	cfg.Planning.AutoApproveLowComplexity = false

	agent := &mockPlanningAgent{
		plan: &db.Plan{
			Title:       "Test Plan",
			Description: "Test Description",
			Complexity:  "medium",
			Steps: []db.PlanStep{
				{Order: 1, Title: "Step 1", Description: "First step"},
				{Order: 2, Title: "Step 2", Description: "Second step"},
			},
			FilesToCreate: []db.FileSpec{
				{Path: "test.go", Operation: "create", Reason: "Test file"},
			},
			FilesToModify: []db.FileSpec{
				{Path: "main.go", Operation: "modify", Reason: "Add main"},
			},
			RiskFactors: []string{"Test risk"},
		},
	}

	store := newMockPlanStore()
	planner := NewPlanner(cfg, agent, store)

	task := &types.Task{
		ID:          "task-123",
		Title:       "Test Task",
		Description: "Test task description",
	}

	plan, err := planner.CreatePlan(context.Background(), task, "worker-1")
	if err != nil {
		t.Fatalf("CreatePlan() error = %v", err)
	}

	if plan == nil {
		t.Fatal("CreatePlan() returned nil plan")
	}

	// Verify plan was saved
	if len(store.saved) != 1 {
		t.Errorf("CreatePlan() saved %d plans, want 1", len(store.saved))
	}

	savedPlan := store.saved[0]
	if savedPlan.Status != db.PlanStatusPending {
		t.Errorf("CreatePlan() status = %v, want %v", savedPlan.Status, db.PlanStatusPending)
	}

	if savedPlan.TaskID != task.ID {
		t.Errorf("CreatePlan() TaskID = %v, want %v", savedPlan.TaskID, task.ID)
	}

	if savedPlan.CreatedBy != "worker-1" {
		t.Errorf("CreatePlan() CreatedBy = %v, want 'worker-1'", savedPlan.CreatedBy)
	}

	if savedPlan.Revision != 1 {
		t.Errorf("CreatePlan() Revision = %v, want 1", savedPlan.Revision)
	}
}

func TestPlannerCreatePlanAutoApprove(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Planning.RequireApproval = true
	cfg.Planning.AutoApproveLowComplexity = true

	agent := &mockPlanningAgent{
		plan: &db.Plan{
			Title:       "Test Plan",
			Description: "Test Description",
			Complexity:  "low",
			Steps:       []db.PlanStep{{Order: 1, Title: "Step 1"}},
		},
	}

	store := newMockPlanStore()
	planner := NewPlanner(cfg, agent, store)

	task := &types.Task{
		ID:          "task-456",
		Title:       "Simple Task",
		Description: "Simple task description",
	}

	plan, err := planner.CreatePlan(context.Background(), task, "worker-2")
	if err != nil {
		t.Fatalf("CreatePlan() error = %v", err)
	}

	if plan.Status != db.PlanStatusApproved {
		t.Errorf("CreatePlan() status = %v, want %v (auto-approved)", plan.Status, db.PlanStatusApproved)
	}

	if plan.ApprovedBy != "system" {
		t.Errorf("CreatePlan() ApprovedBy = %v, want 'system'", plan.ApprovedBy)
	}

	if plan.ApprovedAt == nil {
		t.Error("CreatePlan() ApprovedAt should be set for auto-approved plans")
	}
}

func TestPlannerCreatePlanNoApproval(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Planning.RequireApproval = false
	cfg.Planning.AutoApproveLowComplexity = false

	agent := &mockPlanningAgent{
		plan: &db.Plan{
			Title:       "Test Plan",
			Description: "Test Description",
			Complexity:  "high",
			Steps:       []db.PlanStep{{Order: 1, Title: "Step 1"}},
		},
	}

	store := newMockPlanStore()
	planner := NewPlanner(cfg, agent, store)

	task := &types.Task{
		ID:          "task-789",
		Title:       "High Complexity Task",
		Description: "High complexity task description",
	}

	plan, err := planner.CreatePlan(context.Background(), task, "worker-3")
	if err != nil {
		t.Fatalf("CreatePlan() error = %v", err)
	}

	if plan.Status != db.PlanStatusApproved {
		t.Errorf("CreatePlan() status = %v, want %v (no approval required)", plan.Status, db.PlanStatusApproved)
	}
}

func TestParsePlanFromOutput(t *testing.T) {
	output := "## Overview\n" +
		"This is a test plan to implement a feature.\n" +
		"\n" +
		"## Steps\n" +
		"1. **Create file**\n" +
		"   - Description: Create the main file\n" +
		"   - Files: main.go, utils.go\n" +
		"   - Dependencies:\n" +
		"   - Verification: File exists\n" +
		"\n" +
		"2. **Add tests**\n" +
		"   - Description: Add unit tests\n" +
		"   - Files: main_test.go\n" +
		"   - Dependencies: 1\n" +
		"   - Verification: Tests pass\n" +
		"\n" +
		"## Files to Create\n" +
		"- main.go: Main application file\n" +
		"- utils.go: Utility functions\n" +
		"\n" +
		"## Files to Modify\n" +
		"- config.go: Add configuration\n" +
		"\n" +
		"## Risk Factors\n" +
		"- Breaking change\n" +
		"- Performance impact\n" +
		"\n" +
		"## Complexity\n" +
		"low"

	plan, err := ParsePlanFromOutput(output)
	if err != nil {
		t.Fatalf("ParsePlanFromOutput() error = %v", err)
	}

	if plan.Description != "This is a test plan to implement a feature." {
		t.Errorf("ParsePlanFromOutput() Description = %v, want correct description", plan.Description)
	}

	if len(plan.Steps) != 2 {
		t.Fatalf("ParsePlanFromOutput() Steps length = %v, want 2", len(plan.Steps))
	}

	if plan.Steps[0].Title != "Create file" {
		t.Errorf("ParsePlanFromOutput() Step 1 Title = %v, want 'Create file'", plan.Steps[0].Title)
	}

	if plan.Steps[1].Title != "Add tests" {
		t.Errorf("ParsePlanFromOutput() Step 2 Title = %v, want 'Add tests'", plan.Steps[1].Title)
	}

	if len(plan.FilesToCreate) != 2 {
		t.Errorf("ParsePlanFromOutput() FilesToCreate length = %v, want 2", len(plan.FilesToCreate))
	}

	if len(plan.FilesToModify) != 1 {
		t.Errorf("ParsePlanFromOutput() FilesToModify length = %v, want 1", len(plan.FilesToModify))
	}

	if len(plan.RiskFactors) != 2 {
		t.Errorf("ParsePlanFromOutput() RiskFactors length = %v, want 2", len(plan.RiskFactors))
	}

	if plan.Complexity != "low" {
		t.Errorf("ParsePlanFromOutput() Complexity = %v, want 'low'", plan.Complexity)
	}
}

func TestParsePlanFromOutputEmpty(t *testing.T) {
	output := ""

	plan, err := ParsePlanFromOutput(output)
	if err != nil {
		t.Fatalf("ParsePlanFromOutput() error = %v", err)
	}

	if plan == nil {
		t.Fatal("ParsePlanFromOutput() returned nil plan")
	}

	// Should have empty defaults
	if len(plan.Steps) != 0 {
		t.Errorf("ParsePlanFromOutput() Steps length = %v, want 0", len(plan.Steps))
	}

	if plan.Complexity != "medium" { // Default complexity
		t.Errorf("ParsePlanFromOutput() Complexity = %v, want 'medium'", plan.Complexity)
	}
}

func TestParseFileList(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "single file",
			input:    "main.go",
			expected: []string{"main.go"},
		},
		{
			name:     "multiple files",
			input:    "main.go, utils.go, config.go",
			expected: []string{"main.go", "utils.go", "config.go"},
		},
		{
			name:     "files with spaces",
			input:    "main.go, utils.go, config.go",
			expected: []string{"main.go", "utils.go", "config.go"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseFileList(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("parseFileList() length = %v, want %v", len(result), len(tt.expected))
			}
			for i, f := range result {
				if f != tt.expected[i] {
					t.Errorf("parseFileList()[%d] = %v, want %v", i, f, tt.expected[i])
				}
			}
		})
	}
}

func TestParseDependencies(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []int
	}{
		{
			name:     "single dependency",
			input:    "1",
			expected: []int{1},
		},
		{
			name:     "multiple dependencies",
			input:    "1, 2, 3",
			expected: []int{1, 2, 3},
		},
		{
			name:     "empty string",
			input:    "",
			expected: []int{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseDependencies(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("parseDependencies() length = %v, want %v", len(result), len(tt.expected))
			}
			for i, d := range result {
				if d != tt.expected[i] {
					t.Errorf("parseDependencies()[%d] = %v, want %v", i, d, tt.expected[i])
				}
			}
		})
	}
}

func TestFormatPlanForDisplay(t *testing.T) {
	plan := &db.Plan{
		ID:          "plan-123",
		TaskID:      "task-456",
		Title:       "Test Plan",
		Description: "Test plan description",
		Status:      db.PlanStatusPending,
		Complexity:  "medium",
		Steps: []db.PlanStep{
			{Order: 1, Title: "Step 1", Description: "First step", Verification: "File exists"},
		},
		FilesToCreate: []db.FileSpec{
			{Path: "test.go", Reason: "Test file"},
		},
		FilesToModify: []db.FileSpec{
			{Path: "main.go", Reason: "Update main"},
		},
		RiskFactors: []string{"Test risk"},
	}

	output := FormatPlanForDisplay(plan)

	if !strings.Contains(output, "plan-123") {
		t.Error("FormatPlanForDisplay() should contain plan ID")
	}

	if !strings.Contains(output, "Test Plan") {
		t.Error("FormatPlanForDisplay() should contain plan title")
	}

	if !strings.Contains(output, "pending") {
		t.Error("FormatPlanForDisplay() should contain status")
	}

	if !strings.Contains(output, "Step 1") {
		t.Error("FormatPlanForDisplay() should contain step title")
	}

	if !strings.Contains(output, "test.go") {
		t.Error("FormatPlanForDisplay() should contain files to create")
	}

	if !strings.Contains(output, "Test risk") {
		t.Error("FormatPlanForDisplay() should contain risk factors")
	}
}

func TestPlanToJSON(t *testing.T) {
	plan := &db.Plan{
		ID:     "plan-123",
		TaskID: "task-456",
		Title:  "Test Plan",
		Steps:  []db.PlanStep{{Order: 1, Title: "Step 1"}},
	}

	jsonStr, err := PlanToJSON(plan)
	if err != nil {
		t.Fatalf("PlanToJSON() error = %v", err)
	}

	if jsonStr == "" {
		t.Error("PlanToJSON() returned empty string")
	}

	if !strings.Contains(jsonStr, "plan-123") {
		t.Error("PlanToJSON() should contain plan ID")
	}
}

func TestJSONToPlan(t *testing.T) {
	plan := &db.Plan{
		ID:     "plan-123",
		TaskID: "task-456",
		Title:  "Test Plan",
		Steps:  []db.PlanStep{{Order: 1, Title: "Step 1"}},
	}

	jsonStr, _ := PlanToJSON(plan)
	result, err := JSONToPlan(jsonStr)
	if err != nil {
		t.Fatalf("JSONToPlan() error = %v", err)
	}

	if result.ID != plan.ID {
		t.Errorf("JSONToPlan() ID = %v, want %v", result.ID, plan.ID)
	}

	if result.Title != plan.Title {
		t.Errorf("JSONToPlan() Title = %v, want %v", result.Title, plan.Title)
	}
}

func TestJSONToPlanInvalid(t *testing.T) {
	_, err := JSONToPlan("invalid json")
	if err == nil {
		t.Error("JSONToPlan() should return error for invalid JSON")
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		min      time.Duration
		max      time.Duration
	}{
		{"minutes", "5 minutes", 4*time.Minute, 6*time.Minute},
		{"mins", "10 mins", 9*time.Minute, 11*time.Minute},
		{"hours", "2 hours", 1*time.Hour, 3*time.Hour},
		{"hr", "1 hr", 30*time.Minute, 2*time.Hour},
		{"default", "invalid", 4*time.Minute, 6*time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseDuration(tt.input)
			if result < tt.min || result > tt.max {
				t.Errorf("parseDuration() = %v, want between %v and %v", result, tt.min, tt.max)
			}
		})
	}
}

func TestPlannerBuildPlanningPrompt(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Planning.MaxStepsPerPlan = 15
	cfg.Planning.PromptTemplate = DefaultPlanningPrompt()

	planner := NewPlanner(cfg, &mockPlanningAgent{}, newMockPlanStore())

	task := &types.Task{
		ID:          "task-123",
		Title:       "Test Task",
		Description: "Test task description",
		EpicID:      "epic-456",
	}

	prompt := planner.buildPlanningPrompt(task)

	if !strings.Contains(prompt, "Test Task") {
		t.Error("buildPlanningPrompt() should contain task title")
	}

	if !strings.Contains(prompt, "Test task description") {
		t.Error("buildPlanningPrompt() should contain task description")
	}

	if !strings.Contains(prompt, "epic-456") {
		t.Error("buildPlanningPrompt() should contain epic ID")
	}

	if !strings.Contains(prompt, "15") { // MaxStepsPerPlan
		t.Error("buildPlanningPrompt() should contain max steps")
	}
}
