package workflow

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/dbos-inc/dbos-transact-golang/dbos"
)

// MockDBOSContext is a test-only implementation of dbos.DBOSContext that runs
// workflow steps synchronously without requiring a PostgreSQL database.
// It satisfies the full DBOSContext interface but only implements the methods
// that Drover's orchestrator actually uses:
//   - RunAsStep: executes the step function synchronously
//   - RunWorkflow: stores the handle for later retrieval
//   - Go: executes the step concurrently and returns a result channel
//   - RegisterWorkflow/Launch/Shutdown: no-ops
//
// All other methods return sensible zero values or ErrNotImplemented.
type MockDBOSContext struct {
	context.Context

	mu               sync.Mutex
	stepCalls        []stepCall        // records every RunAsStep invocation
	workflowCalls    []workflowCall    // records every RunWorkflow invocation
	goCalls          int               // number of Go() invocations
	launched         bool
	shutdownCalled   bool
	applicationID    string
	executorID       string
	appVersion       string

	// OnRunAsStep can be set to override the default pass-through behavior.
	// Return (result, error) to control step outcomes in tests.
	OnRunAsStep func(fn dbos.StepFunc) (any, error)

	// OnRunWorkflow can be set to override the default no-op behavior.
	// By default RunWorkflow just records the call and returns a successful mock handle.
	OnRunWorkflow func(fn dbos.WorkflowFunc, input any) (any, error)
}

// stepCall records a single RunAsStep invocation for test assertions.
type stepCall struct {
	Result any
	Err    error
}

// workflowCall records a single RunWorkflow invocation for test assertions.
type workflowCall struct {
	WorkflowID string
	QueueName  string
	Input      any
}

// NewMockDBOSContext creates a new mock context backed by context.Background().
func NewMockDBOSContext() *MockDBOSContext {
	return &MockDBOSContext{
		Context:       context.Background(),
		applicationID: "drover-test-mock",
		executorID:    "mock-executor",
		appVersion:    "test-0.0.0",
	}
}

// NewMockDBOSContextWithParent creates a mock context wrapping the given parent.
func NewMockDBOSContextWithParent(parent context.Context) *MockDBOSContext {
	return &MockDBOSContext{
		Context:       parent,
		applicationID: "drover-test-mock",
		executorID:    "mock-executor",
		appVersion:    "test-0.0.0",
	}
}

// --- Test Assertion Helpers ---

// StepCallCount returns how many times RunAsStep was invoked.
func (m *MockDBOSContext) StepCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.stepCalls)
}

// WorkflowCallCount returns how many times RunWorkflow was invoked.
func (m *MockDBOSContext) WorkflowCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.workflowCalls)
}

// GoCallCount returns how many times Go() was invoked.
func (m *MockDBOSContext) GoCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.goCalls
}

// WasLaunched returns whether Launch() was called.
func (m *MockDBOSContext) WasLaunched() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.launched
}

// WasShutdown returns whether Shutdown() was called.
func (m *MockDBOSContext) WasShutdown() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.shutdownCalled
}

// --- Context Lifecycle ---

func (m *MockDBOSContext) Launch() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.launched = true
	return nil
}

func (m *MockDBOSContext) Shutdown(timeout time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shutdownCalled = true
}

// --- Workflow Operations (the ones Drover actually uses) ---

func (m *MockDBOSContext) RunAsStep(_ dbos.DBOSContext, fn dbos.StepFunc, opts ...dbos.StepOption) (any, error) {
	if m.OnRunAsStep != nil {
		result, err := m.OnRunAsStep(fn)
		m.mu.Lock()
		m.stepCalls = append(m.stepCalls, stepCall{Result: result, Err: err})
		m.mu.Unlock()
		return result, err
	}

	// Default: execute the step function synchronously with a derived context
	result, err := fn(m.Context)
	m.mu.Lock()
	m.stepCalls = append(m.stepCalls, stepCall{Result: result, Err: err})
	m.mu.Unlock()
	return result, err
}

func (m *MockDBOSContext) RunWorkflow(_ dbos.DBOSContext, fn dbos.WorkflowFunc, input any, opts ...dbos.WorkflowOption) (dbos.WorkflowHandle[any], error) {
	m.mu.Lock()
	m.workflowCalls = append(m.workflowCalls, workflowCall{Input: input})
	m.mu.Unlock()

	// If a custom handler is set, use it to execute the workflow
	if m.OnRunWorkflow != nil {
		result, err := m.OnRunWorkflow(fn, input)
		return &mockWorkflowHandle[any]{result: result, err: err}, nil
	}

	// Default: return a successful no-op handle without executing the workflow.
	// This prevents nil panics from workflow functions that depend on infrastructure
	// (e.g., git worktrees, agent executors) that doesn't exist in unit tests.
	return &mockWorkflowHandle[any]{result: nil, err: nil}, nil
}

func (m *MockDBOSContext) Go(_ dbos.DBOSContext, fn dbos.StepFunc, opts ...dbos.StepOption) (chan dbos.StepOutcome[any], error) {
	m.mu.Lock()
	m.goCalls++
	m.mu.Unlock()

	ch := make(chan dbos.StepOutcome[any], 1)
	go func() {
		result, err := fn(m.Context)
		ch <- dbos.StepOutcome[any]{Result: result, Err: err}
	}()
	return ch, nil
}

func (m *MockDBOSContext) Select(_ dbos.DBOSContext, channels []<-chan dbos.StepOutcome[any]) (any, error) {
	// Wait for the first channel to return
	if len(channels) == 0 {
		return nil, fmt.Errorf("no channels provided to Select")
	}

	// Simple: return the first result
	outcome := <-channels[0]
	return outcome.Result, outcome.Err
}

// --- Messaging (not used by Drover, stub implementations) ---

func (m *MockDBOSContext) Send(_ dbos.DBOSContext, destinationID string, message any, topic string, opts ...dbos.SendOption) error {
	return nil
}

func (m *MockDBOSContext) Recv(_ dbos.DBOSContext, topic string, timeout time.Duration) (any, error) {
	return nil, nil
}

func (m *MockDBOSContext) SetEvent(_ dbos.DBOSContext, key string, message any, opts ...dbos.SetEventOption) error {
	return nil
}

func (m *MockDBOSContext) GetEvent(_ dbos.DBOSContext, targetWorkflowID string, key string, timeout time.Duration) (any, error) {
	return nil, nil
}

// --- Streams (not used by Drover, stub implementations) ---

func (m *MockDBOSContext) WriteStream(_ dbos.DBOSContext, key string, value any, opts ...dbos.WriteStreamOption) error {
	return nil
}

func (m *MockDBOSContext) CloseStream(_ dbos.DBOSContext, key string) error {
	return nil
}

func (m *MockDBOSContext) ReadStream(_ dbos.DBOSContext, workflowID string, key string) ([]any, bool, error) {
	return nil, false, nil
}

func (m *MockDBOSContext) ReadStreamAsync(_ dbos.DBOSContext, workflowID string, key string) (<-chan dbos.StreamValue[any], error) {
	return nil, nil
}

// --- Durable Sleep / Patching ---

func (m *MockDBOSContext) Sleep(_ dbos.DBOSContext, duration time.Duration) (time.Duration, error) {
	// In tests, don't actually sleep
	return duration, nil
}

func (m *MockDBOSContext) Patch(_ dbos.DBOSContext, patchName string) (bool, error) {
	return true, nil // Always use "new" path in tests
}

func (m *MockDBOSContext) DeprecatePatch(_ dbos.DBOSContext, patchName string) error {
	return nil
}

func (m *MockDBOSContext) GetWorkflowID() (string, error) {
	return "mock-workflow-id", nil
}

func (m *MockDBOSContext) GetStepID() (int, error) {
	return 0, nil
}

// --- Workflow Management (stubs) ---

func (m *MockDBOSContext) RetrieveWorkflow(_ dbos.DBOSContext, workflowID string) (dbos.WorkflowHandle[any], error) {
	return nil, fmt.Errorf("RetrieveWorkflow not implemented in mock")
}

func (m *MockDBOSContext) CancelWorkflow(_ dbos.DBOSContext, workflowID string) error {
	return nil
}

func (m *MockDBOSContext) SetWorkflowDelay(_ dbos.DBOSContext, workflowID string, opts ...dbos.SetWorkflowDelayOption) error {
	return nil
}

func (m *MockDBOSContext) ResumeWorkflow(_ dbos.DBOSContext, workflowID string, opts ...dbos.ResumeWorkflowOption) (dbos.WorkflowHandle[any], error) {
	return nil, fmt.Errorf("ResumeWorkflow not implemented in mock")
}

func (m *MockDBOSContext) ResumeWorkflows(_ dbos.DBOSContext, workflowIDs []string, opts ...dbos.ResumeWorkflowOption) ([]dbos.WorkflowHandle[any], error) {
	return nil, fmt.Errorf("ResumeWorkflows not implemented in mock")
}

func (m *MockDBOSContext) ForkWorkflow(_ dbos.DBOSContext, input dbos.ForkWorkflowInput) (dbos.WorkflowHandle[any], error) {
	return nil, fmt.Errorf("ForkWorkflow not implemented in mock")
}

func (m *MockDBOSContext) ListWorkflows(_ dbos.DBOSContext, opts ...dbos.ListWorkflowsOption) ([]dbos.WorkflowStatus, error) {
	return nil, nil
}

func (m *MockDBOSContext) GetWorkflowSteps(_ dbos.DBOSContext, workflowID string) ([]dbos.StepInfo, error) {
	return nil, nil
}

func (m *MockDBOSContext) ListRegisteredWorkflows(_ dbos.DBOSContext, opts ...dbos.ListRegisteredWorkflowsOption) ([]dbos.WorkflowRegistryEntry, error) {
	return nil, nil
}

func (m *MockDBOSContext) ListRegisteredQueues(_ dbos.DBOSContext) ([]dbos.WorkflowQueue, error) {
	return nil, nil
}

func (m *MockDBOSContext) DeleteWorkflows(_ dbos.DBOSContext, workflowIDs []string, opts ...dbos.DeleteWorkflowOption) error {
	return nil
}

// --- Accessors ---

func (m *MockDBOSContext) GetApplicationVersion() string { return m.appVersion }
func (m *MockDBOSContext) GetExecutorID() string         { return m.executorID }
func (m *MockDBOSContext) GetApplicationID() string      { return m.applicationID }

// --- Context Management ---

func (m *MockDBOSContext) From(_ dbos.DBOSContext, ctx context.Context) dbos.DBOSContext {
	return &MockDBOSContext{Context: ctx, applicationID: m.applicationID, executorID: m.executorID, appVersion: m.appVersion}
}

func (m *MockDBOSContext) WithoutCancel(_ dbos.DBOSContext) dbos.DBOSContext {
	return m
}

func (m *MockDBOSContext) WithTimeout(_ dbos.DBOSContext, timeout time.Duration) (dbos.DBOSContext, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(m.Context, timeout)
	return &MockDBOSContext{Context: ctx, applicationID: m.applicationID, executorID: m.executorID, appVersion: m.appVersion}, cancel
}

func (m *MockDBOSContext) WithValue(key, val any) dbos.DBOSContext {
	return &MockDBOSContext{Context: context.WithValue(m.Context, key, val), applicationID: m.applicationID, executorID: m.executorID, appVersion: m.appVersion}
}

func (m *MockDBOSContext) WithCancelCause() (dbos.DBOSContext, context.CancelCauseFunc) {
	ctx, cancel := context.WithCancelCause(m.Context)
	return &MockDBOSContext{Context: ctx, applicationID: m.applicationID, executorID: m.executorID, appVersion: m.appVersion}, cancel
}

// --- Queue Configuration ---

func (m *MockDBOSContext) ListenQueues(_ dbos.DBOSContext, queues ...dbos.WorkflowQueue) {
	// no-op in tests
}

// --- Schedule Management (stubs) ---

func (m *MockDBOSContext) CreateSchedule(_ dbos.DBOSContext, fn dbos.ScheduledWorkflowFunc, input dbos.CreateScheduleRequest, opts ...dbos.CreateScheduleOption) error {
	return nil
}

func (m *MockDBOSContext) ApplySchedules(_ dbos.DBOSContext, schedules []dbos.ApplySchedulesRequest) error {
	return nil
}

func (m *MockDBOSContext) PauseSchedule(_ dbos.DBOSContext, scheduleName string) error {
	return nil
}

func (m *MockDBOSContext) ResumeSchedule(_ dbos.DBOSContext, scheduleName string) error {
	return nil
}

func (m *MockDBOSContext) DeleteSchedule(_ dbos.DBOSContext, scheduleName string) error {
	return nil
}

func (m *MockDBOSContext) GetSchedule(_ dbos.DBOSContext, scheduleName string) (*dbos.WorkflowSchedule, error) {
	return nil, nil
}

func (m *MockDBOSContext) ListSchedules(_ dbos.DBOSContext, opts ...dbos.ListSchedulesOption) ([]dbos.WorkflowSchedule, error) {
	return nil, nil
}

func (m *MockDBOSContext) BackfillSchedule(_ dbos.DBOSContext, scheduleName string, start time.Time, end time.Time) ([]string, error) {
	return nil, nil
}

func (m *MockDBOSContext) TriggerSchedule(_ dbos.DBOSContext, scheduleName string) (dbos.WorkflowHandle[any], error) {
	return nil, nil
}

// --- Alert Handling ---

func (m *MockDBOSContext) SetAlertHandler(handler dbos.AlertHandler) {
	// no-op in tests
}

// --- Mock Workflow Handle ---

// mockWorkflowHandle implements dbos.WorkflowHandle for test results.
type mockWorkflowHandle[R any] struct {
	result R
	err    error
}

func (h *mockWorkflowHandle[R]) GetResult(opts ...dbos.GetResultOption) (R, error) {
	return h.result, h.err
}

func (h *mockWorkflowHandle[R]) GetWorkflowID() string {
	return "mock-workflow-handle-id"
}

func (h *mockWorkflowHandle[R]) GetStatus() (dbos.WorkflowStatus, error) {
	return dbos.WorkflowStatus{Status: dbos.WorkflowStatusSuccess}, nil
}

// Compile-time assertion: MockDBOSContext implements dbos.DBOSContext
var _ dbos.DBOSContext = (*MockDBOSContext)(nil)
