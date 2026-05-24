package main

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/cloud-shuttle/drover/internal/config"
	"github.com/cloud-shuttle/drover/internal/db"
	"github.com/cloud-shuttle/drover/internal/tui"
	"github.com/spf13/cobra"
)

func planCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Manage implementation plans",
		Long: `Manage implementation plans for planning/building workflow separation.

Use these commands to review, approve, and track plans created by planning workers.`,
	}

	cmd.AddCommand(
		planListCmd(),
		planShowCmd(),
		planApproveCmd(),
		planRejectCmd(),
		planDeleteCmd(),
		planReviewCmd(),
	)

	return cmd
}

// planListCmd lists all plans with optional filtering
func planListCmd() *cobra.Command {
	var statusFilter string

	command := &cobra.Command{
		Use:   "list",
		Short: "List all implementation plans",
		Long: `List all implementation plans with optional filtering by status.

Available statuses:
  - draft: Plan is being created
  - pending: Plan awaiting review
  - approved: Plan approved for execution
  - rejected: Plan was rejected
  - executing: Plan is currently being executed
  - completed: Plan execution completed
  - failed: Plan execution failed`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			var plans []*db.Plan
			if statusFilter != "" {
				plans, err = store.ListPlans(db.PlanStatus(statusFilter))
				if err != nil {
					return fmt.Errorf("listing plans: %w", err)
				}
			} else {
				// Get all plans by querying without status filter
				plans, err = store.ListPlans("")
				if err != nil {
					return fmt.Errorf("listing plans: %w", err)
				}
			}

			if len(plans) == 0 {
				fmt.Println("No plans found.")
				return nil
			}

			fmt.Printf("\n📋 Plans (%d)\n", len(plans))
			fmt.Println("══════════════════")

			for _, plan := range plans {
				fmt.Printf("\n%s %s\n", formatPlanStatus(plan.Status), plan.ID)
				fmt.Printf("  Title:    %s\n", plan.Title)
				fmt.Printf("  Task:     %s\n", plan.TaskID)
				if plan.Complexity != "" {
					fmt.Printf("  Complexity: %s\n", plan.Complexity)
				}
				if plan.EstimatedTime > 0 {
					fmt.Printf("  Est. Time: %s\n", plan.EstimatedTime)
				}
				fmt.Printf("  Steps:    %d\n", len(plan.Steps))
				if plan.ApprovedBy != "" {
					fmt.Printf("  Approved: by %s\n", plan.ApprovedBy)
				}
				if plan.RejectionReason != "" {
					fmt.Printf("  Rejected: %s\n", plan.RejectionReason)
				}
			}
			fmt.Println()

			return nil
		},
	}

	command.Flags().StringVarP(&statusFilter, "status", "s", "", "Filter by plan status")
	return command
}

// planShowCmd shows detailed information about a specific plan
func planShowCmd() *cobra.Command {
	command := &cobra.Command{
		Use:   "show <plan-id>",
		Short: "Show detailed plan information",
		Long: `Show detailed information about a specific implementation plan.

Displays the plan's title, description, steps, risk factors, and other metadata.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			planID := args[0]
			plan, err := store.GetPlan(planID)
			if err != nil {
				return fmt.Errorf("plan not found: %s", planID)
			}

			printPlanDetails(plan)
			return nil
		},
	}

	return command
}

// planApproveCmd approves a plan for execution
func planApproveCmd() *cobra.Command {
	command := &cobra.Command{
		Use:   "approve <plan-id>",
		Short: "Approve a plan for execution",
		Long: `Approve an implementation plan, allowing it to be executed by building workers.

The plan must be in 'pending' or 'draft' status to be approved.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			planID := args[0]

			// Get operator name
			operator := config.GetOperator()
			if operator == "" {
				operator = "cli"
			}

			// Approve the plan
			if err := store.ApprovePlan(planID, operator); err != nil {
				return fmt.Errorf("approving plan: %w", err)
			}

			fmt.Printf("✅ Approved plan %s\n", planID)

			return nil
		},
	}

	return command
}

// planRejectCmd rejects a plan with optional feedback
func planRejectCmd() *cobra.Command {
	var feedback string

	command := &cobra.Command{
		Use:   "reject <plan-id>",
		Short: "Reject a plan",
		Long: `Reject an implementation plan with optional feedback.

The plan will be marked as rejected and the feedback will be saved
for the planning worker to use when creating a revised plan.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			planID := args[0]

			// If no feedback provided via flag, prompt for it
			if feedback == "" {
				fmt.Print("Enter rejection reason (optional, press Enter to skip): ")
				fmt.Scanln(&feedback)
			}

			// Reject the plan
			if err := store.RejectPlan(planID, feedback); err != nil {
				return fmt.Errorf("rejecting plan: %w", err)
			}

			fmt.Printf("❌ Rejected plan %s\n", planID)
			if feedback != "" {
				fmt.Printf("   Feedback: %s\n", feedback)
			}

			return nil
		},
	}

	command.Flags().StringVarP(&feedback, "feedback", "f", "", "Rejection reason/feedback")
	return command
}

// planDeleteCmd deletes a plan
func planDeleteCmd() *cobra.Command {
	var force bool

	command := &cobra.Command{
		Use:   "delete <plan-id>",
		Short: "Delete a plan",
		Long: `Delete an implementation plan from the database.

Warning: This action cannot be undone. Use --force to skip confirmation.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			planID := args[0]

			// Confirm unless --force
			if !force {
				fmt.Printf("Delete plan %s? [y/N] ", planID)
				var response string
				fmt.Scanln(&response)
				if response != "y" && response != "Y" {
					fmt.Println("Aborted")
					return nil
				}
			}

			// Delete the plan
			if err := store.DeletePlan(planID); err != nil {
				return fmt.Errorf("deleting plan: %w", err)
			}

			fmt.Printf("🗑️  Deleted plan %s\n", planID)

			return nil
		},
	}

	command.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation")
	return command
}

// printPlanDetails prints detailed information about a plan
func printPlanDetails(plan *db.Plan) {
	fmt.Println("\n📋 Plan Details")
	fmt.Println("════════════════")

	fmt.Printf("\nID:         %s\n", plan.ID)
	fmt.Printf("Status:     %s\n", formatPlanStatus(plan.Status))
	fmt.Printf("Title:      %s\n", plan.Title)
	fmt.Printf("Task:       %s\n", plan.TaskID)

	if plan.Description != "" {
		fmt.Printf("\nDescription:\n  %s\n", plan.Description)
	}

	if plan.Complexity != "" {
		fmt.Printf("\nComplexity:  %s", plan.Complexity)
	}

	if plan.EstimatedTime > 0 {
		fmt.Printf("\nEst. Time:  %s", plan.EstimatedTime)
	}

	if len(plan.RiskFactors) > 0 {
		fmt.Printf("\nRisk Factors:\n")
		for _, rf := range plan.RiskFactors {
			fmt.Printf("  • %s\n", rf)
		}
	}

	if len(plan.Dependencies) > 0 {
		fmt.Printf("\nDependencies:\n")
		for _, dep := range plan.Dependencies {
			fmt.Printf("  • %s\n", dep)
		}
	}

	if len(plan.FilesToCreate) > 0 {
		fmt.Printf("\nFiles to Create (%d):\n", len(plan.FilesToCreate))
		for _, f := range plan.FilesToCreate {
			fmt.Printf("  • %s\n", f.Path)
		}
	}

	if len(plan.FilesToModify) > 0 {
		fmt.Printf("\nFiles to Modify (%d):\n", len(plan.FilesToModify))
		for _, f := range plan.FilesToModify {
			fmt.Printf("  • %s\n", f.Path)
		}
	}

	if len(plan.Steps) > 0 {
		fmt.Printf("\nSteps (%d):\n", len(plan.Steps))
		for i, step := range plan.Steps {
			fmt.Printf("  %d. %s\n", i+1, step.Description)
			if step.EstimatedTime > 0 {
				fmt.Printf("     (est: %s)\n", step.EstimatedTime)
			}
		}
	}

	if plan.ApprovedBy != "" {
		fmt.Printf("\nApproved by: %s\n", plan.ApprovedBy)
		if plan.ApprovedAt != nil {
			fmt.Printf("Approved at: %s\n", plan.ApprovedAt.Format(time.RFC1123))
		}
	}

	if plan.RejectionReason != "" {
		fmt.Printf("\nRejection: %s\n", plan.RejectionReason)
	}

	if plan.Revision > 0 {
		fmt.Printf("\nRevision: %d\n", plan.Revision)
	}

	if plan.ParentPlanID != "" {
		fmt.Printf("Parent Plan: %s\n", plan.ParentPlanID)
	}

	if len(plan.Feedback) > 0 {
		fmt.Printf("\nFeedback:\n")
		for _, fb := range plan.Feedback {
			fmt.Printf("  • %s\n", fb)
		}
	}

	// Timestamps
	fmt.Printf("\nCreated:    %s\n", plan.CreatedAt.Format(time.RFC1123))
	fmt.Printf("Updated:    %s\n", plan.UpdatedAt.Format(time.RFC1123))
	if plan.CreatedBy != "" {
		fmt.Printf("Created by: %s\n", plan.CreatedBy)
	}

	fmt.Println()
}

// formatPlanStatus returns a formatted plan status string
func formatPlanStatus(status db.PlanStatus) string {
	switch status {
	case db.PlanStatusDraft:
		return "📝 draft"
	case db.PlanStatusPending:
		return "⏳ pending"
	case db.PlanStatusApproved:
		return "✅ approved"
	case db.PlanStatusRejected:
		return "❌ rejected"
	case db.PlanStatusExecuting:
		return "🔄 executing"
	case db.PlanStatusCompleted:
		return "✓ completed"
	case db.PlanStatusFailed:
		return "✗ failed"
	default:
		return string(status)
	}
}

// planReviewCmd launches the TUI for reviewing plans
func planReviewCmd() *cobra.Command {
	var statusFilter string

	command := &cobra.Command{
		Use:   "review",
		Short: "Review and approve implementation plans",
		Long: `Launch a terminal UI (TUI) for reviewing and approving implementation plans.

This allows you to:
- View all pending plans
- Inspect plan details, steps, and risk factors
- Approve or reject plans
- Provide feedback for rejected plans

Example:
  drover plan review`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, store, err := requireProject()
			if err != nil {
				return err
			}
			defer store.Close()

			// Get plans to review
			var plans []*db.Plan
			if statusFilter != "" {
				filtered, err := store.ListPlans(db.PlanStatus(statusFilter))
				if err != nil {
					return fmt.Errorf("listing plans: %w", err)
				}
				plans = filtered
			} else {
				// Default to pending and draft plans
				pending, err := store.ListPlans(db.PlanStatusPending)
				if err != nil {
					return fmt.Errorf("listing pending plans: %w", err)
				}
				drafts, err := store.ListPlans(db.PlanStatusDraft)
				if err != nil {
					return fmt.Errorf("listing draft plans: %w", err)
				}
				plans = append(pending, drafts...)
			}

			if len(plans) == 0 {
				fmt.Println("No plans to review.")
				return nil
			}

			// Launch TUI
			return runPlanReviewTUI(plans, store)
		},
	}

	command.Flags().StringVarP(&statusFilter, "status", "s", "", "Filter by plan status (draft, pending, approved, rejected, etc.)")
	return command
}

// runPlanReviewTUI starts the plan review TUI
func runPlanReviewTUI(plans []*db.Plan, store *db.Store) error {
	// Import TUI package
	p := tui.NewPlanReviewTUI(plans, store)

	// Start bubbletea program
	return tea.NewProgram(p).Start()
}
