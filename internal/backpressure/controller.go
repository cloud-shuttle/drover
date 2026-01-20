// Package backpressure provides adaptive concurrency control for Drover
package backpressure

import (
	"log"
	"sync"
	"time"

	"github.com/cloud-shuttle/drover/internal/memory"
)

// WorkerSignal represents downstream health signals from workers
type WorkerSignal string

const (
	SignalOK          WorkerSignal = "ok"           // Normal execution
	SignalRateLimited WorkerSignal = "rate_limited" // Rate limit detected
	SignalSlowResponse WorkerSignal = "slow_response" // Slow response
	SignalAPIError    WorkerSignal = "api_error"    // Transient API error
)

// Controller manages adaptive concurrency based on downstream health
type Controller struct {
	mu sync.RWMutex

	// Configuration
	config ControllerConfig

	// State
	rateLimitUntil    time.Time // When to resume after rate limit
	consecutiveSlow   int       // Count of consecutive slow responses
	maxInFlight       int       // Current max concurrent workers
	currentInFlight   int       // Currently active workers
	configuredMax     int       // Initial configured maximum

	// Backoff state
	currentBackoff    time.Duration
	backoffMultiplier float64
}

// ControllerConfig holds backpressure controller configuration
type ControllerConfig struct {
	InitialConcurrency int           // Starting concurrency level
	MinConcurrency     int           // Minimum concurrency (never go below)
	MaxConcurrency     int           // Maximum concurrency (never exceed)
	RateLimitBackoff   time.Duration // Initial backoff on rate limit
	MaxBackoff         time.Duration // Maximum backoff duration
	SlowThreshold      time.Duration // Response time considered slow
	SlowCountThreshold int           // Consecutive slow responses before reducing

	// Memory-aware settings (drover-mem-6)
	MemoryAwareEnabled    bool  // Enable memory-aware spawning
	MemoryThresholdMB     int64 // Minimum available MB before throttling
	MemoryCriticalMB      int64 // Critical memory threshold - stop spawning
	WorkerRSSLimitMB      int64 // Per-worker RSS limit in MB
}

// DefaultControllerConfig returns default backpressure controller configuration
func DefaultControllerConfig() ControllerConfig {
	return ControllerConfig{
		InitialConcurrency: 2,
		MinConcurrency:     1,
		MaxConcurrency:     4,
		RateLimitBackoff:   30 * time.Second,
		MaxBackoff:         5 * time.Minute,
		SlowThreshold:      10 * time.Second,
		SlowCountThreshold: 3,

		// Memory-aware defaults
		MemoryAwareEnabled:   true,
		MemoryThresholdMB:    1024,  // Throttle if less than 1GB available
		MemoryCriticalMB:     512,   // Stop spawning if less than 512MB available
		WorkerRSSLimitMB:     2048,  // Each worker limited to 2GB RSS
	}
}

// NewController creates a new backpressure controller
func NewController(cfg ControllerConfig) *Controller {
	if cfg.InitialConcurrency <= 0 {
		cfg.InitialConcurrency = 2
	}
	if cfg.MinConcurrency <= 0 {
		cfg.MinConcurrency = 1
	}
	if cfg.MaxConcurrency < cfg.MinConcurrency {
		cfg.MaxConcurrency = cfg.MinConcurrency * 2
	}
	if cfg.RateLimitBackoff == 0 {
		cfg.RateLimitBackoff = 30 * time.Second
	}
	if cfg.MaxBackoff == 0 {
		cfg.MaxBackoff = 5 * time.Minute
	}
	if cfg.SlowThreshold == 0 {
		cfg.SlowThreshold = 10 * time.Second
	}
	if cfg.SlowCountThreshold == 0 {
		cfg.SlowCountThreshold = 3
	}

	return &Controller{
		config:            cfg,
		maxInFlight:       cfg.InitialConcurrency,
		configuredMax:     cfg.MaxConcurrency,
		currentBackoff:    cfg.RateLimitBackoff,
		backoffMultiplier: 2.0,
	}
}

// OnWorkerSignal processes a worker signal and adjusts backpressure state
func (c *Controller) OnWorkerSignal(signal WorkerSignal) {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch signal {
	case SignalRateLimited:
		c.handleRateLimit()

	case SignalSlowResponse:
		c.handleSlowResponse()

	case SignalAPIError:
		c.handleAPIError()

	case SignalOK:
		c.handleOK()
	}
}

// handleRateLimit reduces concurrency and applies exponential backoff
func (c *Controller) handleRateLimit() {
	// Calculate new backoff
	newBackoff := time.Duration(float64(c.currentBackoff) * c.backoffMultiplier)
	if newBackoff > c.config.MaxBackoff {
		newBackoff = c.config.MaxBackoff
	}
	c.currentBackoff = newBackoff

	// Set rate limit deadline
	c.rateLimitUntil = time.Now().Add(c.currentBackoff)

	// Reduce concurrency by half (minimum of 1)
	c.maxInFlight = max(c.config.MinConcurrency, c.maxInFlight/2)

	log.Printf("[backpressure] rate limit detected, backing off for %v, concurrency reduced to %d",
		c.currentBackoff, c.maxInFlight)
}

// handleSlowResponse tracks consecutive slow responses and may reduce concurrency
func (c *Controller) handleSlowResponse() {
	c.consecutiveSlow++

	if c.consecutiveSlow >= c.config.SlowCountThreshold {
		// Reduce concurrency by 1
		if c.maxInFlight > c.config.MinConcurrency {
			c.maxInFlight--
			log.Printf("[backpressure] %d consecutive slow responses, concurrency reduced to %d",
				c.consecutiveSlow, c.maxInFlight)
		}
		// Reset counter after taking action
		c.consecutiveSlow = 0
	}
}

// handleAPIError treats API errors similar to slow responses
func (c *Controller) handleAPIError() {
	// Don't reduce concurrency for transient API errors
	// They may be network issues, not overload
	log.Printf("[backpressure] API error detected (not reducing concurrency)")
}

// handleOK resets negative state and gradually increases capacity
func (c *Controller) handleOK() {
	// Reset slow counter
	c.consecutiveSlow = 0

	// Gradually increase concurrency if we were reduced
	if c.maxInFlight < c.config.MaxConcurrency {
		c.maxInFlight++
		if c.maxInFlight == c.config.MaxConcurrency {
			log.Printf("[backpressure] recovered to full concurrency: %d", c.maxInFlight)
		}
	}

	// Reset backoff on successful requests
	c.currentBackoff = c.config.RateLimitBackoff
}

// CanSpawn checks if a new worker can be spawned based on backpressure state
func (c *Controller) CanSpawn() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Check if we're in backoff period
	if time.Now().Before(c.rateLimitUntil) {
		return false
	}

	// Check if we're at concurrency limit
	if c.currentInFlight >= c.maxInFlight {
		return false
	}

	// Memory-aware spawning (drover-mem-6)
	if c.config.MemoryAwareEnabled {
		// Check system memory availability
		if sysMem, err := memory.GetSystemMemory(); err == nil {
			// Critical memory threshold - stop spawning
			if sysMem.AvailableMB < c.config.MemoryCriticalMB {
				log.Printf("[backpressure] critical memory: %dMB available < %dMB threshold, stopping spawns",
					sysMem.AvailableMB, c.config.MemoryCriticalMB)
				return false
			}

			// Memory threshold - consider throttling
			if sysMem.AvailableMB < c.config.MemoryThresholdMB {
				// Only allow spawn if we have significant headroom
				estimatedUsageMB := int64(c.currentInFlight) * c.config.WorkerRSSLimitMB
				if estimatedUsageMB > sysMem.AvailableMB-c.config.MemoryCriticalMB {
					log.Printf("[backpressure] memory constrained: %dMB available, %dMB estimated usage, throttling",
						sysMem.AvailableMB, estimatedUsageMB)
					return false
				}
			}
		}
	}

	return true
}

// WorkerStarted increments the in-flight counter
func (c *Controller) WorkerStarted() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.currentInFlight++
}

// WorkerFinished decrements the in-flight counter
func (c *Controller) WorkerFinished() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.currentInFlight > 0 {
		c.currentInFlight--
	}
}

// GetCurrentConcurrency returns the current max concurrency
func (c *Controller) GetCurrentConcurrency() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.maxInFlight
}

// GetCurrentInFlight returns the current number of in-flight workers
func (c *Controller) GetCurrentInFlight() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.currentInFlight
}

// GetBackoffDeadline returns when the backoff period ends
func (c *Controller) GetBackoffDeadline() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.rateLimitUntil
}

// IsInBackoff returns true if currently in backoff period
func (c *Controller) IsInBackoff() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return time.Now().Before(c.rateLimitUntil)
}

// GetStats returns current controller statistics
type Stats struct {
	MaxInFlight     int       // Current max concurrency
	CurrentInFlight int       // Currently active workers
	BackoffUntil    time.Time // When backoff ends
	InBackoff       bool      // Currently in backoff
	ConsecutiveSlow int       // Count of slow responses
}

// GetStats returns current statistics
func (c *Controller) GetStats() Stats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return Stats{
		MaxInFlight:     c.maxInFlight,
		CurrentInFlight: c.currentInFlight,
		BackoffUntil:    c.rateLimitUntil,
		InBackoff:       time.Now().Before(c.rateLimitUntil),
		ConsecutiveSlow: c.consecutiveSlow,
	}
}

// Reset resets the controller to initial state
func (c *Controller) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.maxInFlight = c.config.InitialConcurrency
	c.currentInFlight = 0
	c.consecutiveSlow = 0
	c.rateLimitUntil = time.Time{}
	c.currentBackoff = c.config.RateLimitBackoff

	log.Printf("[backpressure] controller reset")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
