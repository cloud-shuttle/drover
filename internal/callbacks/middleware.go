package callbacks

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

// Middleware wraps a Callback to add pre/post processing logic
type Middleware interface {
	Callback

	// Wrap returns a new Callback that wraps the given one
	Wrap(next Callback) Callback
}

// MiddlewareFunc is an adapter that allows a function to be used as Middleware
type MiddlewareFunc struct {
	callback Callback
}

// Wrap implements Middleware
func (mf *MiddlewareFunc) Wrap(next Callback) Callback {
	mf.callback = next
	return mf
}

// Callback forwarding for MiddlewareFunc
func (mf *MiddlewareFunc) OnTaskCreated(ctx *TaskEventContext) error {
	if mf.callback != nil {
		return mf.callback.OnTaskCreated(ctx)
	}
	return nil
}

func (mf *MiddlewareFunc) OnTaskAssigned(ctx *TaskEventContext) error {
	if mf.callback != nil {
		return mf.callback.OnTaskAssigned(ctx)
	}
	return nil
}

func (mf *MiddlewareFunc) OnTaskStarted(ctx *TaskEventContext) error {
	if mf.callback != nil {
		return mf.callback.OnTaskStarted(ctx)
	}
	return nil
}

func (mf *MiddlewareFunc) OnTaskCompleted(ctx *TaskEventContext) error {
	if mf.callback != nil {
		return mf.callback.OnTaskCompleted(ctx)
	}
	return nil
}

func (mf *MiddlewareFunc) OnTaskFailed(ctx *TaskEventContext) error {
	if mf.callback != nil {
		return mf.callback.OnTaskFailed(ctx)
	}
	return nil
}

func (mf *MiddlewareFunc) OnTaskBlocked(ctx *TaskEventContext) error {
	if mf.callback != nil {
		return mf.callback.OnTaskBlocked(ctx)
	}
	return nil
}

func (mf *MiddlewareFunc) OnTaskUnblocked(ctx *TaskEventContext) error {
	if mf.callback != nil {
		return mf.callback.OnTaskUnblocked(ctx)
	}
	return nil
}

func (mf *MiddlewareFunc) OnTaskRecovered(ctx *RecoveryEventContext) error {
	if mf.callback != nil {
		return mf.callback.OnTaskRecovered(ctx)
	}
	return nil
}

func (mf *MiddlewareFunc) OnWorkerStarted(ctx *WorkerEventContext) error {
	if mf.callback != nil {
		return mf.callback.OnWorkerStarted(ctx)
	}
	return nil
}

func (mf *MiddlewareFunc) OnWorkerStopped(ctx *WorkerEventContext) error {
	if mf.callback != nil {
		return mf.callback.OnWorkerStopped(ctx)
	}
	return nil
}

func (mf *MiddlewareFunc) OnWorkerStalled(ctx *WorkerEventContext) error {
	if mf.callback != nil {
		return mf.callback.OnWorkerStalled(ctx)
	}
	return nil
}

// ChainMiddleware chains multiple middleware together
func ChainMiddleware(callback Callback, middlewares ...Middleware) Callback {
	for i := len(middlewares) - 1; i >= 0; i-- {
		callback = middlewares[i].Wrap(callback)
	}
	return callback
}

// TimeoutMiddleware adds timeout to callback execution
type TimeoutMiddleware struct {
	timeout  time.Duration
	logger   *log.Logger
	onTimeout func(event string, duration time.Duration)
}

// NewTimeoutMiddleware creates a new timeout middleware
func NewTimeoutMiddleware(timeout time.Duration) *TimeoutMiddleware {
	return &TimeoutMiddleware{
		timeout: timeout,
		logger:  log.New(os.Stdout, "[timeout-middleware] ", log.LstdFlags),
		onTimeout: func(event string, duration time.Duration) {
			// Default: log timeout
		},
	}
}

// SetLogger sets the logger for the timeout middleware
func (tm *TimeoutMiddleware) SetLogger(logger *log.Logger) {
	tm.logger = logger
}

// SetOnTimeout sets a callback function to be called on timeout
func (tm *TimeoutMiddleware) SetOnTimeout(fn func(event string, duration time.Duration)) {
	tm.onTimeout = fn
}

// Wrap implements Middleware
func (tm *TimeoutMiddleware) Wrap(next Callback) Callback {
	return &timeoutCallback{
		next:      next,
		timeout:   tm.timeout,
		logger:    tm.logger,
		onTimeout: tm.onTimeout,
	}
}

type timeoutCallback struct {
	next      Callback
	timeout   time.Duration
	logger    *log.Logger
	onTimeout func(event string, duration time.Duration)
}

func (tc *timeoutCallback) OnTaskCreated(ctx *TaskEventContext) error {
	return tc.runWithTimeout("OnTaskCreated", func() error {
		return tc.next.OnTaskCreated(ctx)
	})
}

func (tc *timeoutCallback) OnTaskAssigned(ctx *TaskEventContext) error {
	return tc.runWithTimeout("OnTaskAssigned", func() error {
		return tc.next.OnTaskAssigned(ctx)
	})
}

func (tc *timeoutCallback) OnTaskStarted(ctx *TaskEventContext) error {
	return tc.runWithTimeout("OnTaskStarted", func() error {
		return tc.next.OnTaskStarted(ctx)
	})
}

func (tc *timeoutCallback) OnTaskCompleted(ctx *TaskEventContext) error {
	return tc.runWithTimeout("OnTaskCompleted", func() error {
		return tc.next.OnTaskCompleted(ctx)
	})
}

func (tc *timeoutCallback) OnTaskFailed(ctx *TaskEventContext) error {
	return tc.runWithTimeout("OnTaskFailed", func() error {
		return tc.next.OnTaskFailed(ctx)
	})
}

func (tc *timeoutCallback) OnTaskBlocked(ctx *TaskEventContext) error {
	return tc.runWithTimeout("OnTaskBlocked", func() error {
		return tc.next.OnTaskBlocked(ctx)
	})
}

func (tc *timeoutCallback) OnTaskUnblocked(ctx *TaskEventContext) error {
	return tc.runWithTimeout("OnTaskUnblocked", func() error {
		return tc.next.OnTaskUnblocked(ctx)
	})
}

func (tc *timeoutCallback) OnTaskRecovered(ctx *RecoveryEventContext) error {
	return tc.runWithTimeout("OnTaskRecovered", func() error {
		return tc.next.OnTaskRecovered(ctx)
	})
}

func (tc *timeoutCallback) OnWorkerStarted(ctx *WorkerEventContext) error {
	return tc.runWithTimeout("OnWorkerStarted", func() error {
		return tc.next.OnWorkerStarted(ctx)
	})
}

func (tc *timeoutCallback) OnWorkerStopped(ctx *WorkerEventContext) error {
	return tc.runWithTimeout("OnWorkerStopped", func() error {
		return tc.next.OnWorkerStopped(ctx)
	})
}

func (tc *timeoutCallback) OnWorkerStalled(ctx *WorkerEventContext) error {
	return tc.runWithTimeout("OnWorkerStalled", func() error {
		return tc.next.OnWorkerStalled(ctx)
	})
}

func (tc *timeoutCallback) runWithTimeout(event string, fn func() error) error {
	done := make(chan error, 1)
	go func() {
		done <- fn()
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(tc.timeout):
		tc.onTimeout(event, tc.timeout)
		tc.logger.Printf("[timeout-middleware] %s exceeded timeout of %v", event, tc.timeout)
		return fmt.Errorf("callback %s timed out after %v", event, tc.timeout)
	}
}

// FilterMiddleware filters which callbacks are executed based on predicates
type FilterMiddleware struct {
	taskEventFilter     func(*TaskEventContext) bool
	recoveryEventFilter func(*RecoveryEventContext) bool
	workerEventFilter   func(*WorkerEventContext) bool
	logger              *log.Logger
}

// NewFilterMiddleware creates a new filter middleware
func NewFilterMiddleware() *FilterMiddleware {
	return &FilterMiddleware{
		taskEventFilter:     func(*TaskEventContext) bool { return true },
		recoveryEventFilter: func(*RecoveryEventContext) bool { return true },
		workerEventFilter:   func(*WorkerEventContext) bool { return true },
		logger:              log.New(os.Stdout, "[filter-middleware] ", log.LstdFlags),
	}
}

// SetLogger sets the logger for the filter middleware
func (fm *FilterMiddleware) SetLogger(logger *log.Logger) {
	fm.logger = logger
}

// SetTaskEventFilter sets the filter for task events
func (fm *FilterMiddleware) SetTaskEventFilter(filter func(*TaskEventContext) bool) {
	fm.taskEventFilter = filter
}

// SetRecoveryEventFilter sets the filter for recovery events
func (fm *FilterMiddleware) SetRecoveryEventFilter(filter func(*RecoveryEventContext) bool) {
	fm.recoveryEventFilter = filter
}

// SetWorkerEventFilter sets the filter for worker events
func (fm *FilterMiddleware) SetWorkerEventFilter(filter func(*WorkerEventContext) bool) {
	fm.workerEventFilter = filter
}

// FilterByEpic filters events by epic ID
func (fm *FilterMiddleware) FilterByEpic(epics ...string) {
	fm.SetTaskEventFilter(func(ctx *TaskEventContext) bool {
		for _, epic := range epics {
			if ctx.EpicID == epic {
				return true
			}
		}
		return false
	})
}

// FilterByTaskType filters events by task type
func (fm *FilterMiddleware) FilterByTaskType(types ...string) {
	fm.SetTaskEventFilter(func(ctx *TaskEventContext) bool {
		for _, t := range types {
			if ctx.Type == t {
				return true
			}
		}
		return false
	})
}

// FilterByWorkerID filters events by worker ID
func (fm *FilterMiddleware) FilterByWorkerID(workers ...string) {
	fm.SetTaskEventFilter(func(ctx *TaskEventContext) bool {
		for _, w := range workers {
			if ctx.WorkerID == w {
				return true
			}
		}
		return false
	})
}

// Wrap implements Middleware
func (fm *FilterMiddleware) Wrap(next Callback) Callback {
	return &filterCallback{
		next:                next,
		taskEventFilter:     fm.taskEventFilter,
		recoveryEventFilter: fm.recoveryEventFilter,
		workerEventFilter:   fm.workerEventFilter,
		logger:              fm.logger,
	}
}

type filterCallback struct {
	next                Callback
	taskEventFilter     func(*TaskEventContext) bool
	recoveryEventFilter func(*RecoveryEventContext) bool
	workerEventFilter   func(*WorkerEventContext) bool
	logger              *log.Logger
}

func (fc *filterCallback) OnTaskCreated(ctx *TaskEventContext) error {
	if !fc.taskEventFilter(ctx) {
		return nil // Filtered out
	}
	return fc.next.OnTaskCreated(ctx)
}

func (fc *filterCallback) OnTaskAssigned(ctx *TaskEventContext) error {
	if !fc.taskEventFilter(ctx) {
		return nil
	}
	return fc.next.OnTaskAssigned(ctx)
}

func (fc *filterCallback) OnTaskStarted(ctx *TaskEventContext) error {
	if !fc.taskEventFilter(ctx) {
		return nil
	}
	return fc.next.OnTaskStarted(ctx)
}

func (fc *filterCallback) OnTaskCompleted(ctx *TaskEventContext) error {
	if !fc.taskEventFilter(ctx) {
		return nil
	}
	return fc.next.OnTaskCompleted(ctx)
}

func (fc *filterCallback) OnTaskFailed(ctx *TaskEventContext) error {
	if !fc.taskEventFilter(ctx) {
		return nil
	}
	return fc.next.OnTaskFailed(ctx)
}

func (fc *filterCallback) OnTaskBlocked(ctx *TaskEventContext) error {
	if !fc.taskEventFilter(ctx) {
		return nil
	}
	return fc.next.OnTaskBlocked(ctx)
}

func (fc *filterCallback) OnTaskUnblocked(ctx *TaskEventContext) error {
	if !fc.taskEventFilter(ctx) {
		return nil
	}
	return fc.next.OnTaskUnblocked(ctx)
}

func (fc *filterCallback) OnTaskRecovered(ctx *RecoveryEventContext) error {
	if !fc.recoveryEventFilter(ctx) {
		return nil
	}
	return fc.next.OnTaskRecovered(ctx)
}

func (fc *filterCallback) OnWorkerStarted(ctx *WorkerEventContext) error {
	if !fc.workerEventFilter(ctx) {
		return nil
	}
	return fc.next.OnWorkerStarted(ctx)
}

func (fc *filterCallback) OnWorkerStopped(ctx *WorkerEventContext) error {
	if !fc.workerEventFilter(ctx) {
		return nil
	}
	return fc.next.OnWorkerStopped(ctx)
}

func (fc *filterCallback) OnWorkerStalled(ctx *WorkerEventContext) error {
	if !fc.workerEventFilter(ctx) {
		return nil
	}
	return fc.next.OnWorkerStalled(ctx)
}

// RetryMiddleware retries failed callback executions
type RetryMiddleware struct {
	maxRetries int
	backoff    func(attempt int) time.Duration
	logger     *log.Logger
}

// NewRetryMiddleware creates a new retry middleware
func NewRetryMiddleware(maxRetries int) *RetryMiddleware {
	return &RetryMiddleware{
		maxRetries: maxRetries,
		backoff:    func(attempt int) time.Duration { return time.Duration(attempt) * 100 * time.Millisecond },
		logger:     log.New(os.Stdout, "[retry-middleware] ", log.LstdFlags),
	}
}

// SetLogger sets the logger for the retry middleware
func (rm *RetryMiddleware) SetLogger(logger *log.Logger) {
	rm.logger = logger
}

// SetBackoff sets the backoff strategy
func (rm *RetryMiddleware) SetBackoff(backoff func(attempt int) time.Duration) {
	rm.backoff = backoff
}

// Wrap implements Middleware
func (rm *RetryMiddleware) Wrap(next Callback) Callback {
	return &retryCallback{
		next:       next,
		maxRetries: rm.maxRetries,
		backoff:    rm.backoff,
		logger:     rm.logger,
	}
}

type retryCallback struct {
	next       Callback
	maxRetries int
	backoff    func(attempt int) time.Duration
	logger     *log.Logger
}

func (rc *retryCallback) OnTaskCreated(ctx *TaskEventContext) error {
	return rc.retry("OnTaskCreated", func() error {
		return rc.next.OnTaskCreated(ctx)
	})
}

func (rc *retryCallback) OnTaskAssigned(ctx *TaskEventContext) error {
	return rc.retry("OnTaskAssigned", func() error {
		return rc.next.OnTaskAssigned(ctx)
	})
}

func (rc *retryCallback) OnTaskStarted(ctx *TaskEventContext) error {
	return rc.retry("OnTaskStarted", func() error {
		return rc.next.OnTaskStarted(ctx)
	})
}

func (rc *retryCallback) OnTaskCompleted(ctx *TaskEventContext) error {
	return rc.retry("OnTaskCompleted", func() error {
		return rc.next.OnTaskCompleted(ctx)
	})
}

func (rc *retryCallback) OnTaskFailed(ctx *TaskEventContext) error {
	return rc.retry("OnTaskFailed", func() error {
		return rc.next.OnTaskFailed(ctx)
	})
}

func (rc *retryCallback) OnTaskBlocked(ctx *TaskEventContext) error {
	return rc.retry("OnTaskBlocked", func() error {
		return rc.next.OnTaskBlocked(ctx)
	})
}

func (rc *retryCallback) OnTaskUnblocked(ctx *TaskEventContext) error {
	return rc.retry("OnTaskUnblocked", func() error {
		return rc.next.OnTaskUnblocked(ctx)
	})
}

func (rc *retryCallback) OnTaskRecovered(ctx *RecoveryEventContext) error {
	return rc.retry("OnTaskRecovered", func() error {
		return rc.next.OnTaskRecovered(ctx)
	})
}

func (rc *retryCallback) OnWorkerStarted(ctx *WorkerEventContext) error {
	return rc.retry("OnWorkerStarted", func() error {
		return rc.next.OnWorkerStarted(ctx)
	})
}

func (rc *retryCallback) OnWorkerStopped(ctx *WorkerEventContext) error {
	return rc.retry("OnWorkerStopped", func() error {
		return rc.next.OnWorkerStopped(ctx)
	})
}

func (rc *retryCallback) OnWorkerStalled(ctx *WorkerEventContext) error {
	return rc.retry("OnWorkerStalled", func() error {
		return rc.next.OnWorkerStalled(ctx)
	})
}

func (rc *retryCallback) retry(event string, fn func() error) error {
	var lastErr error
	for i := 0; i <= rc.maxRetries; i++ {
		err := fn()
		if err == nil {
			return nil
		}
		lastErr = err

		if i < rc.maxRetries {
			backoff := rc.backoff(i)
			rc.logger.Printf("[retry-middleware] %s failed (attempt %d/%d), retrying in %v: %v",
				event, i+1, rc.maxRetries+1, backoff, err)
			time.Sleep(backoff)
		}
	}
	return fmt.Errorf("callback %s failed after %d attempts: %w", event, rc.maxRetries+1, lastErr)
}

// CompositeCallback executes multiple callbacks in sequence
type CompositeCallback struct {
	mu        sync.RWMutex
	callbacks []Callback
	logger    *log.Logger
	stopOnError bool
}

// NewCompositeCallback creates a new composite callback
func NewCompositeCallback(callbacks ...Callback) *CompositeCallback {
	return &CompositeCallback{
		callbacks:   callbacks,
		logger:      log.New(os.Stdout, "[composite] ", log.LstdFlags),
		stopOnError: false,
	}
}

// SetLogger sets the logger for the composite callback
func (cc *CompositeCallback) SetLogger(logger *log.Logger) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.logger = logger
}

// SetStopOnError sets whether to stop execution on first error
func (cc *CompositeCallback) SetStopOnError(stop bool) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.stopOnError = stop
}

// Add adds a callback to the composite
func (cc *CompositeCallback) Add(callback Callback) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.callbacks = append(cc.callbacks, callback)
}

// Remove removes a callback from the composite
func (cc *CompositeCallback) Remove(callback Callback) {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	for i, cb := range cc.callbacks {
		if cb == callback {
			cc.callbacks = append(cc.callbacks[:i], cc.callbacks[i+1:]...)
			break
		}
	}
}

// executeAll runs all callbacks for an event
func (cc *CompositeCallback) executeAll(event string, fn func(Callback) error) error {
	cc.mu.RLock()
	callbacks := make([]Callback, len(cc.callbacks))
	copy(callbacks, cc.callbacks)
	stopOnError := cc.stopOnError
	cc.mu.RUnlock()

	var errs []string
	for _, cb := range callbacks {
		if err := fn(cb); err != nil {
			errs = append(errs, err.Error())
			if stopOnError {
				break
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("callback errors for %s: %s", event, strings.Join(errs, "; "))
	}
	return nil
}

// OnTaskCreated implements Callback
func (cc *CompositeCallback) OnTaskCreated(ctx *TaskEventContext) error {
	return cc.executeAll("OnTaskCreated", func(cb Callback) error {
		return cb.OnTaskCreated(ctx)
	})
}

// OnTaskAssigned implements Callback
func (cc *CompositeCallback) OnTaskAssigned(ctx *TaskEventContext) error {
	return cc.executeAll("OnTaskAssigned", func(cb Callback) error {
		return cb.OnTaskAssigned(ctx)
	})
}

// OnTaskStarted implements Callback
func (cc *CompositeCallback) OnTaskStarted(ctx *TaskEventContext) error {
	return cc.executeAll("OnTaskStarted", func(cb Callback) error {
		return cb.OnTaskStarted(ctx)
	})
}

// OnTaskCompleted implements Callback
func (cc *CompositeCallback) OnTaskCompleted(ctx *TaskEventContext) error {
	return cc.executeAll("OnTaskCompleted", func(cb Callback) error {
		return cb.OnTaskCompleted(ctx)
	})
}

// OnTaskFailed implements Callback
func (cc *CompositeCallback) OnTaskFailed(ctx *TaskEventContext) error {
	return cc.executeAll("OnTaskFailed", func(cb Callback) error {
		return cb.OnTaskFailed(ctx)
	})
}

// OnTaskBlocked implements Callback
func (cc *CompositeCallback) OnTaskBlocked(ctx *TaskEventContext) error {
	return cc.executeAll("OnTaskBlocked", func(cb Callback) error {
		return cb.OnTaskBlocked(ctx)
	})
}

// OnTaskUnblocked implements Callback
func (cc *CompositeCallback) OnTaskUnblocked(ctx *TaskEventContext) error {
	return cc.executeAll("OnTaskUnblocked", func(cb Callback) error {
		return cb.OnTaskUnblocked(ctx)
	})
}

// OnTaskRecovered implements Callback
func (cc *CompositeCallback) OnTaskRecovered(ctx *RecoveryEventContext) error {
	return cc.executeAll("OnTaskRecovered", func(cb Callback) error {
		return cb.OnTaskRecovered(ctx)
	})
}

// OnWorkerStarted implements Callback
func (cc *CompositeCallback) OnWorkerStarted(ctx *WorkerEventContext) error {
	return cc.executeAll("OnWorkerStarted", func(cb Callback) error {
		return cb.OnWorkerStarted(ctx)
	})
}

// OnWorkerStopped implements Callback
func (cc *CompositeCallback) OnWorkerStopped(ctx *WorkerEventContext) error {
	return cc.executeAll("OnWorkerStopped", func(cb Callback) error {
		return cb.OnWorkerStopped(ctx)
	})
}

// OnWorkerStalled implements Callback
func (cc *CompositeCallback) OnWorkerStalled(ctx *WorkerEventContext) error {
	return cc.executeAll("OnWorkerStalled", func(cb Callback) error {
		return cb.OnWorkerStalled(ctx)
	})
}

// GetCallbacks returns all registered callbacks
func (cc *CompositeCallback) GetCallbacks() []Callback {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	result := make([]Callback, len(cc.callbacks))
	copy(result, cc.callbacks)
	return result
}

// Count returns the number of registered callbacks
func (cc *CompositeCallback) Count() int {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	return len(cc.callbacks)
}
