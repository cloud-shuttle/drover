package telemetry

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// ============================================================================
// Test Helpers
// ============================================================================

// setupTestTracer installs an in-memory span recorder and returns it.
func setupTestTracer(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	tracer = tp.Tracer("drover-test")
	t.Cleanup(func() { tp.Shutdown(context.Background()) })
	return sr
}

// setupTestMeter installs an in-memory metric reader and returns it.
func setupTestMeter(t *testing.T) *sdkmetric.ManualReader {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	meter = mp.Meter("drover-test")
	if err := initMetrics(); err != nil {
		t.Fatalf("initMetrics: %v", err)
	}
	t.Cleanup(func() { mp.Shutdown(context.Background()) })
	return reader
}

func findAttr(attrs []attribute.KeyValue, key string) (attribute.Value, bool) {
	for _, a := range attrs {
		if string(a.Key) == key {
			return a.Value, true
		}
	}
	return attribute.Value{}, false
}

// ============================================================================
// Config & Init
// ============================================================================

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.ServiceName != DefaultServiceName {
		t.Errorf("expected service name %q, got %q", DefaultServiceName, cfg.ServiceName)
	}
	if cfg.SampleRate != 1.0 {
		t.Errorf("expected sample rate 1.0, got %f", cfg.SampleRate)
	}
	if cfg.Enabled {
		t.Error("telemetry should be disabled by default")
	}
}

func TestDefaultConfig_ProductionSampleRate(t *testing.T) {
	t.Setenv("DROVER_ENV", "production")
	cfg := DefaultConfig()
	if cfg.SampleRate != 0.1 {
		t.Errorf("expected production sample rate 0.1, got %f", cfg.SampleRate)
	}
}

func TestInit_Disabled(t *testing.T) {
	cfg := &Config{Enabled: false}
	shutdown, err := Init(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Init error: %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}
}

func TestInit_NilConfig(t *testing.T) {
	// With no env vars, defaults to disabled → returns no-op shutdown
	shutdown, err := Init(context.Background(), nil)
	if err != nil {
		t.Fatalf("Init(nil) error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("expected non-nil shutdown function")
	}
}

func TestMustInit_Disabled(t *testing.T) {
	cfg := &Config{Enabled: false}
	shutdown := MustInit(context.Background(), cfg)
	if shutdown == nil {
		t.Fatal("expected non-nil shutdown")
	}
}

func TestGetEnvironment_Defaults(t *testing.T) {
	env := getEnvironment()
	if env != "development" {
		t.Errorf("expected 'development', got %q", env)
	}
}

func TestGetEnvironment_DroverEnv(t *testing.T) {
	t.Setenv("DROVER_ENV", "staging")
	if env := getEnvironment(); env != "staging" {
		t.Errorf("expected 'staging', got %q", env)
	}
}

func TestGetEnvironment_FallbackEnv(t *testing.T) {
	t.Setenv("ENVIRONMENT", "qa")
	if env := getEnvironment(); env != "qa" {
		t.Errorf("expected 'qa', got %q", env)
	}
}

func TestGetOTLPEndpoint_Default(t *testing.T) {
	if ep := getOTLPEndpoint(); ep != DefaultOTLPEndpoint {
		t.Errorf("expected %q, got %q", DefaultOTLPEndpoint, ep)
	}
}

func TestGetOTLPEndpoint_Custom(t *testing.T) {
	t.Setenv(EnvOTLPEndpoint, "collector:4317")
	if ep := getOTLPEndpoint(); ep != "collector:4317" {
		t.Errorf("expected 'collector:4317', got %q", ep)
	}
}

func TestIsEnabled(t *testing.T) {
	tests := []struct {
		val  string
		want bool
	}{
		{"", false},
		{"true", true},
		{"1", true},
		{"false", false},
		{"0", false},
	}
	for _, tt := range tests {
		t.Run(tt.val, func(t *testing.T) {
			t.Setenv(EnvOTelEnabled, tt.val)
			if got := isEnabled(); got != tt.want {
				t.Errorf("isEnabled(%q) = %v, want %v", tt.val, got, tt.want)
			}
		})
	}
}

// ============================================================================
// Attributes
// ============================================================================

func TestTaskAttrs(t *testing.T) {
	attrs := TaskAttrs("t-1", "Fix bug", "ready", "agent", 2, 1)
	if len(attrs) != 6 {
		t.Fatalf("expected 6 attrs, got %d", len(attrs))
	}
	if v, ok := findAttr(attrs, KeyTaskID); !ok || v.AsString() != "t-1" {
		t.Errorf("expected task ID 't-1', got %v", v)
	}
	if v, ok := findAttr(attrs, KeyTaskPriority); !ok || v.AsInt64() != 2 {
		t.Errorf("expected priority 2, got %v", v)
	}
}

func TestWorkerAttrs(t *testing.T) {
	attrs := WorkerAttrs("w-42")
	if len(attrs) != 1 {
		t.Fatalf("expected 1 attr, got %d", len(attrs))
	}
	if v, _ := findAttr(attrs, KeyWorkerID); v.AsString() != "w-42" {
		t.Errorf("expected 'w-42', got %v", v)
	}
}

func TestProjectAttrs(t *testing.T) {
	attrs := ProjectAttrs("p-1", "/code", "drover")
	if len(attrs) != 3 {
		t.Fatalf("expected 3 attrs, got %d", len(attrs))
	}
	if v, _ := findAttr(attrs, KeyProjectName); v.AsString() != "drover" {
		t.Errorf("expected 'drover', got %v", v)
	}
}

func TestEpicAttrs(t *testing.T) {
	attrs := EpicAttrs("epic-99")
	if v, _ := findAttr(attrs, KeyEpicID); v.AsString() != "epic-99" {
		t.Errorf("expected 'epic-99', got %v", v)
	}
}

// ============================================================================
// Tracer — Span Creation
// ============================================================================

func TestStartWorkflowSpan(t *testing.T) {
	sr := setupTestTracer(t)
	ctx, span := StartWorkflowSpan(context.Background(), WorkflowTypeParallel, "wf-1")
	span.End()
	_ = ctx

	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	s := spans[0]
	if s.Name() != SpanWorkflowRun {
		t.Errorf("expected span name %q, got %q", SpanWorkflowRun, s.Name())
	}
	if v, ok := findAttr(s.Attributes(), KeyWorkflowType); !ok || v.AsString() != WorkflowTypeParallel {
		t.Error("missing or wrong workflow type attribute")
	}
}

func TestStartTaskSpan(t *testing.T) {
	sr := setupTestTracer(t)
	_, span := StartTaskSpan(context.Background(), SpanTaskExecute, attribute.String(KeyTaskID, "t-1"))
	span.End()

	spans := sr.Ended()
	if len(spans) != 1 || spans[0].Name() != SpanTaskExecute {
		t.Error("unexpected task span")
	}
}

func TestStartWorkerSpan(t *testing.T) {
	sr := setupTestTracer(t)
	_, span := StartWorkerSpan(context.Background(), SpanWorkerRun, "w-1")
	span.End()

	if v, ok := findAttr(sr.Ended()[0].Attributes(), KeyWorkerID); !ok || v.AsString() != "w-1" {
		t.Error("missing worker ID attribute")
	}
}

func TestStartAgentSpan(t *testing.T) {
	sr := setupTestTracer(t)
	_, span := StartAgentSpan(context.Background(), AgentTypeClaudeCode, "claude-4")
	span.End()

	s := sr.Ended()[0]
	if s.Name() != SpanAgentExecute {
		t.Errorf("expected %q, got %q", SpanAgentExecute, s.Name())
	}
}

func TestStartWorktreeSpan(t *testing.T) {
	sr := setupTestTracer(t)
	_, span := StartWorktreeSpan(context.Background(), SpanWorktreeCreate, "/tmp/wt")
	span.End()

	if v, ok := findAttr(sr.Ended()[0].Attributes(), KeyWorktreePath); !ok || v.AsString() != "/tmp/wt" {
		t.Error("missing worktree path attribute")
	}
}

// ============================================================================
// Tracer — Error Recording
// ============================================================================

func TestRecordError_NilError(t *testing.T) {
	sr := setupTestTracer(t)
	_, span := tracer.Start(context.Background(), "test")
	RecordError(span, nil, "type", "cat")
	span.End()
	// Should not record any events
	if len(sr.Ended()[0].Events()) != 0 {
		t.Error("expected no events for nil error")
	}
}

func TestRecordError_WithError(t *testing.T) {
	sr := setupTestTracer(t)
	_, span := tracer.Start(context.Background(), "test")
	RecordError(span, errors.New("boom"), "RuntimeError", ErrorCategoryAgent)
	span.End()

	s := sr.Ended()[0]
	if s.Status().Code != codes.Error {
		t.Error("expected Error status code")
	}
	if len(s.Events()) == 0 {
		t.Error("expected error event recorded")
	}
}

func TestRecordErrorWithStatus_NilSetsOk(t *testing.T) {
	sr := setupTestTracer(t)
	_, span := tracer.Start(context.Background(), "test")
	RecordErrorWithStatus(span, nil, "", "")
	span.End()

	if sr.Ended()[0].Status().Code != codes.Ok {
		t.Error("expected Ok status for nil error")
	}
}

// ============================================================================
// Tracer — Span Attribute Helpers
// ============================================================================

func TestSetTaskStatus(t *testing.T) {
	sr := setupTestTracer(t)
	_, span := tracer.Start(context.Background(), "test")
	SetTaskStatus(span, "in_progress")
	span.End()
	if v, ok := findAttr(sr.Ended()[0].Attributes(), KeyTaskState); !ok || v.AsString() != "in_progress" {
		t.Error("task state not set")
	}
}

func TestSetBlockerInfo(t *testing.T) {
	sr := setupTestTracer(t)
	_, span := tracer.Start(context.Background(), "test")
	SetBlockerInfo(span, "merge_conflict", "t-5", "conflicting files")
	span.End()

	s := sr.Ended()[0]
	if v, _ := findAttr(s.Attributes(), KeyBlockerType); v.AsString() != "merge_conflict" {
		t.Error("blocker type not set")
	}
	if v, _ := findAttr(s.Attributes(), KeyBlockerTaskID); v.AsString() != "t-5" {
		t.Error("blocker task ID not set")
	}
}

func TestAddProjectAttrs(t *testing.T) {
	sr := setupTestTracer(t)
	_, span := tracer.Start(context.Background(), "test")
	AddProjectAttrs(span, "p1", "/path", "proj")
	span.End()
	if v, _ := findAttr(sr.Ended()[0].Attributes(), KeyProjectName); v.AsString() != "proj" {
		t.Error("project name not set")
	}
}

func TestAddEpicAttrs(t *testing.T) {
	sr := setupTestTracer(t)
	_, span := tracer.Start(context.Background(), "test")
	AddEpicAttrs(span, "e-1")
	span.End()
	if v, _ := findAttr(sr.Ended()[0].Attributes(), KeyEpicID); v.AsString() != "e-1" {
		t.Error("epic ID not set")
	}
}

// ============================================================================
// Tracer — Context Helpers
// ============================================================================

func TestGetTraceID_NoSpan(t *testing.T) {
	id := GetTraceID(context.Background())
	if id != "" {
		t.Errorf("expected empty trace ID, got %q", id)
	}
}

func TestGetTraceID_WithSpan(t *testing.T) {
	setupTestTracer(t)
	ctx, span := tracer.Start(context.Background(), "test")
	defer span.End()
	id := GetTraceID(ctx)
	if id == "" {
		t.Error("expected non-empty trace ID")
	}
}

func TestGetSpanID_NoSpan(t *testing.T) {
	if id := GetSpanID(context.Background()); id != "" {
		t.Errorf("expected empty span ID, got %q", id)
	}
}

func TestGetSpanID_WithSpan(t *testing.T) {
	setupTestTracer(t)
	ctx, span := tracer.Start(context.Background(), "test")
	defer span.End()
	if id := GetSpanID(ctx); id == "" {
		t.Error("expected non-empty span ID")
	}
}

func TestErrorTypeFromError(t *testing.T) {
	if et := ErrorTypeFromError(nil); et != "" {
		t.Errorf("expected empty for nil, got %q", et)
	}
	et := ErrorTypeFromError(errors.New("x"))
	if et != "*errors.errorString" {
		t.Errorf("expected '*errors.errorString', got %q", et)
	}
}

func TestTraceIDFromString(t *testing.T) {
	if TraceIDFromString("abc") != "abc" {
		t.Error("expected passthrough")
	}
}

func TestContextWithTraceID_NoTrace(t *testing.T) {
	ctx := ContextWithTraceID(context.Background())
	if ctx.Value("trace_id") != nil {
		t.Error("expected no trace_id value")
	}
}

func TestContextWithTraceID_WithTrace(t *testing.T) {
	setupTestTracer(t)
	ctx, span := tracer.Start(context.Background(), "test")
	defer span.End()
	ctx = ContextWithTraceID(ctx)
	if ctx.Value("trace_id") == nil {
		t.Error("expected trace_id in context")
	}
}

// ============================================================================
// Metrics — Init & Recording
// ============================================================================

func TestInitMetrics(t *testing.T) {
	reader := setupTestMeter(t)
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	// Just verify initialization succeeded without error
}

func TestRecordTaskClaimed(t *testing.T) {
	reader := setupTestMeter(t)
	RecordTaskClaimed(context.Background(), "w1", "p1")
	var rm metricdata.ResourceMetrics
	reader.Collect(context.Background(), &rm)
	assertHasMetric(t, rm, "drover_tasks_claimed_total")
}

func TestRecordTaskCompleted(t *testing.T) {
	reader := setupTestMeter(t)
	RecordTaskCompleted(context.Background(), "w1", "p1", "agent", 5*time.Second)
	var rm metricdata.ResourceMetrics
	reader.Collect(context.Background(), &rm)
	assertHasMetric(t, rm, "drover_tasks_completed_total")
}

func TestRecordTaskFailed(t *testing.T) {
	reader := setupTestMeter(t)
	RecordTaskFailed(context.Background(), "w1", "p1", "agent", "timeout", 3*time.Second)
	var rm metricdata.ResourceMetrics
	reader.Collect(context.Background(), &rm)
	assertHasMetric(t, rm, "drover_tasks_failed_total")
}

func TestRecordTaskRetry(t *testing.T) {
	reader := setupTestMeter(t)
	RecordTaskRetry(context.Background(), "t1", 2)
	var rm metricdata.ResourceMetrics
	reader.Collect(context.Background(), &rm)
	assertHasMetric(t, rm, "drover_tasks_retried_total")
}

func TestRecordClaimLatency(t *testing.T) {
	reader := setupTestMeter(t)
	RecordClaimLatency(context.Background(), "p1", 200*time.Millisecond)
	var rm metricdata.ResourceMetrics
	reader.Collect(context.Background(), &rm)
	assertHasMetric(t, rm, "drover_claim_latency_seconds")
}

func TestRecordBlockerDetected(t *testing.T) {
	reader := setupTestMeter(t)
	RecordBlockerDetected(context.Background(), "merge_conflict", "p1")
	var rm metricdata.ResourceMetrics
	reader.Collect(context.Background(), &rm)
	assertHasMetric(t, rm, "drover_blockers_detected_total")
}

func TestRecordFixTaskCreated(t *testing.T) {
	reader := setupTestMeter(t)
	RecordFixTaskCreated(context.Background(), "merge_conflict", "p1")
	var rm metricdata.ResourceMetrics
	reader.Collect(context.Background(), &rm)
	assertHasMetric(t, rm, "drover_fix_tasks_created_total")
}

func TestRecordAgentPrompt(t *testing.T) {
	reader := setupTestMeter(t)
	RecordAgentPrompt(context.Background(), AgentTypeClaudeCode)
	var rm metricdata.ResourceMetrics
	reader.Collect(context.Background(), &rm)
	assertHasMetric(t, rm, "drover_agent_prompts_total")
}

func TestRecordAgentToolCall(t *testing.T) {
	reader := setupTestMeter(t)
	RecordAgentToolCall(context.Background(), AgentTypeClaudeCode, "bash")
	var rm metricdata.ResourceMetrics
	reader.Collect(context.Background(), &rm)
	assertHasMetric(t, rm, "drover_agent_tool_calls_total")
}

func TestRecordAgentError(t *testing.T) {
	reader := setupTestMeter(t)
	RecordAgentError(context.Background(), AgentTypeCodex, "rate_limit")
	var rm metricdata.ResourceMetrics
	reader.Collect(context.Background(), &rm)
	assertHasMetric(t, rm, "drover_agent_errors_total")
}

func TestRecordAgentDuration(t *testing.T) {
	reader := setupTestMeter(t)
	RecordAgentDuration(context.Background(), AgentTypeAmp, 12*time.Second)
	var rm metricdata.ResourceMetrics
	reader.Collect(context.Background(), &rm)
	assertHasMetric(t, rm, "drover_agent_duration_seconds")
}

func TestRecordWorktreeSetup(t *testing.T) {
	reader := setupTestMeter(t)
	RecordWorktreeSetup(context.Background(), 500*time.Millisecond)
	var rm metricdata.ResourceMetrics
	reader.Collect(context.Background(), &rm)
	assertHasMetric(t, rm, "drover_worktree_setup_seconds")
}

// ============================================================================
// Nil-Guard Tests (counters not initialized)
// ============================================================================

func TestNilGuards(t *testing.T) {
	// Save and nil-out all counters
	saved := struct {
		claimed, completed, failed, retried                           metric.Int64Counter
		blockers, fixTasks, prompts, toolCalls, agentErrors           metric.Int64Counter
		taskDur, agentDur, claimLat, wtSetup                          metric.Float64Histogram
	}{
		tasksClaimedCounter, tasksCompletedCounter, tasksFailedCounter, tasksRetriedCounter,
		blockersDetectedCounter, fixTasksCreatedCounter, agentPromptsCounter, agentToolCallsCounter, agentErrorsCounter,
		taskDurationHistogram, agentDurationHistogram, claimLatencyHistogram, worktreeSetupHistogram,
	}
	tasksClaimedCounter, tasksCompletedCounter, tasksFailedCounter, tasksRetriedCounter = nil, nil, nil, nil
	blockersDetectedCounter, fixTasksCreatedCounter, agentPromptsCounter, agentToolCallsCounter, agentErrorsCounter = nil, nil, nil, nil, nil
	taskDurationHistogram, agentDurationHistogram, claimLatencyHistogram, worktreeSetupHistogram = nil, nil, nil, nil

	t.Cleanup(func() {
		tasksClaimedCounter = saved.claimed
		tasksCompletedCounter = saved.completed
		tasksFailedCounter = saved.failed
		tasksRetriedCounter = saved.retried
		blockersDetectedCounter = saved.blockers
		fixTasksCreatedCounter = saved.fixTasks
		agentPromptsCounter = saved.prompts
		agentToolCallsCounter = saved.toolCalls
		agentErrorsCounter = saved.agentErrors
		taskDurationHistogram = saved.taskDur
		agentDurationHistogram = saved.agentDur
		claimLatencyHistogram = saved.claimLat
		worktreeSetupHistogram = saved.wtSetup
	})

	ctx := context.Background()
	// None of these should panic
	RecordTaskClaimed(ctx, "", "")
	RecordTaskCompleted(ctx, "", "", "", 0)
	RecordTaskFailed(ctx, "", "", "", "", 0)
	RecordTaskRetry(ctx, "", 0)
	RecordClaimLatency(ctx, "", 0)
	RecordBlockerDetected(ctx, "", "")
	RecordFixTaskCreated(ctx, "", "")
	RecordAgentPrompt(ctx, "")
	RecordAgentToolCall(ctx, "", "")
	RecordAgentError(ctx, "", "")
	RecordAgentDuration(ctx, "", 0)
	RecordWorktreeSetup(ctx, 0)
}

// ============================================================================
// WithTaskID / WithWorkerID / WithEpicID span options
// ============================================================================

func TestWithTaskID(t *testing.T) {
	sr := setupTestTracer(t)
	_, span := tracer.Start(context.Background(), "test", trace.WithAttributes(attribute.String(KeyTaskID, "t-1")))
	span.End()
	if v, ok := findAttr(sr.Ended()[0].Attributes(), KeyTaskID); !ok || v.AsString() != "t-1" {
		t.Error("WithTaskID attribute missing")
	}
}

// ============================================================================
// Helpers
// ============================================================================

func assertHasMetric(t *testing.T, rm metricdata.ResourceMetrics, name string) {
	t.Helper()
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				return
			}
		}
	}
	t.Errorf("metric %q not found", name)
}
