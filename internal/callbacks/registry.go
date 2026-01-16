package callbacks

import (
	"log"
	"sync"
)

// Priority defines callback execution order (lower = earlier)
type Priority int

const (
	PriorityHigh   Priority = 0
	PriorityMedium Priority = 50
	PriorityLow    Priority = 100
)

// registeredCallback holds a callback with its priority and metadata
type registeredCallback struct {
	callback  Callback
	priority  Priority
	name      string
	enabled   bool
}

// Registry manages lifecycle event callbacks.
// It provides thread-safe registration, unregistration, and dispatching
// of callbacks for workforce lifecycle events.
//
// The registry is designed for zero-overhead when no callbacks are registered:
// dispatch operations check if there are any registered callbacks before
// creating context or invoking handlers.
type Registry struct {
	mu        sync.RWMutex
	callbacks map[EventType][]*registeredCallback
	logger    *log.Logger
}

// NewRegistry creates a new callback registry
func NewRegistry() *Registry {
	return &Registry{
		callbacks: make(map[EventType][]*registeredCallback),
	}
}

// SetLogger sets the logger for the registry
func (r *Registry) SetLogger(logger *log.Logger) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logger = logger
}

// Register registers a callback for specific event types.
// The callback will be invoked for all specified event types.
//
// Priority controls execution order: lower priority callbacks run first.
// Use the same priority for independent callbacks where order doesn't matter.
//
// The name is used for debugging and error logging.
// If a callback with the same name exists for the same event, it will be replaced.
func (r *Registry) Register(callback Callback, events []EventType, priority Priority, name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, event := range events {
		// Initialize slice if needed
		if r.callbacks[event] == nil {
			r.callbacks[event] = make([]*registeredCallback, 0, 4)
		}

		// Check for existing callback with same name
		existingIndex := -1
		for i, cb := range r.callbacks[event] {
			if cb.name == name {
				existingIndex = i
				break
			}
		}

		reg := &registeredCallback{
			callback: callback,
			priority: priority,
			name:     name,
			enabled:  true,
		}

		if existingIndex >= 0 {
			// Replace existing callback
			r.callbacks[event][existingIndex] = reg
			r.logf("Updated callback '%s' for event %s", name, event)
		} else {
			// Add new callback and insert in priority order
			r.callbacks[event] = r.insertSorted(r.callbacks[event], reg)
			r.logf("Registered callback '%s' for event %s", name, event)
		}
	}
}

// Unregister removes a callback by name for specific event types.
// If events is empty, the callback is removed from all event types.
func (r *Registry) Unregister(name string, events []EventType) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(events) == 0 {
		// Remove from all event types
		for event, callbacks := range r.callbacks {
			filtered := make([]*registeredCallback, 0, len(callbacks))
			for _, cb := range callbacks {
				if cb.name != name {
					filtered = append(filtered, cb)
				}
			}
			r.callbacks[event] = filtered
		}
		r.logf("Unregistered callback '%s' from all events", name)
		return
	}

	// Remove from specific event types
	for _, event := range events {
		callbacks := r.callbacks[event]
		filtered := make([]*registeredCallback, 0, len(callbacks))
		for _, cb := range callbacks {
			if cb.name != name {
				filtered = append(filtered, cb)
			}
		}
		r.callbacks[event] = filtered
		r.logf("Unregistered callback '%s' from event %s", name, event)
	}
}

// Enable enables a callback by name
func (r *Registry) Enable(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, callbacks := range r.callbacks {
		for _, cb := range callbacks {
			if cb.name == name {
				cb.enabled = true
			}
		}
	}
}

// Disable disables a callback by name
func (r *Registry) Disable(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, callbacks := range r.callbacks {
		for _, cb := range callbacks {
			if cb.name == name {
				cb.enabled = false
			}
		}
	}
}

// Dispatch invokes all registered callbacks for a task event.
// Returns the first non-nil error from any callback, but continues
// invoking remaining callbacks even after an error.
// Callback errors are logged but do not propagate to crash the system.
func (r *Registry) Dispatch(event EventType, ctx *TaskEventContext) error {
	return r.dispatchTask(event, ctx)
}

// DispatchTask invokes all registered callbacks for a task event
func (r *Registry) DispatchTask(event EventType, ctx *TaskEventContext) error {
	return r.dispatchTask(event, ctx)
}

// dispatchTask is the internal implementation for task event dispatch
func (r *Registry) dispatchTask(event EventType, ctx *TaskEventContext) error {
	r.mu.RLock()
	callbacks := r.callbacks[event]
	r.mu.RUnlock()

	// Fast path: no callbacks registered
	if len(callbacks) == 0 {
		return nil
	}

	var firstErr error
	for _, cb := range callbacks {
		if !cb.enabled {
			continue
		}

		if err := r.invokeCallback(cb.callback, event, ctx); err != nil {
			r.logf("Callback '%s' error on event %s: %v", cb.name, event, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	return firstErr
}

// DispatchWorker invokes all registered callbacks for a worker event
func (r *Registry) DispatchWorker(event EventType, ctx *WorkerEventContext) error {
	r.mu.RLock()
	callbacks := r.callbacks[event]
	r.mu.RUnlock()

	// Fast path: no callbacks registered
	if len(callbacks) == 0 {
		return nil
	}

	var firstErr error
	for _, cb := range callbacks {
		if !cb.enabled {
			continue
		}

		if err := r.invokeCallback(cb.callback, event, ctx); err != nil {
			r.logf("Callback '%s' error on event %s: %v", cb.name, event, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	return firstErr
}

// DispatchRecovery invokes all registered callbacks for a recovery event
func (r *Registry) DispatchRecovery(event EventType, ctx *RecoveryEventContext) error {
	r.mu.RLock()
	callbacks := r.callbacks[event]
	r.mu.RUnlock()

	// Fast path: no callbacks registered
	if len(callbacks) == 0 {
		return nil
	}

	var firstErr error
	for _, cb := range callbacks {
		if !cb.enabled {
			continue
		}

		if err := r.invokeCallback(cb.callback, event, ctx); err != nil {
			r.logf("Callback '%s' error on event %s: %v", cb.name, event, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	return firstErr
}

// invokeCallback invokes a single callback with proper error handling
func (r *Registry) invokeCallback(cb Callback, event EventType, ctx any) error {
	// Recover from panics in callbacks
	defer func() {
		if recovered := recover(); recovered != nil {
			r.logf("Callback panic on event %s: %v", event, recovered)
		}
	}()

	var err error
	switch event {
	case EventTaskCreated:
		err = cb.OnTaskCreated(ctx.(*TaskEventContext))
	case EventTaskAssigned:
		err = cb.OnTaskAssigned(ctx.(*TaskEventContext))
	case EventTaskStarted:
		err = cb.OnTaskStarted(ctx.(*TaskEventContext))
	case EventTaskCompleted:
		err = cb.OnTaskCompleted(ctx.(*TaskEventContext))
	case EventTaskFailed:
		err = cb.OnTaskFailed(ctx.(*TaskEventContext))
	case EventTaskBlocked:
		err = cb.OnTaskBlocked(ctx.(*TaskEventContext))
	case EventTaskUnblocked:
		err = cb.OnTaskUnblocked(ctx.(*TaskEventContext))
	case EventTaskRecovered:
		err = cb.OnTaskRecovered(ctx.(*RecoveryEventContext))
	case EventWorkerStarted:
		err = cb.OnWorkerStarted(ctx.(*WorkerEventContext))
	case EventWorkerStopped:
		err = cb.OnWorkerStopped(ctx.(*WorkerEventContext))
	case EventWorkerStalled:
		err = cb.OnWorkerStalled(ctx.(*WorkerEventContext))
	}
	return err
}

// Count returns the number of callbacks registered for an event type
func (r *Registry) Count(event EventType) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.callbacks[event])
}

// RegisteredNames returns the names of all callbacks registered for an event type
func (r *Registry) RegisteredNames(event EventType) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.callbacks[event]))
	for _, cb := range r.callbacks[event] {
		names = append(names, cb.name)
	}
	return names
}

// insertSorted inserts a callback into a sorted list by priority
func (r *Registry) insertSorted(list []*registeredCallback, item *registeredCallback) []*registeredCallback {
	// Find insertion point
	i := 0
	for i < len(list) && list[i].priority <= item.priority {
		i++
	}

	// Insert at position i
	if i == len(list) {
		return append(list, item)
	}

	result := make([]*registeredCallback, 0, len(list)+1)
	result = append(result, list[:i]...)
	result = append(result, item)
	result = append(result, list[i:]...)
	return result
}

// logf logs a message if a logger is configured
func (r *Registry) logf(format string, args ...any) {
	if r.logger != nil {
		r.logger.Printf(format, args...)
	}
}

// GlobalRegistry is the default global callback registry
var GlobalRegistry = NewRegistry()
