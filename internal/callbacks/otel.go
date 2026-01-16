package callbacks

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	// InstrumentationName is the OpenTelemetry instrumentation name
	InstrumentationName = "github.com/cloud-shuttle/drover"
	// TracerName is the tracer name for Drover
	TracerName = "drover-tracer"

	// Span names
	SpanTaskCreated    = "task.created"
	SpanTaskAssigned   = "task.assigned"
	SpanTaskStarted    = "task.started"
	SpanTaskCompleted  = "task.completed"
	SpanTaskFailed     = "task.failed"
	SpanTaskBlocked    = "task.blocked"
	SpanTaskUnblocked  = "task.unblocked"
	SpanTaskRecovered  = "task.recovered"
	SpanWorkerStarted  = "worker.started"
	SpanWorkerStopped  = "worker.stopped"
	SpanWorkerStalled  = "worker.stalled"
)

// OTelCallback implements Callback with OpenTelemetry tracing.
// It creates spans and propagates trace context through task execution.
//
// This is a stub implementation that provides the basic structure for
// OpenTelemetry integration. A full implementation would:
// - Use context propagation for distributed tracing
// - Add more detailed span attributes
// - Handle span links for async operations
// - Integrate with metrics and logs
type OTelCallback struct {
	tracer trace.Tracer
	logger *log.Logger
}

// NewOTelCallback creates a new OpenTelemetry callback
func NewOTelCallback() *OTelCallback {
	tracer := otel.Tracer(TracerName)

	return &OTelCallback{
		tracer: tracer,
	}
}

// SetLogger sets the logger for the OTel callback
func (o *OTelCallback) SetLogger(logger *log.Logger) {
	o.logger = logger
}

// taskAttrs converts TaskEventContext to OpenTelemetry attributes
func taskAttrs(ctx *TaskEventContext) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("drover.task.id", ctx.TaskID),
		attribute.String("drover.task.title", ctx.Title),
		attribute.String("drover.task.epic_id", ctx.EpicID),
		attribute.String("drover.task.type", ctx.Type),
		attribute.Int("drover.task.priority", ctx.Priority),
		attribute.Int("drover.task.attempt", ctx.Attempt),
		attribute.Int("drover.task.max_attempts", ctx.MaxAttempts),
	}

	if ctx.PrevState != "" {
		attrs = append(attrs, attribute.String("drover.task.prev_state", ctx.PrevState))
	}

	if ctx.NewState != "" {
		attrs = append(attrs, attribute.String("drover.task.new_state", ctx.NewState))
	}

	if ctx.Transition != "" {
		attrs = append(attrs, attribute.String("drover.task.transition", ctx.Transition))
	}

	if ctx.WorkerID != "" {
		attrs = append(attrs, attribute.String("drover.task.worker_id", ctx.WorkerID))
	}

	if ctx.Duration != nil {
		attrs = append(attrs, attribute.Int64("drover.task.duration_ms", ctx.Duration.Milliseconds()))
	}

	if ctx.Error != "" {
		attrs = append(attrs,
			attribute.String("drover.task.error", ctx.Error),
			attribute.String("drover.task.error_type", ctx.ErrorType),
			attribute.String("drover.task.error_category", ctx.ErrorCategory),
		)
	}

	// Add custom metadata
	for k, v := range ctx.Metadata {
		attrs = append(attrs, attribute.String(fmt.Sprintf("drover.task.metadata.%s", k), v))
	}

	return attrs
}

// workerAttrs converts WorkerEventContext to OpenTelemetry attributes
func workerAttrs(ctx *WorkerEventContext) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("drover.worker.id", ctx.WorkerID),
		attribute.Int("drover.worker.index", ctx.WorkerIndex),
		attribute.String("drover.worker.state", ctx.State),
	}

	if ctx.CurrentTaskID != "" {
		attrs = append(attrs, attribute.String("drover.worker.current_task_id", ctx.CurrentTaskID))
	}

	if ctx.StallReason != "" {
		attrs = append(attrs,
			attribute.String("drover.worker.stall_reason", ctx.StallReason),
			attribute.Int64("drover.worker.stall_duration_ms", ctx.StallDuration.Milliseconds()),
		)
	}

	// Add custom metadata
	for k, v := range ctx.Metadata {
		attrs = append(attrs, attribute.String(fmt.Sprintf("drover.worker.metadata.%s", k), v))
	}

	return attrs
}

// recoveryAttrs converts RecoveryEventContext to OpenTelemetry attributes
func recoveryAttrs(ctx *RecoveryEventContext) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("drover.task.id", ctx.TaskID),
		attribute.String("drover.task.title", ctx.Title),
		attribute.String("drover.recovery.strategy", ctx.Strategy),
		attribute.String("drover.recovery.reason", ctx.Reason),
		attribute.Float64("drover.recovery.confidence", ctx.Confidence),
		attribute.Int("drover.recovery.attempt_count", ctx.AttemptCount),
	}

	if ctx.OriginalError != "" {
		attrs = append(attrs, attribute.String("drover.recovery.original_error", ctx.OriginalError))
	}

	// Add custom metadata
	for k, v := range ctx.Metadata {
		attrs = append(attrs, attribute.String(fmt.Sprintf("drover.recovery.metadata.%s", k), v))
	}

	return attrs
}

// startSpan starts a new span with the given name and attributes
func (o *OTelCallback) startSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return o.tracer.Start(ctx, name, trace.WithAttributes(attrs...), trace.WithTimestamp(time.Now()))
}

// Task lifecycle callbacks with span creation

// OnTaskCreated implements Callback
func (o *OTelCallback) OnTaskCreated(ctx *TaskEventContext) error {
	_, span := o.startSpan(context.Background(), SpanTaskCreated, taskAttrs(ctx)...)
	span.End()
	return nil
}

// OnTaskAssigned implements Callback
func (o *OTelCallback) OnTaskAssigned(ctx *TaskEventContext) error {
	_, span := o.startSpan(context.Background(), SpanTaskAssigned, taskAttrs(ctx)...)
	span.End()
	return nil
}

// OnTaskStarted implements Callback
func (o *OTelCallback) OnTaskStarted(ctx *TaskEventContext) error {
	_, span := o.startSpan(context.Background(), SpanTaskStarted, taskAttrs(ctx)...)
	span.End()
	return nil
}

// OnTaskCompleted implements Callback
func (o *OTelCallback) OnTaskCompleted(ctx *TaskEventContext) error {
	_, span := o.startSpan(context.Background(), SpanTaskCompleted, taskAttrs(ctx)...)
	span.SetStatus(codes.Ok, "task completed")
	span.End()
	return nil
}

// OnTaskFailed implements Callback
func (o *OTelCallback) OnTaskFailed(ctx *TaskEventContext) error {
	attrs := taskAttrs(ctx)
	_, span := o.startSpan(context.Background(), SpanTaskFailed, attrs...)

	// Set span status with error
	if ctx.Error != "" {
		span.SetStatus(codes.Error, ctx.Error)
		span.RecordError(errors.New(ctx.Error))
	}

	span.End()
	return nil
}

// OnTaskBlocked implements Callback
func (o *OTelCallback) OnTaskBlocked(ctx *TaskEventContext) error {
	_, span := o.startSpan(context.Background(), SpanTaskBlocked, taskAttrs(ctx)...)
	span.End()
	return nil
}

// OnTaskUnblocked implements Callback
func (o *OTelCallback) OnTaskUnblocked(ctx *TaskEventContext) error {
	_, span := o.startSpan(context.Background(), SpanTaskUnblocked, taskAttrs(ctx)...)
	span.End()
	return nil
}

// OnTaskRecovered implements Callback
func (o *OTelCallback) OnTaskRecovered(ctx *RecoveryEventContext) error {
	_, span := o.startSpan(context.Background(), SpanTaskRecovered, recoveryAttrs(ctx)...)
	span.SetStatus(codes.Ok, fmt.Sprintf("recovered using %s strategy", ctx.Strategy))
	span.End()
	return nil
}

// Worker lifecycle callbacks with span creation

// OnWorkerStarted implements Callback
func (o *OTelCallback) OnWorkerStarted(ctx *WorkerEventContext) error {
	_, span := o.startSpan(context.Background(), SpanWorkerStarted, workerAttrs(ctx)...)
	span.End()
	return nil
}

// OnWorkerStopped implements Callback
func (o *OTelCallback) OnWorkerStopped(ctx *WorkerEventContext) error {
	_, span := o.startSpan(context.Background(), SpanWorkerStopped, workerAttrs(ctx)...)
	span.End()
	return nil
}

// OnWorkerStalled implements Callback
func (o *OTelCallback) OnWorkerStalled(ctx *WorkerEventContext) error {
	_, span := o.startSpan(context.Background(), SpanWorkerStalled, workerAttrs(ctx)...)
	span.SetStatus(codes.Error, fmt.Sprintf("worker stalled: %s", ctx.StallReason))
	span.End()
	return nil
}

// ContextKey is used for storing trace context in task execution context
type ContextKey struct{}

// GetSpanFromContext extracts the current span from context if available
func GetSpanFromContext(ctx context.Context) trace.Span {
	if span := trace.SpanFromContext(ctx); span != nil {
		return span
	}
	return trace.SpanFromContext(context.WithValue(ctx, ContextKey{}, nil))
}

// ContextWithSpan returns a context with the given span
func ContextWithSpan(ctx context.Context, span trace.Span) context.Context {
	return trace.ContextWithSpan(ctx, span)
}
