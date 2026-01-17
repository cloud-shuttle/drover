// Package modes implements planning/building worker mode separation
package modes

import (
	"time"

	"github.com/cloud-shuttle/drover/internal/db"
	"github.com/cloud-shuttle/drover/pkg/types"
)

// WorkerMode defines the operational mode for workers
type WorkerMode string

const (
	// ModeCombined is the traditional mode where planning and building happen together
	ModeCombined WorkerMode = "combined"
	// ModePlanning is for creating detailed plans without making changes
	ModePlanning WorkerMode = "planning"
	// ModeBuilding is for executing approved plans
	ModeBuilding WorkerMode = "building"
)

// String returns the string representation of the mode
func (m WorkerMode) String() string {
	return string(m)
}

// IsValid checks if the mode is valid
func (m WorkerMode) IsValid() bool {
	switch m {
	case ModeCombined, ModePlanning, ModeBuilding:
		return true
	}
	return false
}

// PlanRefinementTrigger defines when a plan should be refined
type PlanRefinementTrigger string

const (
	TriggerOnFailure    PlanRefinementTrigger = "on_failure"  // Auto-refine on execution failure
	TriggerOnFeedback   PlanRefinementTrigger = "on_feedback"  // User requests refinement
	TriggerOnComplexity PlanRefinementTrigger = "on_complexity" // Plan too complex
	TriggerOnDependency PlanRefinementTrigger = "on_dependency" // Dependencies changed
)

// String returns the string representation of the trigger
func (t PlanRefinementTrigger) String() string {
	return string(t)
}

// Config holds configuration for worker modes
type Config struct {
	// Mode is the current worker mode
	Mode WorkerMode `json:"mode" yaml:"mode"`

	// Planning configuration
	Planning PlanningConfig `json:"planning" yaml:"planning"`

	// Building configuration
	Building BuildingConfig `json:"building" yaml:"building"`

	// Refinement configuration
	Refinement RefinementConfig `json:"refinement" yaml:"refinement"`
}

// PlanningConfig holds configuration for planning mode
type PlanningConfig struct {
	// RequireApproval requires manual approval before plans can be built
	RequireApproval bool `json:"require_approval" yaml:"require_approval"`

	// AutoApproveLowComplexity automatically approves low-complexity plans
	AutoApproveLowComplexity bool `json:"auto_approve_low_complexity" yaml:"auto_approve_low_complexity"`

	// MaxStepsPerPlan limits the number of steps in a single plan
	MaxStepsPerPlan int `json:"max_steps_per_plan" yaml:"max_steps_per_plan"`

	// PromptTemplate is the template for planning prompts
	PromptTemplate string `json:"prompt_template,omitempty" yaml:"prompt_template,omitempty"`
}

// BuildingConfig holds configuration for building mode
type BuildingConfig struct {
	// ExecuteApprovedOnly only executes approved plans
	ExecuteApprovedOnly bool `json:"execute_approved_only" yaml:"execute_approved_only"`

	// VerifySteps verifies each step after execution
	VerifySteps bool `json:"verify_steps" yaml:"verify_steps"`

	// PromptTemplate is the template for building prompts
	PromptTemplate string `json:"prompt_template,omitempty" yaml:"prompt_template,omitempty"`
}

// RefinementConfig holds configuration for plan refinement
type RefinementConfig struct {
	// Enabled enables automatic plan refinement
	Enabled bool `json:"enabled" yaml:"enabled"`

	// Triggers defines when refinement should occur
	Triggers []PlanRefinementTrigger `json:"triggers" yaml:"triggers"`

	// MaxRefinements is the maximum number of refinements allowed
	MaxRefinements int `json:"max_refinements" yaml:"max_refinements"`

	// PromptTemplate is the template for refinement prompts
	PromptTemplate string `json:"prompt_template,omitempty" yaml:"prompt_template,omitempty"`
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		Mode: ModeCombined,
		Planning: PlanningConfig{
			RequireApproval:         true,
			AutoApproveLowComplexity: true,
			MaxStepsPerPlan:         20,
			PromptTemplate:          DefaultPlanningPrompt(),
		},
		Building: BuildingConfig{
			ExecuteApprovedOnly: true,
			VerifySteps:         true,
			PromptTemplate:      DefaultBuildingPrompt(),
		},
		Refinement: RefinementConfig{
			Enabled:        true,
			Triggers:       []PlanRefinementTrigger{TriggerOnFailure, TriggerOnFeedback},
			MaxRefinements: 3,
			PromptTemplate: DefaultRefinementPrompt(),
		},
	}
}

// ExecutionContext provides context for plan execution
type ExecutionContext struct {
	// The plan being executed
	Plan *db.Plan `json:"plan"`

	// Current step being executed
	CurrentStep int `json:"current_step"`

	// Results from previous steps
	StepResults map[int]StepResult `json:"step_results,omitempty"`

	// Working directory
	WorkDir string `json:"work_dir"`

	// Task context
	Task *types.Task `json:"task"`
}

// StepResult represents the result of executing a plan step
type StepResult struct {
	Success     bool        `json:"success"`
	Output      string      `json:"output,omitempty"`
	Error       string      `json:"error,omitempty"`
	CompletedAt time.Time   `json:"completed_at"`
	Duration    time.Duration `json:"duration"`
}

// ConvertPlanToModes converts a db.Plan to modes.Plan (no-op since we use db.Plan directly now)
// This function is kept for compatibility
func ConvertPlanToModes(plan *db.Plan) *db.Plan {
	return plan
}

// ConvertPlanToDB converts a modes.Plan to db.Plan (no-op since we use db.Plan directly now)
// This function is kept for compatibility
func ConvertPlanToDB(plan *db.Plan) *db.Plan {
	return plan
}
