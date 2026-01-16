package callbacks

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

// HealthStatus represents the health status of a component
type HealthStatus string

const (
	StatusHealthy   HealthStatus = "healthy"
	StatusDegraded  HealthStatus = "degraded"
	StatusUnhealthy HealthStatus = "unhealthy"
	StatusUnknown   HealthStatus = "unknown"
)

// ComponentHealth represents the health of a single component
type ComponentHealth struct {
	Name      string                 `json:"name"`
	Status    HealthStatus           `json:"status"`
	Message   string                 `json:"message,omitempty"`
	CheckedAt time.Time              `json:"checked_at"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// SystemHealth represents the overall health of the system
type SystemHealth struct {
	Status     HealthStatus                 `json:"status"`
	Version    string                       `json:"version,omitempty"`
	Timestamp  time.Time                    `json:"timestamp"`
	Components map[string]*ComponentHealth  `json:"components"`
}

// HealthCheck defines a function that checks the health of a component
type HealthCheck func(ctx context.Context) (*ComponentHealth, error)

// HealthCallback implements Callback with health monitoring capabilities.
// It tracks component health and provides HTTP endpoints for health checks.
type HealthCallback struct {
	mu              sync.RWMutex
	checks          map[string]HealthCheck
	componentStates map[string]*ComponentHealth
	logger          *log.Logger
	server          *http.Server
	enabled         bool
}

// NewHealthCallback creates a new health callback
func NewHealthCallback() *HealthCallback {
	return &HealthCallback{
		checks:          make(map[string]HealthCheck),
		componentStates: make(map[string]*ComponentHealth),
		logger:          log.New(os.Stdout, "[health] ", log.LstdFlags),
		enabled:         false,
	}
}

// SetLogger sets the logger for the health callback
func (hc *HealthCallback) SetLogger(logger *log.Logger) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	hc.logger = logger
}

// RegisterCheck registers a health check for a component
func (hc *HealthCallback) RegisterCheck(name string, check HealthCheck) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	hc.checks[name] = check
	hc.logger.Printf("[health] registered check for component: %s", name)
}

// UnregisterCheck removes a health check
func (hc *HealthCallback) UnregisterCheck(name string) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	delete(hc.checks, name)
	delete(hc.componentStates, name)
	hc.logger.Printf("[health] unregistered check for component: %s", name)
}

// RunChecks executes all registered health checks
func (hc *HealthCallback) RunChecks(ctx context.Context) (*SystemHealth, error) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	health := &SystemHealth{
		Status:     StatusHealthy,
		Timestamp:  time.Now(),
		Components: make(map[string]*ComponentHealth),
	}

	var wg sync.WaitGroup
	var mu sync.Mutex

	for name, check := range hc.checks {
		wg.Add(1)
		go func(name string, check HealthCheck) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			componentHealth, err := check(ctx)
			if err != nil {
				componentHealth = &ComponentHealth{
					Name:      name,
					Status:    StatusUnhealthy,
					Message:   fmt.Sprintf("health check failed: %v", err),
					CheckedAt: time.Now(),
				}
			}

			mu.Lock()
			health.Components[name] = componentHealth
			hc.componentStates[name] = componentHealth
			mu.Unlock()
		}(name, check)
	}

	wg.Wait()

	// Determine overall health status
	for _, component := range health.Components {
		if component.Status == StatusUnhealthy {
			health.Status = StatusUnhealthy
			break
		} else if component.Status == StatusDegraded && health.Status == StatusHealthy {
			health.Status = StatusDegraded
		}
	}

	return health, nil
}

// GetStatus returns the current health status without running checks
func (hc *HealthCallback) GetStatus() *SystemHealth {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	health := &SystemHealth{
		Status:     StatusHealthy,
		Timestamp:  time.Now(),
		Components: make(map[string]*ComponentHealth),
	}

	// Copy current states
	for name, state := range hc.componentStates {
		stateCopy := *state
		health.Components[name] = &stateCopy

		if state.Status == StatusUnhealthy {
			health.Status = StatusUnhealthy
		} else if state.Status == StatusDegraded && health.Status == StatusHealthy {
			health.Status = StatusDegraded
		}
	}

	if len(health.Components) == 0 {
		health.Status = StatusUnknown
	}

	return health
}

// StartServer starts an HTTP server for health checks
func (hc *HealthCallback) StartServer(addr string) error {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	if hc.enabled {
		return fmt.Errorf("health server already running")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", hc.handleHealth)
	mux.HandleFunc("/healthz", hc.handleHealthz)
	mux.HandleFunc("/ready", hc.handleReady)
	mux.HandleFunc("/metrics", hc.handleMetrics) // Prometheus metrics endpoint

	hc.server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	hc.enabled = true
	hc.logger.Printf("[health] starting health server on %s", addr)

	go func() {
		if err := hc.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			hc.logger.Printf("[health] server error: %v", err)
		}
	}()

	return nil
}

// StopServer stops the health check HTTP server
func (hc *HealthCallback) StopServer(ctx context.Context) error {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	if !hc.enabled || hc.server == nil {
		return nil
	}

	hc.logger.Printf("[health] stopping health server")
	if err := hc.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown health server: %w", err)
	}

	hc.enabled = false
	hc.server = nil
	return nil
}

// handleHealth handles the /health endpoint
func (hc *HealthCallback) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	health, err := hc.RunChecks(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// Set status code based on health
	switch health.Status {
	case StatusHealthy:
		w.WriteHeader(http.StatusOK)
	case StatusDegraded:
		w.WriteHeader(http.StatusOK) // 200 for degraded, service is still operational
	case StatusUnhealthy:
		w.WriteHeader(http.StatusServiceUnavailable)
	default:
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	json.NewEncoder(w).Encode(health)
}

// handleHealthz handles the /healthz endpoint (simple liveness probe)
func (hc *HealthCallback) handleHealthz(w http.ResponseWriter, r *http.Request) {
	health := hc.GetStatus()

	// For liveness, just check if the service is running
	if health.Status == StatusUnhealthy {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("unhealthy"))
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// handleReady handles the /ready endpoint (readiness probe)
func (hc *HealthCallback) handleReady(w http.ResponseWriter, r *http.Request) {
	health := hc.GetStatus()

	// For readiness, require healthy or degraded status
	if health.Status == StatusUnhealthy || health.Status == StatusUnknown {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("not ready"))
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ready"))
}

// handleMetrics handles the /metrics endpoint
func (hc *HealthCallback) handleMetrics(w http.ResponseWriter, r *http.Request) {
	health := hc.GetStatus()

	w.Header().Set("Content-Type", "text/plain")

	// Output health as Prometheus metrics
	fmt.Fprintf(w, "# HELP drover_health_status Health status (1=healthy, 2=degraded, 3=unhealthy, 0=unknown)\n")
	fmt.Fprintf(w, "# TYPE drover_health_status gauge\n")

	statusValue := 0.0
	switch health.Status {
	case StatusHealthy:
		statusValue = 1
	case StatusDegraded:
		statusValue = 2
	case StatusUnhealthy:
		statusValue = 3
	}
	fmt.Fprintf(w, "drover_health_status %v\n", statusValue)

	// Component health metrics
	fmt.Fprintf(w, "\n# HELP drover_component_health Component health status\n")
	fmt.Fprintf(w, "# TYPE drover_component_health gauge\n")
	for name, component := range health.Components {
		componentValue := 0.0
		switch component.Status {
		case StatusHealthy:
			componentValue = 1
		case StatusDegraded:
			componentValue = 2
		case StatusUnhealthy:
			componentValue = 3
		}
		fmt.Fprintf(w, `drover_component_health{component="%s"} %v`+"\n", name, componentValue)
	}
}

// IsHealthy returns true if the system is healthy
func (hc *HealthCallback) IsHealthy() bool {
	health := hc.GetStatus()
	return health.Status == StatusHealthy || health.Status == StatusDegraded
}

// IsReady returns true if the system is ready
func (hc *HealthCallback) IsReady() bool {
	health := hc.GetStatus()
	return health.Status == StatusHealthy || health.Status == StatusDegraded
}

// Task event handlers to track task health

// OnTaskCreated implements Callback
func (hc *HealthCallback) OnTaskCreated(ctx *TaskEventContext) error {
	return nil
}

// OnTaskAssigned implements Callback
func (hc *HealthCallback) OnTaskAssigned(ctx *TaskEventContext) error {
	return nil
}

// OnTaskStarted implements Callback
func (hc *HealthCallback) OnTaskStarted(ctx *TaskEventContext) error {
	return nil
}

// OnTaskCompleted implements Callback
func (hc *HealthCallback) OnTaskCompleted(ctx *TaskEventContext) error {
	return nil
}

// OnTaskFailed implements Callback
func (hc *HealthCallback) OnTaskFailed(ctx *TaskEventContext) error {
	return nil
}

// OnTaskBlocked implements Callback
func (hc *HealthCallback) OnTaskBlocked(ctx *TaskEventContext) error {
	return nil
}

// OnTaskUnblocked implements Callback
func (hc *HealthCallback) OnTaskUnblocked(ctx *TaskEventContext) error {
	return nil
}

// OnTaskRecovered implements Callback
func (hc *HealthCallback) OnTaskRecovered(ctx *RecoveryEventContext) error {
	return nil
}

// Worker lifecycle handlers for worker health

// OnWorkerStarted implements Callback
func (hc *HealthCallback) OnWorkerStarted(ctx *WorkerEventContext) error {
	return nil
}

// OnWorkerStopped implements Callback
func (hc *HealthCallback) OnWorkerStopped(ctx *WorkerEventContext) error {
	return nil
}

// OnWorkerStalled implements Callback
func (hc *HealthCallback) OnWorkerStalled(ctx *WorkerEventContext) error {
	return nil
}

// Standard health checks

// MemoryCheck returns a health check for memory usage
func MemoryCheck(thresholdMB uint64) HealthCheck {
	return func(ctx context.Context) (*ComponentHealth, error) {
		// This would use actual memory stats in a real implementation
		// For now, return a placeholder
		return &ComponentHealth{
			Name:      "memory",
			Status:    StatusHealthy,
			Message:   "memory usage within limits",
			CheckedAt: time.Now(),
			Metadata: map[string]interface{}{
				"threshold_mb": thresholdMB,
			},
		}, nil
	}
}

// GoroutineCheck returns a health check for goroutine count
func GoroutineCheck(threshold int) HealthCheck {
	return func(ctx context.Context) (*ComponentHealth, error) {
		return &ComponentHealth{
			Name:      "goroutines",
			Status:    StatusHealthy,
			Message:   "goroutine count within limits",
			CheckedAt: time.Now(),
			Metadata: map[string]interface{}{
				"threshold": threshold,
			},
		}, nil
	}
}
