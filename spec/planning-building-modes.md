# Planning/Building Modes Specification

**Status:** ✅ Implemented | **Since:** v0.3.0 (Epic 5)

## Overview

Drover separates planning from building with distinct worker modes. This enables review of implementation plans before execution, reducing wasted work and ensuring alignment.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Drover Modes System                      │
│                                                              │
│  ┌────────────────┐      ┌──────────────────┐              │
│  │  Planning Mode │─────►│  Building Mode   │              │
│  │  (create plans)│      │  (execute plans) │              │
│  └────────────────┘      └──────────────────┘              │
│           │                        ▲                         │
│           │                        │                         │
│           ▼                        │                         │
│  ┌────────────────┐                │                         │
│  │  Plan Review   │────────────────┘                         │
│  │  (TUI approve) │                                          │
│  └────────────────┘                                          │
│                                                              │
│  ┌────────────────┐      ┌──────────────────┐              │
│  │ Combined Mode  │ =    │ Planning + Building             │
│  │ (single worker) │      │ (default behavior)             │
│  └────────────────┘      └──────────────────┘              │
└─────────────────────────────────────────────────────────────┘
```

## Worker Modes

### Combined Mode (Default)

Single worker handles both planning and building.

```bash
drover run --workers 4 --mode combined
```

**Characteristics:**
- Single agent per task
- Plans and executes in one session
- No approval step
- Fastest execution

### Planning Mode

Dedicated planning workers only create implementation plans.

```bash
drover run --workers 2 --mode planning
```

**Characteristics:**
- Analyzes task requirements
- Creates detailed implementation plans
- Does not make code changes
- Outputs plan to database

### Building Mode

Dedicated building workers only execute approved plans.

```bash
drover run --workers 4 --mode building
```

**Characteristics:**
- Reads approved plans
- Executes step-by-step
- Reports progress
- Does not deviate from plan

## Planning Mode

### Planning Prompt Template

```markdown
## PLANNING MODE

You are a planning agent. Your job is to create a detailed implementation plan.

### Task
{{.TaskTitle}}
{{.TaskDescription}}

### Requirements
1. Create a detailed step-by-step implementation plan
2. DO NOT make any code changes
3. DO NOT use file editing tools
4. Focus on thorough analysis and planning

### Output Format
## Summary
[Brief overview of approach]

## Steps
1. [Step Title]
   - Description: ...
   - Files: ...
   - Dependencies: ...
   - Verification: ...

## Files to Create
- path/to/file.go (description)

## Files to Modify
- path/to/existing.go (changes needed)

## Risk Factors
- Potential blockers or risks

## Complexity
- Estimated complexity: Low/Medium/High
```

### Plan Structure

```go
type Plan struct {
    ID          string
    TaskID      string
    EpicID      string
    Status      PlanStatus  // pending, approved, rejected, expired
    Overview    string
    Steps       []PlanStep
    FilesToCreate  []FilePlan
    FilesToModify  []FilePlan
    RiskFactors []string
    Complexity  string
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

type PlanStep struct {
    Order       int
    Title       string
    Description string
    Files       []string
    Dependencies []string
    Verification string
}
```

## Building Mode

### Building Prompt Template

```markdown
## BUILDING MODE

You are a building agent. Your job is to execute an approved implementation plan.

### Task
{{.TaskTitle}}
{{.TaskDescription}}

### Approved Plan
{{.PlanOverview}}

## Steps
{{.PlanSteps}}

### Requirements
1. Follow the plan exactly
2. ONLY make changes specified in the approved plan
3. If you encounter issues not covered in the plan, STOP and report
4. Run tests if specified in verification steps
5. Report progress after each step

### Progress Format
Step [N]/[Total]: [Step Title]
Status: Completed / Failed / In Progress
Details: [What you did]
```

### Execution Flow

```go
func ExecutePlan(ctx context.Context, plan *Plan) error {
    for i, step := range plan.Steps {
        log.Printf("Step %d/%d: %s", i+1, len(plan.Steps), step.Title)

        // Execute step
        err := executeStep(ctx, step)
        if err != nil {
            return fmt.Errorf("step %d failed: %w", i+1, err)
        }

        // Verification
        if step.Verification != "" {
            if !verifyStep(ctx, step) {
                return fmt.Errorf("step %d verification failed", i+1)
            }
        }
    }
    return nil
}
```

## Plan Review TUI

Interactive terminal UI for reviewing and approving plans.

### Launch

```bash
drover plan review
```

### Controls

| Key | Action |
|-----|--------|
| `↑`/`↓` | Navigate plans |
| `Enter` | View plan details |
| `a` | Approve plan |
| `r` | Reject plan (with feedback) |
| `f` | Filter by status |
| `q` | Quit |

### Plan Detail View

```
┌─────────────────────────────────────────────────────────┐
│ Plan: task-123: Implement OAuth flow                    │
├─────────────────────────────────────────────────────────┤
│                                                           │
│ Overview:                                                 │
│ Implement OAuth 2.0 authentication flow...              │
│                                                           │
│ Steps (5):                                                │
│ 1. Design user schema                                    │
│ 2. Implement OAuth endpoints                             │
│ 3. Add JWT middleware                                    │
│ 4. Build login/signup UI                                 │
│ 5. Add tests                                             │
│                                                           │
│ Files to Create:                                         │
│ - internal/auth/oauth.go                                 │
│ - internal/auth/jwt.go                                   │
│                                                           │
│ [a] Approve  [r] Reject  [q] Back                        │
└─────────────────────────────────────────────────────────┘
```

## Plan Refinement

When a plan is rejected with feedback, it enters refinement mode.

### Refinement Prompt

```markdown
## PLAN REFINEMENT MODE

Your previous plan was rejected. You need to improve it based on feedback.

### Original Plan
{{.PlanOverview}}

### Rejection Reason
{{.FailureReason}}

### Your Task
Revise the plan to address the feedback. Provide:
- What changed from the original plan
- Root cause of previous failure (if applicable)
- Revised steps

### Output Format
[Same as planning mode, but highlight changes]
```

## Configuration

### Worker Mode Selection

```bash
# Command-line flag
drover run --mode planning
drover run --mode building
drover run --mode combined  # default
```

### Environment Variable

```bash
export DROVER_WORKER_MODE=planning
drover run
```

### Configuration File

```yaml
# ~/.drover/config.yaml or .drover/config.yaml

worker:
  mode: planning  # planning | building | combined
  planning:
    max_steps: 20
    require_approval: true
  building:
    stop_on_error: true
    verify_steps: true
```

## CLI Commands

### Plan Management

```bash
# Review and approve plans
drover plan review

# List all plans
drover plan list

# Show plan details
drover plan show <plan-id>

# Approve plan
drover plan approve <plan-id>

# Reject plan with feedback
drover plan reject <plan-id> --feedback "Missing error handling"

# Delete plan
drover plan delete <plan-id>
```

### Filtering

```bash
# Filter by status
drover plan list --status pending
drover plan list --status approved,rejected

# Filter by epic
drover plan list --epic epic-abc

# Filter by task
drover plan list --task task-123
```

## Workflows

### Typical Workflow

1. **Create tasks**
   ```bash
   drover add "Implement OAuth flow" --epic epic-auth
   ```

2. **Generate plans** (planning mode)
   ```bash
   drover run --workers 2 --mode planning
   ```

3. **Review plans**
   ```bash
   drover plan review
   # Approve/reject plans in TUI
   ```

4. **Execute plans** (building mode)
   ```bash
   drover run --workers 4 --mode building
   ```

### Combined Workflow

For rapid iteration without review:

```bash
drover run --workers 4 --mode combined
```

## Database Schema

```sql
-- Plans table
CREATE TABLE plans (
    id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL,
    epic_id TEXT,
    status TEXT DEFAULT 'pending',
    overview TEXT,
    steps_json TEXT,  -- JSON encoded []PlanStep
    files_to_create_json TEXT,  -- JSON encoded []FilePlan
    files_to_modify_json TEXT,  -- JSON encoded []FilePlan
    risk_factors TEXT,  -- JSON encoded []string
    complexity TEXT,
    rejection_reason TEXT,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
    FOREIGN KEY (epic_id) REFERENCES epics(id) ON DELETE CASCADE
);

CREATE INDEX idx_plans_status ON plans(status);
CREATE INDEX idx_plans_task ON plans(task_id);
CREATE INDEX idx_plans_epic ON plans(epic_id);
```

## See Also

- [Internal Modes Documentation](../internal/modes/README.md)
- [Prompts](../internal/modes/prompts.go)
- [Planner](../internal/modes/planner.go)
- [Builder](../internal/modes/builder.go)
- [Plan Review TUI](../internal/tui/planreview.go)

---

*Last updated: 2026-01-16*
