// Package telemetry provides OpenTelemetry observability for Drover
package telemetry

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// tracer is the global tracer for Drover
var tracer = otel.Tracer("drover")

// Span names for Drover operations
const (
	// Workflow spans
	SpanWorkflowRun        = "drover.workflow.run"
	SpanWorkflowSequential = "drover.workflow.sequential"
	SpanWorkflowParallel   = "drover.workflow.parallel"

	// Task spans
	SpanTaskClaim    = "drover.task.claim"
	SpanTaskExecute  = "drover.task.execute"
	SpanTaskComplete = "drover.task.complete"
	SpanTaskFail     = "drover.task.fail"
	SpanTaskRetry    = "drover.task.retry"

	// Worker spans
	SpanWorkerRun    = "drover.worker.run"
	SpanWorkerPoll   = "drover.worker.poll"
	SpanWorkerLoop   = "drover.worker.loop"

	// Worktree spans
	SpanWorktreeCreate  = "drover.worktree.create"
	SpanWorktreeSetup   = "drover.worktree.setup"
	SpanWorktreeCleanup = "drover.worktree.cleanup"

	// Agent spans
	SpanAgentExecute     = "drover.agent.execute"
	SpanAgentPrompt      = "drover.agent.prompt"
	SpanAgentToolCall    = "drover.agent.tool_call"

	// Blocker spans
	SpanBlockerDetect    = "drover.blocker.detect"
	SpanBlockerAnalyze   = "drover.blocker.analyze"
	SpanBlockerCreateFix = "drover.blocker.create_fix_task"

	// Git spans
	SpanGitCommit    = "drover.git.commit"
	SpanGitPush      = "drover.git.push"
	SpanGitMerge     = "drover.git.merge"
)

// StartWorkflowSpan starts a span for workflow execution
func StartWorkflowSpan(ctx context.Context, workflowType, workflowID string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	attrs = append(attrs,
		attribute.String(KeyWorkflowType, workflowType),
		attribute.String(KeyWorkflowID, workflowID),
	)
	return tracer.Start(ctx, SpanWorkflowRun, trace.WithAttributes(attrs...))
}

// StartTaskSpan starts a span for a task operation with task attributes
func StartTaskSpan(ctx context.Context, name string, taskAttrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return tracer.Start(ctx, name, trace.WithAttributes(taskAttrs...))
}

// StartWorkerSpan starts a span for a worker operation
func StartWorkerSpan(ctx context.Context, name string, workerID string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	attrs = append(attrs, attribute.String(KeyWorkerID, workerID))
	return tracer.Start(ctx, name, trace.WithAttributes(attrs...))
}

// StartAgentSpan starts a span for agent execution
func StartAgentSpan(ctx context.Context, agentType, model string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	attrs = append(attrs,
		attribute.String(KeyAgentType, agentType),
		attribute.String(KeyAgentModel, model),
	)
	return tracer.Start(ctx, SpanAgentExecute, trace.WithAttributes(attrs...))
}

// StartWorktreeSpan starts a span for worktree operations
func StartWorktreeSpan(ctx context.Context, name, worktreePath string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	attrs = append(attrs, attribute.String(KeyWorktreePath, worktreePath))
	return tracer.Start(ctx, name, trace.WithAttributes(attrs...))
}

// RecordError records an error on a span with optional error type/category
func RecordError(span trace.Span, err error, errorType, errorCategory string) {
	if err == nil {
		return
	}

	attrs := []attribute.KeyValue{
		attribute.String("exception.message", err.Error()),
		attribute.String("exception.type", errorType),
	}

	if errorCategory != "" {
		attrs = append(attrs, attribute.String(KeyErrorCategory, errorCategory))
	}

	span.RecordError(err, trace.WithAttributes(attrs...))
	span.SetStatus(codes.Error, err.Error())
}

// RecordErrorWithStatus records an error and sets span status
func RecordErrorWithStatus(span trace.Span, err error, errorType, errorCategory string) {
	if err == nil {
		span.SetStatus(codes.Ok, "")
		return
	}

	RecordError(span, err, errorType, errorCategory)
}

// SetTaskStatus sets the task status as a span attribute
func SetTaskStatus(span trace.Span, status string) {
	span.SetAttributes(attribute.String(KeyTaskState, status))
}

// SetBlockerInfo sets blocker-related attributes on a span
func SetBlockerInfo(span trace.Span, blockerType, blockerTaskID, reason string) {
	span.SetAttributes(
		attribute.String(KeyBlockerType, blockerType),
		attribute.String(KeyBlockerTaskID, blockerTaskID),
		attribute.String(KeyBlockerReason, reason),
	)
}

// AddProjectAttrs adds project attributes to a span
func AddProjectAttrs(span trace.Span, projectID, path, name string) {
	span.SetAttributes(
		attribute.String(KeyProjectID, projectID),
		attribute.String(KeyProjectPath, path),
		attribute.String(KeyProjectName, name),
	)
}

// AddEpicAttrs adds epic attributes to a span
func AddEpicAttrs(span trace.Span, epicID string) {
	span.SetAttributes(attribute.String(KeyEpicID, epicID))
}

// WithTaskID creates an option to add task ID to span attributes
func WithTaskID(taskID string) trace.SpanStartEventOption {
	return trace.WithAttributes(attribute.String(KeyTaskID, taskID))
}

// WithWorkerID creates an option to add worker ID to span attributes
func WithWorkerID(workerID string) trace.SpanStartEventOption {
	return trace.WithAttributes(attribute.String(KeyWorkerID, workerID))
}

// WithEpicID creates an option to add epic ID to span attributes
func WithEpicID(epicID string) trace.SpanStartEventOption {
	return trace.WithAttributes(attribute.String(KeyEpicID, epicID))
}

// GetTraceID returns the trace ID from context if available
func GetTraceID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		return span.SpanContext().TraceID().String()
	}
	return ""
}

// GetSpanID returns the span ID from context if available
func GetSpanID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		return span.SpanContext().SpanID().String()
	}
	return ""
}

// TraceIDFromString extracts trace ID from a string for logging/correlation
func TraceIDFromString(traceID string) string {
	return traceID
}

// ContextWithTraceID returns a context with trace information for logging
func ContextWithTraceID(ctx context.Context) context.Context {
	traceID := GetTraceID(ctx)
	if traceID != "" {
		// Store trace ID in context for log correlation
		// This can be retrieved by logging frameworks
		return context.WithValue(ctx, "trace_id", traceID)
	}
	return ctx
}

// ErrorTypeFromError extracts a human-readable error type
func ErrorTypeFromError(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%T", err)
}
