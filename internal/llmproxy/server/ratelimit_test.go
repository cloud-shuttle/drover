package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cloud-shuttle/drover/internal/llmproxy"
)

func TestNewRateLimiter(t *testing.T) {
	cfg := llmproxy.RateLimitConfig{
		RequestsPerMinute: 60,
		TokensPerMinute:    90000,
		Enabled:            true,
	}

	limiter := NewRateLimiter(cfg)

	if limiter == nil {
		t.Fatal("NewRateLimiter() returned nil")
	}

	if limiter.config.RequestsPerMinute != 60 {
		t.Errorf("RateLimiter.config.RequestsPerMinute = %v, want 60", limiter.config.RequestsPerMinute)
	}
}

func TestRateLimiterAllowRequest(t *testing.T) {
	cfg := llmproxy.RateLimitConfig{
		RequestsPerMinute: 2,
		TokensPerMinute:    1000,
		Enabled:            true,
	}

	limiter := NewRateLimiter(cfg)

	// First request should be allowed
	allowed := limiter.allowRequest("key-1")
	if !allowed {
		t.Error("First request should be allowed")
	}

	// Second request should be allowed
	allowed = limiter.allowRequest("key-1")
	if !allowed {
		t.Error("Second request should be allowed")
	}

	// Third request should exceed the limit
	allowed = limiter.allowRequest("key-1")
	if allowed {
		t.Error("Third request should exceed rate limit")
	}
}

func TestRateLimiterAllowRequestDifferentKeys(t *testing.T) {
	cfg := llmproxy.RateLimitConfig{
		RequestsPerMinute: 1,
		Enabled:            true,
	}

	limiter := NewRateLimiter(cfg)

	// Each API key should have its own limit
	allowed1 := limiter.allowRequest("key-1")
	allowed2 := limiter.allowRequest("key-2")

	if !allowed1 {
		t.Error("First key request should be allowed")
	}

	if !allowed2 {
		t.Error("Second key request should be allowed")
	}

	// Second request with same key should be rate limited
	allowed3 := limiter.allowRequest("key-1")
	if allowed3 {
		t.Error("Second request with same key should exceed rate limit")
	}
}

func TestRateLimiterAllowTokens(t *testing.T) {
	cfg := llmproxy.RateLimitConfig{
		TokensPerMinute: 1000,
		Enabled:          true,
	}

	limiter := NewRateLimiter(cfg)

	// First request with 500 tokens
	allowed := limiter.AllowTokens("key-1", 500)
	if !allowed {
		t.Error("Request with 500 tokens should be allowed")
	}

	// Second request with 600 tokens should exceed limit
	allowed = limiter.AllowTokens("key-1", 600)
	if allowed {
		t.Error("Request with 1100 total tokens should exceed token rate limit")
	}
}

func TestRateLimiterNoConfig(t *testing.T) {
	cfg := llmproxy.RateLimitConfig{
		RequestsPerMinute: 0,
		TokensPerMinute:    0,
		Enabled:            false,
	}

	limiter := NewRateLimiter(cfg)

	// With no limits, all requests should be allowed
	for i := 0; i < 100; i++ {
		allowed := limiter.allowRequest("key-1")
		if !allowed {
			t.Errorf("Request %d should be allowed when rate limiting is disabled", i)
		}
	}

	// All token requests should be allowed
	for i := 0; i < 100; i++ {
		allowed := limiter.AllowTokens("key-1", 1000)
		if !allowed {
			t.Errorf("Token request %d should be allowed when rate limiting is disabled", i)
		}
	}
}

func TestTokenBucket(t *testing.T) {
	bucket := &tokenBucket{
		maxTokens:  10,
		tokens:     10,
		refillRate: 1,
		lastRefill: time.Now(),
	}

	// Consume tokens
	if !bucket.consume(5) {
		t.Error("consume(5) should succeed")
	}

	if bucket.tokens < 4.9 || bucket.tokens > 5.1 {
		t.Errorf("After consume(5), tokens = %v, want ~5", bucket.tokens)
	}

	// Consume more than available
	if bucket.consume(6) {
		t.Error("consume(6) should fail when only 5 tokens available")
	}

	// Consume exact amount
	if !bucket.consume(5) {
		t.Error("consume(5) should succeed when 5 tokens available")
	}

	// Allow for small floating point errors
	if bucket.tokens > 0.001 {
		t.Errorf("After consume(5), tokens = %v, want ~0", bucket.tokens)
	}
}

func TestTokenBucketRefill(t *testing.T) {
	bucket := &tokenBucket{
		maxTokens:  10,
		tokens:     0,
		refillRate: 10, // 10 tokens per second
		lastRefill: time.Now().Add(-1 * time.Second),
	}

	// Trigger refill by consuming
	bucket.consume(0)

	if bucket.tokens != 10 {
		t.Errorf("After refill, tokens = %v, want 10", bucket.tokens)
	}

	// Should not exceed capacity
	bucket.lastRefill = time.Now().Add(-2 * time.Second)
	bucket.consume(0)

	if bucket.tokens != 10 {
		t.Errorf("Tokens should not exceed capacity, got %v, want 10", bucket.tokens)
	}
}

func TestRateLimiterMiddleware(t *testing.T) {
	cfg := llmproxy.RateLimitConfig{
		RequestsPerMinute: 1,
		Enabled:            true,
	}

	limiter := NewRateLimiter(cfg)

	// Create a test handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Wrap with middleware
	middleware := limiter.Middleware(handler)

	// First request should succeed
	req1 := httptest.NewRequest("GET", "/test", nil)
	req1.Header.Set("Authorization", "Bearer test-key")
	rec1 := httptest.NewRecorder()
	middleware.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Errorf("First request should succeed, got status %d", rec1.Code)
	}

	// Second request with same key should be rate limited
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.Header.Set("Authorization", "Bearer test-key")
	rec2 := httptest.NewRecorder()
	middleware.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusTooManyRequests {
		t.Errorf("Second request should be rate limited, got status %d", rec2.Code)
	}

	// Different key should succeed
	req3 := httptest.NewRequest("GET", "/test", nil)
	req3.Header.Set("Authorization", "Bearer different-key")
	rec3 := httptest.NewRecorder()
	middleware.ServeHTTP(rec3, req3)

	if rec3.Code != http.StatusOK {
		t.Errorf("Request with different key should succeed, got status %d", rec3.Code)
	}
}

func TestRateLimiterMiddlewareDisabled(t *testing.T) {
	cfg := llmproxy.RateLimitConfig{
		RequestsPerMinute: 1,
		Enabled:            false,
	}

	limiter := NewRateLimiter(cfg)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	middleware := limiter.Middleware(handler)

	// Even with same key, all requests should succeed when disabled
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer test-key")
		rec := httptest.NewRecorder()
		middleware.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Request %d should succeed when rate limiting is disabled, got status %d", i, rec.Code)
		}
	}
}

func TestRateLimiterMiddlewareByIP(t *testing.T) {
	cfg := llmproxy.RateLimitConfig{
		RequestsPerMinute: 1,
		Enabled:            true,
	}

	limiter := NewRateLimiter(cfg)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	middleware := limiter.Middleware(handler)

	// Request without auth header should use IP
	req1 := httptest.NewRequest("GET", "/test", nil)
	req1.RemoteAddr = "192.168.1.1:1234"
	rec1 := httptest.NewRecorder()
	middleware.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Errorf("First request from IP should succeed, got status %d", rec1.Code)
	}

	// Second request from same IP (same port, RemoteAddr is used as-is) should be rate limited
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.RemoteAddr = "192.168.1.1:1234"
	rec2 := httptest.NewRecorder()
	middleware.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusTooManyRequests {
		t.Errorf("Second request from same IP should be rate limited, got status %d", rec2.Code)
	}

	// Different IP should succeed
	req3 := httptest.NewRequest("GET", "/test", nil)
	req3.RemoteAddr = "192.168.1.2:1234"
	rec3 := httptest.NewRecorder()
	middleware.ServeHTTP(rec3, req3)

	if rec3.Code != http.StatusOK {
		t.Errorf("Request from different IP should succeed, got status %d", rec3.Code)
	}
}

func TestGetClientKey(t *testing.T) {
	tests := []struct {
		name        string
		authHeader  string
		remoteAddr  string
		wantPrefix  string
	}{
		{
			name:        "API key from Authorization header",
			authHeader:  "Bearer sk-1234567890",
			remoteAddr:  "",
			wantPrefix:  "api:Bearer sk-1234567890",
		},
		{
			name:        "IP address when no auth",
			authHeader:  "",
			remoteAddr:  "192.168.1.1:1234",
			wantPrefix:  "ip:192.168.1.1:1234",
		},
		{
			name:        "unknown when neither available",
			authHeader:  "",
			remoteAddr:  "",
			wantPrefix:  "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			req.RemoteAddr = tt.remoteAddr

			key := getClientKey(req)
			if key != tt.wantPrefix {
				t.Errorf("getClientKey() = %v, want %v", key, tt.wantPrefix)
			}
		})
	}
}

func TestRateLimitConfigDefaults(t *testing.T) {
	cfg := llmproxy.RateLimitConfig{}

	if cfg.RequestsPerMinute != 0 {
		t.Errorf("Default RequestsPerMinute = %v, want 0", cfg.RequestsPerMinute)
	}

	if cfg.TokensPerMinute != 0 {
		t.Errorf("Default TokensPerMinute = %v, want 0", cfg.TokensPerMinute)
	}
}
