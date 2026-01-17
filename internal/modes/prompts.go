// Package modes provides default prompts for different worker modes
package modes

// DefaultPlanningPrompt returns the default prompt for planning mode
func DefaultPlanningPrompt() string {
	return "You are in PLANNING MODE. Your task is to create a detailed implementation plan WITHOUT making any code changes.\n\n" +
		"## Your Objective\n" +
		"Create a comprehensive, step-by-step implementation plan for the following task:\n\n" +
		"{{.TaskTitle}}\n" +
		"{{if .TaskDescription}}Description: {{.TaskDescription}}{{end}}\n" +
		"{{if .EpicID}}Epic Context: This task is part of epic {{.EpicID}}{{end}}\n\n" +
		"## Requirements for Your Plan\n\n" +
		"1. **Analyze the Request**: Understand what needs to be built\n" +
		"2. **Break Down Steps**: Create logical, sequential steps (max {{.MaxSteps}} steps)\n" +
		"3. **Identify Files**: List all files to create/modify with reasons\n" +
		"4. **Estimate Complexity**: Rate as low/medium/high\n" +
		"5. **Identify Risks**: List potential issues or dependencies\n" +
		"6. **Estimate Time**: Provide time estimates for each step\n\n" +
		"## Output Format\n\n" +
		"Respond with a structured plan in this format:\n\n" +
		"# Implementation Plan: [Brief Title]\n\n" +
		"## Overview\n" +
		"[2-3 sentence summary of what will be built]\n\n" +
		"## Steps\n" +
		"1. [Step Title]\n" +
		"   - Description: [What this step does]\n" +
		"   - Files: [file1.ts, file2.ts]\n" +
		"   - Dependencies: [step numbers this depends on]\n" +
		"   - Verification: [How to verify completion]\n" +
		"   - Estimated Time: [e.g., 5 minutes]\n\n" +
		"2. [Step Title]\n" +
		"   [... continue for all steps ...]\n\n" +
		"## Files to Create\n" +
		"- `path/to/file.ext`: [Reason for creation]\n\n" +
		"## Files to Modify\n" +
		"- `path/to/file.ext`: [What changes and why]\n\n" +
		"## Risk Factors\n" +
		"- [Potential risk 1]\n" +
		"- [Potential risk 2]\n\n" +
		"## Complexity\n" +
		"[low/medium/high] - [Brief justification]\n\n" +
		"## Estimated Total Time\n" +
		"[Total time estimate]\n\n" +
		"## Important Constraints\n\n" +
		"- DO NOT make any code changes\n" +
		"- DO NOT use file editing tools\n" +
		"- DO NOT run build or test commands\n" +
		"- Focus on thorough analysis and clear documentation\n" +
		"- Consider edge cases and error handling\n" +
		"- Think about testing and validation\n\n" +
		"Begin your planning analysis now."
}

// DefaultBuildingPrompt returns the default prompt for building mode
func DefaultBuildingPrompt() string {
	return "You are in BUILDING MODE. Your task is to execute an approved implementation plan.\n\n" +
		"## Your Objective\n" +
		"Execute the following approved plan to complete the task:\n\n" +
		"{{.TaskTitle}}\n" +
		"{{if .TaskDescription}}Description: {{.TaskDescription}}{{end}}\n\n" +
		"## Approved Plan\n\n" +
		"### Overview\n" +
		"{{.PlanOverview}}\n\n" +
		"### Steps to Execute\n" +
		"{{.PlanSteps}}\n\n" +
		"## Execution Guidelines\n\n" +
		"1. **Follow the Plan**: Execute steps in the specified order\n" +
		"2. **Make Changes**: Use file editing tools to make code changes\n" +
		"3. **Verify Each Step**: After each step, verify it works as expected\n" +
		"4. **Report Progress**: Clearly indicate which step you're working on\n" +
		"5. **Handle Errors**: If something fails, report the error and suggest fixes\n\n" +
		"## How to Report Progress\n\n" +
		"Use this format for each step:\n\n" +
		"Step [N]/[Total]: [Step Title]\n" +
		"Completed / Failed / In Progress\n\n" +
		"[Details about what you did]\n\n" +
		"## Important Constraints\n\n" +
		"- ONLY make changes specified in the approved plan\n" +
		"- If you encounter issues not covered in the plan, STOP and report\n" +
		"- Use appropriate file editing tools\n" +
		"- Run tests if specified in verification steps\n" +
		"- Commit frequently with clear messages\n\n" +
		"Begin executing the plan now."
}

// DefaultRefinementPrompt returns the default prompt for plan refinement
func DefaultRefinementPrompt() string {
	return "You are in PLAN REFINEMENT MODE. Your task is to improve a failed or rejected implementation plan.\n\n" +
		"## Original Plan (Failed/Rejected)\n\n" +
		"### Plan Overview\n" +
		"{{.PlanOverview}}\n\n" +
		"### Steps\n" +
		"{{.PlanSteps}}\n\n" +
		"## Failure/Rejection Reason\n" +
		"{{.FailureReason}}\n\n" +
		"## Your Task\n\n" +
		"Create a refined plan that addresses the failure. Consider:\n\n" +
		"1. **Root Cause Analysis**: Why did the original plan fail?\n" +
		"2. **Alternative Approaches**: Are there better ways to achieve the goal?\n" +
		"3. **Missing Steps**: Did we overlook something important?\n" +
		"4. **Risk Mitigation**: How can we avoid similar failures?\n" +
		"5. **Dependency Issues**: Are there external factors we need to handle?\n\n" +
		"## Output Format\n\n" +
		"# Refined Implementation Plan\n\n" +
		"## What Changed\n" +
		"[Brief explanation of what's different from the original plan]\n\n" +
		"## Root Cause of Previous Failure\n" +
		"[Analysis of why the first plan failed]\n\n" +
		"## Revised Steps\n" +
		"1. [Step Title]\n" +
		"   - Description: [What this step does]\n" +
		"   - Files: [file1.ts, file2.ts]\n" +
		"   - Dependencies: [step numbers this depends on]\n" +
		"   - Verification: [How to verify completion]\n" +
		"   - Estimated Time: [e.g., 5 minutes]\n\n" +
		"[... continue for all revised steps ...]\n\n" +
		"## New Risk Factors\n" +
		"- [Any new risks identified]\n\n" +
		"## Complexity\n" +
		"[low/medium/high]\n\n" +
		"## Estimated Total Time\n" +
		"[Total time estimate]\n\n" +
		"Begin your refinement analysis now."
}
