package callbacks

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"
)

// LogLevel defines the verbosity level for logging
type LogLevel int

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

// String returns the string representation of the log level
func (l LogLevel) String() string {
	switch l {
	case LogLevelDebug:
		return "debug"
	case LogLevelInfo:
		return "info"
	case LogLevelWarn:
		return "warn"
	case LogLevelError:
		return "error"
	default:
		return "unknown"
	}
}

// LoggingConfig configures the structured logging callback
type LoggingConfig struct {
	// Output destination (defaults to stdout)
	Writer io.Writer
	// Minimum log level to output
	MinLevel LogLevel
	// Include timestamp in logs
	Timestamp bool
	// Include correlation ID for request tracing
	CorrelationID bool
	// Enable pretty-printing for development
	Pretty bool
}

// StructuredLoggingCallback implements Callback with JSON-formatted logs.
// All events are logged as structured JSON for easy parsing by external tools.
type StructuredLoggingCallback struct {
	config LoggingConfig
	mu     sync.Mutex
	logger *log.Logger
}

// NewStructuredLoggingCallback creates a new structured logging callback
func NewStructuredLoggingCallback(config LoggingConfig) *StructuredLoggingCallback {
	writer := config.Writer
	if writer == nil {
		writer = os.Stdout
	}

	return &StructuredLoggingCallback{
		config: config,
		logger: log.New(writer, "", 0),
	}
}

// logEvent writes a structured log entry
func (c *StructuredLoggingCallback) logEvent(eventType EventType, data any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Build log entry
	entry := map[string]any{
		"event":     eventType,
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
	}

	// Add correlation ID if enabled
	if c.config.CorrelationID {
		// In a real implementation, this would extract from context
		entry["correlation_id"] = generateCorrelationID()
	}

	// Add event-specific data
	switch v := data.(type) {
	case *TaskEventContext:
		entry["task_id"] = v.TaskID
		entry["title"] = v.Title
		entry["epic_id"] = v.EpicID
		entry["type"] = v.Type
		entry["priority"] = v.Priority
		entry["prev_state"] = v.PrevState
		entry["new_state"] = v.NewState
		entry["worker_id"] = v.WorkerID
		entry["attempt"] = v.Attempt
		entry["max_attempts"] = v.MaxAttempts
		if v.Duration != nil {
			entry["duration_ms"] = v.Duration.Milliseconds()
		}
		if v.Error != "" {
			entry["error"] = v.Error
			entry["error_type"] = v.ErrorType
			entry["error_category"] = v.ErrorCategory
		}
		// Add metadata
		if len(v.Metadata) > 0 {
			entry["metadata"] = v.Metadata
		}

	case *WorkerEventContext:
		entry["worker_id"] = v.WorkerID
		entry["worker_index"] = v.WorkerIndex
		entry["state"] = v.State
		entry["current_task_id"] = v.CurrentTaskID
		if v.StallReason != "" {
			entry["stall_reason"] = v.StallReason
			entry["stall_duration_ms"] = v.StallDuration.Milliseconds()
		}
		if len(v.Metadata) > 0 {
			entry["metadata"] = v.Metadata
		}

	case *RecoveryEventContext:
		entry["task_id"] = v.TaskID
		entry["title"] = v.Title
		entry["strategy"] = v.Strategy
		entry["reason"] = v.Reason
		entry["confidence"] = v.Confidence
		entry["original_error"] = v.OriginalError
		entry["attempt_count"] = v.AttemptCount
		if len(v.Metadata) > 0 {
			entry["metadata"] = v.Metadata
		}
	}

	// Marshal to JSON
	var output []byte
	var err error
	if c.config.Pretty {
		output, err = json.MarshalIndent(entry, "", "  ")
	} else {
		output, err = json.Marshal(entry)
	}
	if err != nil {
		return fmt.Errorf("marshaling log entry: %w", err)
	}

	// Write output
	c.logger.Println(string(output))
	return nil
}

// OnTaskCreated implements Callback
func (c *StructuredLoggingCallback) OnTaskCreated(ctx *TaskEventContext) error {
	return c.logEvent(EventTaskCreated, ctx)
}

// OnTaskAssigned implements Callback
func (c *StructuredLoggingCallback) OnTaskAssigned(ctx *TaskEventContext) error {
	return c.logEvent(EventTaskAssigned, ctx)
}

// OnTaskStarted implements Callback
func (c *StructuredLoggingCallback) OnTaskStarted(ctx *TaskEventContext) error {
	return c.logEvent(EventTaskStarted, ctx)
}

// OnTaskCompleted implements Callback
func (c *StructuredLoggingCallback) OnTaskCompleted(ctx *TaskEventContext) error {
	return c.logEvent(EventTaskCompleted, ctx)
}

// OnTaskFailed implements Callback
func (c *StructuredLoggingCallback) OnTaskFailed(ctx *TaskEventContext) error {
	return c.logEvent(EventTaskFailed, ctx)
}

// OnTaskBlocked implements Callback
func (c *StructuredLoggingCallback) OnTaskBlocked(ctx *TaskEventContext) error {
	return c.logEvent(EventTaskBlocked, ctx)
}

// OnTaskUnblocked implements Callback
func (c *StructuredLoggingCallback) OnTaskUnblocked(ctx *TaskEventContext) error {
	return c.logEvent(EventTaskUnblocked, ctx)
}

// OnTaskRecovered implements Callback
func (c *StructuredLoggingCallback) OnTaskRecovered(ctx *RecoveryEventContext) error {
	return c.logEvent(EventTaskRecovered, ctx)
}

// OnWorkerStarted implements Callback
func (c *StructuredLoggingCallback) OnWorkerStarted(ctx *WorkerEventContext) error {
	return c.logEvent(EventWorkerStarted, ctx)
}

// OnWorkerStopped implements Callback
func (c *StructuredLoggingCallback) OnWorkerStopped(ctx *WorkerEventContext) error {
	return c.logEvent(EventWorkerStopped, ctx)
}

// OnWorkerStalled implements Callback
func (c *StructuredLoggingCallback) OnWorkerStalled(ctx *WorkerEventContext) error {
	return c.logEvent(EventWorkerStalled, ctx)
}

// generateCorrelationID generates a unique correlation ID for request tracing
func generateCorrelationID() string {
	return fmt.Sprintf("corr-%d", time.Now().UnixNano())
}
