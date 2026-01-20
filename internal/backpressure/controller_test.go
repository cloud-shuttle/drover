// Package backpressure provides tests for adaptive concurrency control
package backpressure

import (
	"testing"
	"time"
)

func TestDefaultControllerConfig(t *testing.T) {
	cfg := DefaultControllerConfig()

	if cfg.InitialConcurrency != 2 {
		t.Errorf("DefaultControllerConfig() InitialConcurrency = %d, want 2", cfg.InitialConcurrency)
	}

	if cfg.MinConcurrency != 1 {
		t.Errorf("DefaultControllerConfig() MinConcurrency = %d, want 1", cfg.MinConcurrency)
	}

	if cfg.MaxConcurrency != 4 {
		t.Errorf("DefaultControllerConfig() MaxConcurrency = %d, want 4", cfg.MaxConcurrency)
	}

	if cfg.RateLimitBackoff != 30*time.Second {
		t.Errorf("DefaultControllerConfig() RateLimitBackoff = %v, want %v", cfg.RateLimitBackoff, 30*time.Second)
	}

	if cfg.MaxBackoff != 5*time.Minute {
		t.Errorf("DefaultControllerConfig() MaxBackoff = %v, want %v", cfg.MaxBackoff, 5*time.Minute)
	}

	if cfg.SlowThreshold != 10*time.Second {
		t.Errorf("DefaultControllerConfig() SlowThreshold = %v, want %v", cfg.SlowThreshold, 10*time.Second)
	}

	if cfg.SlowCountThreshold != 3 {
		t.Errorf("DefaultControllerConfig() SlowCountThreshold = %d, want 3", cfg.SlowCountThreshold)
	}

	// Memory-aware defaults
	if !cfg.MemoryAwareEnabled {
		t.Errorf("DefaultControllerConfig() MemoryAwareEnabled = false, want true")
	}

	if cfg.MemoryThresholdMB != 1024 {
		t.Errorf("DefaultControllerConfig() MemoryThresholdMB = %d, want 1024", cfg.MemoryThresholdMB)
	}

	if cfg.MemoryCriticalMB != 512 {
		t.Errorf("DefaultControllerConfig() MemoryCriticalMB = %d, want 512", cfg.MemoryCriticalMB)
	}

	if cfg.WorkerRSSLimitMB != 2048 {
		t.Errorf("DefaultControllerConfig() WorkerRSSLimitMB = %d, want 2048", cfg.WorkerRSSLimitMB)
	}
}

func TestNewController(t *testing.T) {
	cfg := ControllerConfig{
		InitialConcurrency: 2,
		MinConcurrency:     1,
		MaxConcurrency:     4,
		RateLimitBackoff:   30 * time.Second,
		MaxBackoff:         5 * time.Minute,
		SlowThreshold:      10 * time.Second,
		SlowCountThreshold: 3,
	}

	c := NewController(cfg)

	if c == nil {
		t.Fatal("NewController() returned nil")
	}

	if c.maxInFlight != cfg.InitialConcurrency {
		t.Errorf("NewController() maxInFlight = %d, want %d", c.maxInFlight, cfg.InitialConcurrency)
	}

	if c.configuredMax != cfg.MaxConcurrency {
		t.Errorf("NewController() configuredMax = %d, want %d", c.configuredMax, cfg.MaxConcurrency)
	}
}

func TestNewControllerDefaults(t *testing.T) {
	// Test with zero values - should use defaults
	cfg := ControllerConfig{}
	c := NewController(cfg)

	if c.maxInFlight != 2 {
		t.Errorf("NewController() default maxInFlight = %d, want 2", c.maxInFlight)
	}

	if c.config.MinConcurrency != 1 {
		t.Errorf("NewController() default MinConcurrency = %d, want 1", c.config.MinConcurrency)
	}

	if c.config.MaxConcurrency != 2 {
		t.Errorf("NewController() default MaxConcurrency = %d, want 2", c.config.MaxConcurrency)
	}
}

func TestControllerCanSpawn(t *testing.T) {
	cfg := ControllerConfig{
		InitialConcurrency:     2,
		MinConcurrency:         1,
		MaxConcurrency:         4,
		RateLimitBackoff:       30 * time.Second,
		MaxBackoff:             5 * time.Minute,
		SlowThreshold:          10 * time.Second,
		SlowCountThreshold:     3,
		MemoryAwareEnabled:     false, // Disable memory-aware for this test
	}

	c := NewController(cfg)

	// Initially should be able to spawn
	if !c.CanSpawn() {
		t.Error("CanSpawn() = false initially, want true")
	}

	// Start workers up to limit
	for i := 0; i < cfg.InitialConcurrency; i++ {
		c.WorkerStarted()
	}

	// Should not be able to spawn more
	if c.CanSpawn() {
		t.Error("CanSpawn() = true at limit, want false")
	}

	// Finish one worker
	c.WorkerFinished()

	// Should be able to spawn again
	if !c.CanSpawn() {
		t.Error("CanSpawn() = false after worker finished, want true")
	}
}

func TestControllerOnWorkerSignalOK(t *testing.T) {
	cfg := ControllerConfig{
		InitialConcurrency: 2,
		MinConcurrency:     1,
		MaxConcurrency:     4,
		MemoryAwareEnabled: false,
	}

	c := NewController(cfg)
	c.maxInFlight = 2 // Reduce from MaxConcurrency

	// Send OK signal
	c.OnWorkerSignal(SignalOK)

	// Should increase by 1
	if c.maxInFlight != 3 {
		t.Errorf("OnWorkerSignal(SignalOK) maxInFlight = %d, want 3", c.maxInFlight)
	}

	// Send another OK signal
	c.OnWorkerSignal(SignalOK)

	// Should reach MaxConcurrency
	if c.maxInFlight != 4 {
		t.Errorf("OnWorkerSignal(SignalOK) maxInFlight = %d, want 4", c.maxInFlight)
	}

	// Send another OK signal - should stay at MaxConcurrency
	c.OnWorkerSignal(SignalOK)

	if c.maxInFlight != 4 {
		t.Errorf("OnWorkerSignal(SignalOK) maxInFlight = %d, want 4", c.maxInFlight)
	}
}

func TestControllerOnWorkerSignalRateLimited(t *testing.T) {
	cfg := ControllerConfig{
		InitialConcurrency: 4,
		MinConcurrency:     1,
		MaxConcurrency:     8,
		RateLimitBackoff:   5 * time.Second,
		MaxBackoff:         1 * time.Minute,
		MemoryAwareEnabled: false,
	}

	c := NewController(cfg)

	initialMax := c.maxInFlight

	// Send rate limit signal
	c.OnWorkerSignal(SignalRateLimited)

	// Should reduce concurrency by half
	expectedMax := max(1, initialMax/2)
	if c.maxInFlight != expectedMax {
		t.Errorf("OnWorkerSignal(SignalRateLimited) maxInFlight = %d, want %d", c.maxInFlight, expectedMax)
	}

	// Should be in backoff
	if !c.IsInBackoff() {
		t.Error("OnWorkerSignal(SignalRateLimited) IsInBackoff = false, want true")
	}

	// CanSpawn should return false
	if c.CanSpawn() {
		t.Error("CanSpawn() = true during backoff, want false")
	}

	// Wait for backoff to expire - the backoff is doubled (5s * 2 = 10s)
	// Wait longer to ensure it expires
	time.Sleep(11 * time.Second)

	// Should no longer be in backoff
	if c.IsInBackoff() {
		t.Error("IsInBackoff = true after backoff expired, want false")
	}
}

func TestControllerOnWorkerSignalSlowResponse(t *testing.T) {
	cfg := ControllerConfig{
		InitialConcurrency: 4,
		MinConcurrency:     1,
		MaxConcurrency:     8,
		SlowCountThreshold: 3,
		MemoryAwareEnabled: false,
	}

	c := NewController(cfg)
	c.maxInFlight = 4

	// Send slow response signals
	c.OnWorkerSignal(SignalSlowResponse)
	c.OnWorkerSignal(SignalSlowResponse)

	// Should not reduce yet (below threshold)
	if c.maxInFlight != 4 {
		t.Errorf("OnWorkerSignal(SignalSlowResponse) maxInFlight = %d, want 4", c.maxInFlight)
	}

	// Send third slow signal
	c.OnWorkerSignal(SignalSlowResponse)

	// Should reduce by 1
	if c.maxInFlight != 3 {
		t.Errorf("OnWorkerSignal(SignalSlowResponse) maxInFlight = %d, want 3", c.maxInFlight)
	}

	// Counter should be reset
	if c.consecutiveSlow != 0 {
		t.Errorf("OnWorkerSignal(SignalSlowResponse) consecutiveSlow = %d, want 0", c.consecutiveSlow)
	}
}

func TestControllerOnWorkerSignalAPIError(t *testing.T) {
	cfg := ControllerConfig{
		InitialConcurrency: 4,
		MinConcurrency:     1,
		MaxConcurrency:     8,
		MemoryAwareEnabled: false,
	}

	c := NewController(cfg)
	initialMax := c.maxInFlight

	// Send API error signal
	c.OnWorkerSignal(SignalAPIError)

	// Should not reduce concurrency
	if c.maxInFlight != initialMax {
		t.Errorf("OnWorkerSignal(SignalAPIError) maxInFlight = %d, want %d", c.maxInFlight, initialMax)
	}
}

func TestControllerExponentialBackoff(t *testing.T) {
	cfg := ControllerConfig{
		InitialConcurrency: 8,
		MinConcurrency:     1,
		MaxConcurrency:     16,
		RateLimitBackoff:   1 * time.Second,
		MaxBackoff:         10 * time.Second,
		MemoryAwareEnabled: false,
	}

	c := NewController(cfg)

	// First rate limit - 2s backoff (doubles from initial 1s)
	c.OnWorkerSignal(SignalRateLimited)
	if c.currentBackoff != 2*time.Second {
		t.Errorf("First rate limit backoff = %v, want 2s", c.currentBackoff)
	}

	// Second rate limit - 4s backoff (doubles)
	time.Sleep(3 * time.Second) // Wait for first backoff to expire
	c.OnWorkerSignal(SignalRateLimited)
	if c.currentBackoff != 4*time.Second {
		t.Errorf("Second rate limit backoff = %v, want 4s", c.currentBackoff)
	}

	// Third rate limit - 8s backoff (doubles)
	time.Sleep(5 * time.Second) // Wait for second backoff to expire
	c.OnWorkerSignal(SignalRateLimited)
	if c.currentBackoff != 8*time.Second {
		t.Errorf("Third rate limit backoff = %v, want 8s", c.currentBackoff)
	}

	// Fourth rate limit - should cap at MaxBackoff (10s)
	time.Sleep(9 * time.Second) // Wait for third backoff to expire
	c.OnWorkerSignal(SignalRateLimited)
	if c.currentBackoff != 10*time.Second {
		t.Errorf("Fourth rate limit backoff (capped) = %v, want 10s", c.currentBackoff)
	}

	// Fifth rate limit - should stay at MaxBackoff (10s)
	time.Sleep(11 * time.Second) // Wait for fourth backoff to expire
	c.OnWorkerSignal(SignalRateLimited)
	if c.currentBackoff != 10*time.Second {
		t.Errorf("Fifth rate limit backoff (capped) = %v, want 10s", c.currentBackoff)
	}
}

func TestControllerBackoffResetOnSuccess(t *testing.T) {
	cfg := ControllerConfig{
		InitialConcurrency: 4,
		MinConcurrency:     1,
		MaxConcurrency:     8,
		RateLimitBackoff:   30 * time.Second,
		MaxBackoff:         5 * time.Minute,
		MemoryAwareEnabled: false,
	}

	c := NewController(cfg)

	// Trigger rate limit
	c.OnWorkerSignal(SignalRateLimited)

	backedOffBackoff := c.currentBackoff
	if backedOffBackoff == cfg.RateLimitBackoff {
		t.Error("Backoff didn't increase after rate limit")
	}

	// Send OK signal
	c.OnWorkerSignal(SignalOK)

	// Backoff should reset to initial
	if c.currentBackoff != cfg.RateLimitBackoff {
		t.Errorf("Backoff after OK = %v, want %v", c.currentBackoff, cfg.RateLimitBackoff)
	}
}

func TestControllerGetStats(t *testing.T) {
	cfg := ControllerConfig{
		InitialConcurrency: 2,
		MinConcurrency:     1,
		MaxConcurrency:     4,
		MemoryAwareEnabled: false,
	}

	c := NewController(cfg)

	stats := c.GetStats()

	if stats.MaxInFlight != c.maxInFlight {
		t.Errorf("GetStats() MaxInFlight = %d, want %d", stats.MaxInFlight, c.maxInFlight)
	}

	if stats.CurrentInFlight != c.currentInFlight {
		t.Errorf("GetStats() CurrentInFlight = %d, want %d", stats.CurrentInFlight, c.currentInFlight)
	}

	if stats.InBackoff {
		t.Error("GetStats() InBackoff = true initially, want false")
	}

	if stats.ConsecutiveSlow != 0 {
		t.Errorf("GetStats() ConsecutiveSlow = %d, want 0", stats.ConsecutiveSlow)
	}

	// Start a worker
	c.WorkerStarted()

	stats = c.GetStats()
	if stats.CurrentInFlight != 1 {
		t.Errorf("GetStats() CurrentInFlight after start = %d, want 1", stats.CurrentInFlight)
	}
}

func TestControllerReset(t *testing.T) {
	cfg := ControllerConfig{
		InitialConcurrency: 2,
		MinConcurrency:     1,
		MaxConcurrency:     4,
		RateLimitBackoff:   30 * time.Second, // Set explicitly
		MemoryAwareEnabled: false,
	}

	c := NewController(cfg)

	// Modify state
	c.maxInFlight = 4
	c.currentInFlight = 2
	c.consecutiveSlow = 5
	c.rateLimitUntil = time.Now().Add(1 * time.Hour)

	// Reset
	c.Reset()

	// Verify state is reset
	if c.maxInFlight != cfg.InitialConcurrency {
		t.Errorf("Reset() maxInFlight = %d, want %d", c.maxInFlight, cfg.InitialConcurrency)
	}

	if c.currentInFlight != 0 {
		t.Errorf("Reset() currentInFlight = %d, want 0", c.currentInFlight)
	}

	if c.consecutiveSlow != 0 {
		t.Errorf("Reset() consecutiveSlow = %d, want 0", c.consecutiveSlow)
	}

	if !c.rateLimitUntil.IsZero() {
		t.Error("Reset() rateLimitUntil not zero")
	}

	if c.currentBackoff != cfg.RateLimitBackoff {
		t.Errorf("Reset() currentBackoff = %v, want %v", c.currentBackoff, cfg.RateLimitBackoff)
	}
}

func TestControllerGetCurrentConcurrency(t *testing.T) {
	cfg := ControllerConfig{
		InitialConcurrency: 2,
		MinConcurrency:     1,
		MaxConcurrency:     4,
		MemoryAwareEnabled: false,
	}

	c := NewController(cfg)

	if c.GetCurrentConcurrency() != cfg.InitialConcurrency {
		t.Errorf("GetCurrentConcurrency() = %d, want %d", c.GetCurrentConcurrency(), cfg.InitialConcurrency)
	}
}

func TestControllerGetCurrentInFlight(t *testing.T) {
	cfg := ControllerConfig{
		InitialConcurrency: 2,
		MinConcurrency:     1,
		MaxConcurrency:     4,
		MemoryAwareEnabled: false,
	}

	c := NewController(cfg)

	if c.GetCurrentInFlight() != 0 {
		t.Errorf("GetCurrentInFlight() = %d, want 0", c.GetCurrentInFlight())
	}

	c.WorkerStarted()
	if c.GetCurrentInFlight() != 1 {
		t.Errorf("GetCurrentInFlight() after start = %d, want 1", c.GetCurrentInFlight())
	}

	c.WorkerFinished()
	if c.GetCurrentInFlight() != 0 {
		t.Errorf("GetCurrentInFlight() after finish = %d, want 0", c.GetCurrentInFlight())
	}
}

func TestControllerGetBackoffDeadline(t *testing.T) {
	cfg := ControllerConfig{
		InitialConcurrency: 2,
		MinConcurrency:     1,
		MaxConcurrency:     4,
		RateLimitBackoff:   30 * time.Second,
		MaxBackoff:         5 * time.Minute,
		MemoryAwareEnabled: false,
	}

	c := NewController(cfg)

	// Initially no backoff
	deadline := c.GetBackoffDeadline()
	if !deadline.IsZero() {
		t.Errorf("GetBackoffDeadline() = %v, want zero time", deadline)
	}

	// Trigger rate limit
	c.OnWorkerSignal(SignalRateLimited)

	deadline = c.GetBackoffDeadline()
	if deadline.IsZero() {
		t.Error("GetBackoffDeadline() = zero time after rate limit, want non-zero")
	}

	if deadline.Before(time.Now()) {
		t.Error("GetBackoffDeadline() is in the past")
	}
}
