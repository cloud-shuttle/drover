package callbacks

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

// LifecyclePhase represents the current phase of task or worker lifecycle
type LifecyclePhase string

const (
	PhaseInitializing LifecyclePhase = "initializing"
	PhaseRunning      LifecyclePhase = "running"
	PhaseSuspending   LifecyclePhase = "suspending"
	PhaseSuspended    LifecyclePhase = "suspended"
	PhaseTerminating  LifecyclePhase = "terminating"
	PhaseTerminated   LifecyclePhase = "terminated"
)

// LifecycleState represents the state of a tracked lifecycle entity
type LifecycleState struct {
	Phase        LifecyclePhase
	StartedAt    time.Time
	LastActivity time.Time
	Metadata     map[string]string
}

// TaskStateMachine tracks state transitions for tasks
type TaskStateMachine struct {
	mu      sync.RWMutex
	states  map[string]*LifecycleState // task ID -> state
	logger  *log.Logger
	transitions map[string][]StateTransition
}

// StateTransition records a state change
type StateTransition struct {
	From      LifecyclePhase
	To        LifecyclePhase
	Timestamp time.Time
	Reason    string
}

// NewTaskStateMachine creates a new task state machine
func NewTaskStateMachine() *TaskStateMachine {
	return &TaskStateMachine{
		states:      make(map[string]*LifecycleState),
		transitions: make(map[string][]StateTransition),
		logger:      log.New(os.Stdout, "[lifecycle] ", log.LstdFlags),
	}
}

// SetLogger sets the logger for the state machine
func (sm *TaskStateMachine) SetLogger(logger *log.Logger) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.logger = logger
}

// Initialize adds a new task to the state machine
func (sm *TaskStateMachine) Initialize(taskID string, metadata map[string]string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now()
	sm.states[taskID] = &LifecycleState{
		Phase:        PhaseInitializing,
		StartedAt:    now,
		LastActivity: now,
		Metadata:     metadata,
	}
	sm.transitions[taskID] = []StateTransition{
		{
			From:      "",
			To:        PhaseInitializing,
			Timestamp: now,
			Reason:    "task_created",
		},
	}

	sm.logger.Printf("[lifecycle] task %s initialized in phase %s", taskID, PhaseInitializing)
}

// Transition moves a task to a new phase
func (sm *TaskStateMachine) Transition(taskID string, newPhase LifecyclePhase, reason string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	state, exists := sm.states[taskID]
	if !exists {
		return fmt.Errorf("task %s not found in state machine", taskID)
	}

	if !sm.isValidTransition(state.Phase, newPhase) {
		return fmt.Errorf("invalid transition from %s to %s for task %s", state.Phase, newPhase, taskID)
	}

	oldPhase := state.Phase
	state.Phase = newPhase
	state.LastActivity = time.Now()

	sm.transitions[taskID] = append(sm.transitions[taskID], StateTransition{
		From:      oldPhase,
		To:        newPhase,
		Timestamp: time.Now(),
		Reason:    reason,
	})

	sm.logger.Printf("[lifecycle] task %s transitioned: %s -> %s (reason: %s)", taskID, oldPhase, newPhase, reason)
	return nil
}

// GetState returns the current state of a task
func (sm *TaskStateMachine) GetState(taskID string) (*LifecycleState, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	state, exists := sm.states[taskID]
	if !exists {
		return nil, false
	}

	// Return a copy
	stateCopy := *state
	return &stateCopy, true
}

// GetTransitions returns the transition history for a task
func (sm *TaskStateMachine) GetTransitions(taskID string) []StateTransition {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	transitions, exists := sm.transitions[taskID]
	if !exists {
		return nil
	}

	// Return a copy
	result := make([]StateTransition, len(transitions))
	copy(result, transitions)
	return result
}

// Remove removes a task from the state machine
func (sm *TaskStateMachine) Remove(taskID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	delete(sm.states, taskID)
	delete(sm.transitions, taskID)
	sm.logger.Printf("[lifecycle] task %s removed from state machine", taskID)
}

// isValidTransition checks if a transition between phases is valid
func (sm *TaskStateMachine) isValidTransition(from, to LifecyclePhase) bool {
	validTransitions := map[LifecyclePhase][]LifecyclePhase{
		PhaseInitializing: {PhaseRunning, PhaseTerminating},
		PhaseRunning:      {PhaseSuspending, PhaseTerminating},
		PhaseSuspending:   {PhaseSuspended, PhaseRunning},
		PhaseSuspended:    {PhaseRunning, PhaseTerminating},
		PhaseTerminating:  {PhaseTerminated},
		PhaseTerminated:   {},
	}

	allowed, exists := validTransitions[from]
	if !exists {
		return false
	}

	for _, valid := range allowed {
		if valid == to {
			return true
		}
	}
	return false
}

// GetAllStates returns all tracked states
func (sm *TaskStateMachine) GetAllStates() map[string]*LifecycleState {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	result := make(map[string]*LifecycleState, len(sm.states))
	for k, v := range sm.states {
		stateCopy := *v
		result[k] = &stateCopy
	}
	return result
}

// LifecycleCallback implements Callback with lifecycle orchestration capabilities.
// It provides hooks for pre/post task execution and state machine management.
type LifecycleCallback struct {
	mu           sync.RWMutex
	stateMachine *TaskStateMachine
	preHooks     map[string][]func(*TaskEventContext) error
	postHooks    map[string][]func(*TaskEventContext) error
	logger       *log.Logger
}

// NewLifecycleCallback creates a new lifecycle callback
func NewLifecycleCallback() *LifecycleCallback {
	return &LifecycleCallback{
		stateMachine: NewTaskStateMachine(),
		preHooks:     make(map[string][]func(*TaskEventContext) error),
		postHooks:    make(map[string][]func(*TaskEventContext) error),
		logger:       log.New(os.Stdout, "[lifecycle] ", log.LstdFlags),
	}
}

// SetLogger sets the logger for the lifecycle callback
func (lc *LifecycleCallback) SetLogger(logger *log.Logger) {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	lc.logger = logger
	lc.stateMachine.SetLogger(logger)
}

// RegisterPreHook registers a function to be called before a task event
func (lc *LifecycleCallback) RegisterPreHook(eventType string, hook func(*TaskEventContext) error) {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	lc.preHooks[eventType] = append(lc.preHooks[eventType], hook)
}

// RegisterPostHook registers a function to be called after a task event
func (lc *LifecycleCallback) RegisterPostHook(eventType string, hook func(*TaskEventContext) error) {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	lc.postHooks[eventType] = append(lc.postHooks[eventType], hook)
}

// GetStateMachine returns the state machine
func (lc *LifecycleCallback) GetStateMachine() *TaskStateMachine {
	return lc.stateMachine
}

// invokePreHooks calls all pre-hooks for an event type
func (lc *LifecycleCallback) invokePreHooks(eventType string, ctx *TaskEventContext) error {
	lc.mu.RLock()
	hooks := lc.preHooks[eventType]
	lc.mu.RUnlock()

	for _, hook := range hooks {
		if err := hook(ctx); err != nil {
			return fmt.Errorf("pre-hook failed for %s: %w", eventType, err)
		}
	}
	return nil
}

// invokePostHooks calls all post-hooks for an event type
func (lc *LifecycleCallback) invokePostHooks(eventType string, ctx *TaskEventContext) error {
	lc.mu.RLock()
	hooks := lc.postHooks[eventType]
	lc.mu.RUnlock()

	for _, hook := range hooks {
		if err := hook(ctx); err != nil {
			return fmt.Errorf("post-hook failed for %s: %w", eventType, err)
		}
	}
	return nil
}

// OnTaskCreated implements Callback
func (lc *LifecycleCallback) OnTaskCreated(ctx *TaskEventContext) error {
	if err := lc.invokePreHooks("created", ctx); err != nil {
		return err
	}

	lc.stateMachine.Initialize(ctx.TaskID, map[string]string{
		"epic":    ctx.EpicID,
		"type":    ctx.Type,
		"created": time.Now().Format(time.RFC3339),
	})

	if err := lc.invokePostHooks("created", ctx); err != nil {
		return err
	}

	return nil
}

// OnTaskAssigned implements Callback
func (lc *LifecycleCallback) OnTaskAssigned(ctx *TaskEventContext) error {
	if err := lc.invokePreHooks("assigned", ctx); err != nil {
		return err
	}

	if err := lc.stateMachine.Transition(ctx.TaskID, PhaseRunning, "assigned_to_worker"); err != nil {
		lc.logger.Printf("[lifecycle] warning: %v", err)
	}

	if err := lc.invokePostHooks("assigned", ctx); err != nil {
		return err
	}

	return nil
}

// OnTaskStarted implements Callback
func (lc *LifecycleCallback) OnTaskStarted(ctx *TaskEventContext) error {
	if err := lc.invokePreHooks("started", ctx); err != nil {
		return err
	}

	if err := lc.stateMachine.Transition(ctx.TaskID, PhaseRunning, "execution_started"); err != nil {
		// May already be in running phase, which is fine
		lc.logger.Printf("[lifecycle] warning: %v", err)
	}

	if err := lc.invokePostHooks("started", ctx); err != nil {
		return err
	}

	return nil
}

// OnTaskCompleted implements Callback
func (lc *LifecycleCallback) OnTaskCompleted(ctx *TaskEventContext) error {
	if err := lc.invokePreHooks("completed", ctx); err != nil {
		return err
	}

	if err := lc.stateMachine.Transition(ctx.TaskID, PhaseTerminating, "task_completed"); err != nil {
		lc.logger.Printf("[lifecycle] warning: %v", err)
	}

	// Mark as terminated after completion
	if err := lc.stateMachine.Transition(ctx.TaskID, PhaseTerminated, "success"); err != nil {
		lc.logger.Printf("[lifecycle] warning: %v", err)
	}

	if err := lc.invokePostHooks("completed", ctx); err != nil {
		return err
	}

	return nil
}

// OnTaskFailed implements Callback
func (lc *LifecycleCallback) OnTaskFailed(ctx *TaskEventContext) error {
	if err := lc.invokePreHooks("failed", ctx); err != nil {
		return err
	}

	if err := lc.stateMachine.Transition(ctx.TaskID, PhaseTerminating, "task_failed"); err != nil {
		lc.logger.Printf("[lifecycle] warning: %v", err)
	}

	// Mark as terminated after failure
	if err := lc.stateMachine.Transition(ctx.TaskID, PhaseTerminated, "failed"); err != nil {
		lc.logger.Printf("[lifecycle] warning: %v", err)
	}

	if err := lc.invokePostHooks("failed", ctx); err != nil {
		return err
	}

	return nil
}

// OnTaskBlocked implements Callback
func (lc *LifecycleCallback) OnTaskBlocked(ctx *TaskEventContext) error {
	if err := lc.invokePreHooks("blocked", ctx); err != nil {
		return err
	}

	if err := lc.stateMachine.Transition(ctx.TaskID, PhaseSuspending, "task_blocked"); err != nil {
		lc.logger.Printf("[lifecycle] warning: %v", err)
	}

	if err := lc.stateMachine.Transition(ctx.TaskID, PhaseSuspended, "awaiting_dependencies"); err != nil {
		lc.logger.Printf("[lifecycle] warning: %v", err)
	}

	if err := lc.invokePostHooks("blocked", ctx); err != nil {
		return err
	}

	return nil
}

// OnTaskUnblocked implements Callback
func (lc *LifecycleCallback) OnTaskUnblocked(ctx *TaskEventContext) error {
	if err := lc.invokePreHooks("unblocked", ctx); err != nil {
		return err
	}

	if err := lc.stateMachine.Transition(ctx.TaskID, PhaseRunning, "dependencies_resolved"); err != nil {
		lc.logger.Printf("[lifecycle] warning: %v", err)
	}

	if err := lc.invokePostHooks("unblocked", ctx); err != nil {
		return err
	}

	return nil
}

// OnTaskRecovered implements Callback
func (lc *LifecycleCallback) OnTaskRecovered(ctx *RecoveryEventContext) error {
	// Recovery may transition back to running depending on strategy
	if ctx.Strategy == "retry" || ctx.Strategy == "escalate" {
		if err := lc.stateMachine.Transition(ctx.TaskID, PhaseRunning, "recovery_initiated"); err != nil {
			lc.logger.Printf("[lifecycle] warning: %v", err)
		}
	}

	return nil
}

// SuspendAll transitions all running tasks to suspended state
func (lc *LifecycleCallback) SuspendAll(ctx context.Context) error {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	states := lc.stateMachine.GetAllStates()
	for taskID, state := range states {
		if state.Phase == PhaseRunning {
			if err := lc.stateMachine.Transition(taskID, PhaseSuspending, "global_suspend"); err != nil {
				lc.logger.Printf("[lifecycle] warning: failed to suspend %s: %v", taskID, err)
			}
			if err := lc.stateMachine.Transition(taskID, PhaseSuspended, "suspended"); err != nil {
				lc.logger.Printf("[lifecycle] warning: failed to mark suspended %s: %v", taskID, err)
			}
		}
	}

	return nil
}

// ResumeAll transitions all suspended tasks back to running
func (lc *LifecycleCallback) ResumeAll(ctx context.Context) error {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	states := lc.stateMachine.GetAllStates()
	for taskID, state := range states {
		if state.Phase == PhaseSuspended {
			if err := lc.stateMachine.Transition(taskID, PhaseRunning, "global_resume"); err != nil {
				lc.logger.Printf("[lifecycle] warning: failed to resume %s: %v", taskID, err)
			}
		}
	}

	return nil
}

// ShutdownAll transitions all active tasks to terminated
func (lc *LifecycleCallback) ShutdownAll(ctx context.Context) error {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	states := lc.stateMachine.GetAllStates()
	for taskID, state := range states {
		if state.Phase == PhaseRunning || state.Phase == PhaseSuspended {
			if err := lc.stateMachine.Transition(taskID, PhaseTerminating, "global_shutdown"); err != nil {
				lc.logger.Printf("[lifecycle] warning: failed to shutdown %s: %v", taskID, err)
			}
			if err := lc.stateMachine.Transition(taskID, PhaseTerminated, "shutdown"); err != nil {
				lc.logger.Printf("[lifecycle] warning: failed to mark terminated %s: %v", taskID, err)
			}
		}
	}

	return nil
}

// GetTasksByPhase returns all tasks in a specific phase
func (lc *LifecycleCallback) GetTasksByPhase(phase LifecyclePhase) []string {
	lc.mu.RLock()
	defer lc.mu.RUnlock()

	states := lc.stateMachine.GetAllStates()
	var result []string
	for taskID, state := range states {
		if state.Phase == phase {
			result = append(result, taskID)
		}
	}
	return result
}

// GetStalledTasks returns tasks that have been in a non-terminal phase too long
func (lc *LifecycleCallback) GetStalledTasks(timeout time.Duration) []string {
	lc.mu.RLock()
	defer lc.mu.RUnlock()

	states := lc.stateMachine.GetAllStates()
	now := time.Now()
	var result []string

	terminalPhases := map[LifecyclePhase]bool{
		PhaseTerminated: true,
	}

	for taskID, state := range states {
		if !terminalPhases[state.Phase] && now.Sub(state.LastActivity) > timeout {
			result = append(result, taskID)
		}
	}

	return result
}

// Worker lifecycle handlers

// OnWorkerStarted implements Callback
func (lc *LifecycleCallback) OnWorkerStarted(ctx *WorkerEventContext) error {
	return nil
}

// OnWorkerStopped implements Callback
func (lc *LifecycleCallback) OnWorkerStopped(ctx *WorkerEventContext) error {
	return nil
}

// OnWorkerStalled implements Callback
func (lc *LifecycleCallback) OnWorkerStalled(ctx *WorkerEventContext) error {
	return nil
}
