// Package telemetry provides OpenTelemetry observability for Drover
package telemetry

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Meter is the global meter for Drover metrics
var meter = otel.Meter("drover")

// Counter instruments
var (
	// Task counters
	tasksClaimedCounter    metric.Int64Counter
	tasksCompletedCounter  metric.Int64Counter
	tasksFailedCounter     metric.Int64Counter
	tasksRetriedCounter    metric.Int64Counter

	// Blocker counters
	blockersDetectedCounter   metric.Int64Counter
	fixTasksCreatedCounter    metric.Int64Counter

	// Agent counters
	agentPromptsCounter       metric.Int64Counter
	agentToolCallsCounter     metric.Int64Counter
	agentErrorsCounter        metric.Int64Counter
)

// Gauge instruments
var (
	tasksActiveGauge        metric.Int64ObservableGauge
	workersActiveGauge      metric.Int64ObservableGauge
	tasksPendingGauge       metric.Int64ObservableGauge
	worktreesActiveGauge    metric.Int64ObservableGauge
)

// Histogram instruments
var (
	taskDurationHistogram       metric.Float64Histogram
	agentDurationHistogram      metric.Float64Histogram
	claimLatencyHistogram       metric.Float64Histogram
	worktreeSetupHistogram      metric.Float64Histogram
)

// initMetrics initializes all metric instruments
// Must be called after Init() has set up the global meter provider
func initMetrics() error {
	var err error

	// Task counters
	if tasksClaimedCounter, err = meter.Int64Counter(
		"drover_tasks_claimed_total",
		metric.WithDescription("Total number of tasks claimed by workers"),
		metric.WithUnit("{task}"),
	); err != nil {
		return err
	}

	if tasksCompletedCounter, err = meter.Int64Counter(
		"drover_tasks_completed_total",
		metric.WithDescription("Total number of tasks completed successfully"),
		metric.WithUnit("{task}"),
	); err != nil {
		return err
	}

	if tasksFailedCounter, err = meter.Int64Counter(
		"drover_tasks_failed_total",
		metric.WithDescription("Total number of tasks that failed"),
		metric.WithUnit("{task}"),
	); err != nil {
		return err
	}

	if tasksRetriedCounter, err = meter.Int64Counter(
		"drover_tasks_retried_total",
		metric.WithDescription("Total number of task retry attempts"),
		metric.WithUnit("{attempt}"),
	); err != nil {
		return err
	}

	// Blocker counters
	if blockersDetectedCounter, err = meter.Int64Counter(
		"drover_blockers_detected_total",
		metric.WithDescription("Total number of blockers detected"),
		metric.WithUnit("{blocker}"),
	); err != nil {
		return err
	}

	if fixTasksCreatedCounter, err = meter.Int64Counter(
		"drover_fix_tasks_created_total",
		metric.WithDescription("Total number of fix tasks created for blockers"),
		metric.WithUnit("{task}"),
	); err != nil {
		return err
	}

	// Agent counters
	if agentPromptsCounter, err = meter.Int64Counter(
		"drover_agent_prompts_total",
		metric.WithDescription("Total number of agent prompts sent"),
		metric.WithUnit("{prompt}"),
	); err != nil {
		return err
	}

	if agentToolCallsCounter, err = meter.Int64Counter(
		"drover_agent_tool_calls_total",
		metric.WithDescription("Total number of agent tool calls"),
		metric.WithUnit("{call}"),
	); err != nil {
		return err
	}

	if agentErrorsCounter, err = meter.Int64Counter(
		"drover_agent_errors_total",
		metric.WithDescription("Total number of agent errors"),
		metric.WithUnit("{error}"),
	); err != nil {
		return err
	}

	// Histograms
	if taskDurationHistogram, err = meter.Float64Histogram(
		"drover_task_duration_seconds",
		metric.WithDescription("Duration of task execution in seconds"),
		metric.WithUnit("s"),
	); err != nil {
		return err
	}

	if agentDurationHistogram, err = meter.Float64Histogram(
		"drover_agent_duration_seconds",
		metric.WithDescription("Duration of agent execution in seconds"),
		metric.WithUnit("s"),
	); err != nil {
		return err
	}

	if claimLatencyHistogram, err = meter.Float64Histogram(
		"drover_claim_latency_seconds",
		metric.WithDescription("Time from task ready to being claimed"),
		metric.WithUnit("s"),
	); err != nil {
		return err
	}

	if worktreeSetupHistogram, err = meter.Float64Histogram(
		"drover_worktree_setup_seconds",
		metric.WithDescription("Time to set up a worktree"),
		metric.WithUnit("s"),
	); err != nil {
		return err
	}

	return nil
}

// InitMetrics initializes metrics. Called automatically by Init().
// Exported for explicit initialization if needed.
func InitMetrics() error {
	return initMetrics()
}

// Task metric recording functions

// RecordTaskClaimed records that a task was claimed
func RecordTaskClaimed(ctx context.Context, workerID, projectID string) {
	if tasksClaimedCounter == nil {
		return
	}
	tasksClaimedCounter.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String(KeyWorkerID, workerID),
			attribute.String(KeyProjectID, projectID),
		),
	)
}

// RecordTaskCompleted records that a task completed successfully
func RecordTaskCompleted(ctx context.Context, workerID, projectID string, duration time.Duration) {
	if tasksCompletedCounter == nil {
		return
	}
	tasksCompletedCounter.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String(KeyWorkerID, workerID),
			attribute.String(KeyProjectID, projectID),
		),
	)
	if taskDurationHistogram != nil {
		taskDurationHistogram.Record(ctx, duration.Seconds(),
			metric.WithAttributes(
				attribute.String(KeyProjectID, projectID),
				attribute.String(KeyTaskState, "completed"),
			),
		)
	}
}

// RecordTaskFailed records that a task failed
func RecordTaskFailed(ctx context.Context, workerID, projectID, errorType string, duration time.Duration) {
	if tasksFailedCounter == nil {
		return
	}
	tasksFailedCounter.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String(KeyWorkerID, workerID),
			attribute.String(KeyProjectID, projectID),
			attribute.String(KeyErrorType, errorType),
		),
	)
	if taskDurationHistogram != nil {
		taskDurationHistogram.Record(ctx, duration.Seconds(),
			metric.WithAttributes(
				attribute.String(KeyProjectID, projectID),
				attribute.String(KeyTaskState, "failed"),
				attribute.String(KeyErrorType, errorType),
			),
		)
	}
}

// RecordTaskRetry records a task retry attempt
func RecordTaskRetry(ctx context.Context, taskID string, attempt int) {
	if tasksRetriedCounter == nil {
		return
	}
	tasksRetriedCounter.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String(KeyTaskID, taskID),
			attribute.Int(KeyTaskAttempt, attempt),
		),
	)
}

// RecordClaimLatency records the time from task ready to being claimed
func RecordClaimLatency(ctx context.Context, projectID string, latency time.Duration) {
	if claimLatencyHistogram == nil {
		return
	}
	claimLatencyHistogram.Record(ctx, latency.Seconds(),
		metric.WithAttributes(attribute.String(KeyProjectID, projectID)),
	)
}

// Blocker metric recording functions

// RecordBlockerDetected records that a blocker was detected
func RecordBlockerDetected(ctx context.Context, blockerType, projectID string) {
	if blockersDetectedCounter == nil {
		return
	}
	blockersDetectedCounter.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String(KeyBlockerType, blockerType),
			attribute.String(KeyProjectID, projectID),
		),
	)
}

// RecordFixTaskCreated records that a fix task was created for a blocker
func RecordFixTaskCreated(ctx context.Context, blockerType, projectID string) {
	if fixTasksCreatedCounter == nil {
		return
	}
	fixTasksCreatedCounter.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String(KeyBlockerType, blockerType),
			attribute.String(KeyProjectID, projectID),
		),
	)
}

// Agent metric recording functions

// RecordAgentPrompt records an agent prompt
func RecordAgentPrompt(ctx context.Context, agentType string) {
	if agentPromptsCounter == nil {
		return
	}
	agentPromptsCounter.Add(ctx, 1,
		metric.WithAttributes(attribute.String(KeyAgentType, agentType)),
	)
}

// RecordAgentToolCall records an agent tool call
func RecordAgentToolCall(ctx context.Context, agentType, toolName string) {
	if agentToolCallsCounter == nil {
		return
	}
	agentToolCallsCounter.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String(KeyAgentType, agentType),
			attribute.String("drover.agent.tool", toolName),
		),
	)
}

// RecordAgentError records an agent error
func RecordAgentError(ctx context.Context, agentType, errorType string) {
	if agentErrorsCounter == nil {
		return
	}
	agentErrorsCounter.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String(KeyAgentType, agentType),
			attribute.String(KeyErrorType, errorType),
		),
	)
}

// RecordAgentDuration records the duration of agent execution
func RecordAgentDuration(ctx context.Context, agentType string, duration time.Duration) {
	if agentDurationHistogram == nil {
		return
	}
	agentDurationHistogram.Record(ctx, duration.Seconds(),
		metric.WithAttributes(attribute.String(KeyAgentType, agentType)),
	)
}

// Worktree metric recording functions

// RecordWorktreeSetup records the time to set up a worktree
func RecordWorktreeSetup(ctx context.Context, duration time.Duration) {
	if worktreeSetupHistogram == nil {
		return
	}
	worktreeSetupHistogram.Record(ctx, duration.Seconds())
}

// Gauges are typically set via callbacks in production.
// These functions provide a simpler interface for Drover.
