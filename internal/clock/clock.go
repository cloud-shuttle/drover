// Package clock provides a mockable time abstraction for deterministic testing.
package clock

import "time"

// Clock abstracts time.Now() for testability.
type Clock interface {
	Now() time.Time
}

// RealClock uses the real system time.
type RealClock struct{}

// Now returns the current time.
func (RealClock) Now() time.Time { return time.Now() }

// MockClock returns a fixed or controlled time for testing.
type MockClock struct {
	current time.Time
}

// NewMockClock creates a MockClock starting at the given time.
func NewMockClock(t time.Time) *MockClock {
	return &MockClock{current: t}
}

// Now returns the mock clock's current time.
func (m *MockClock) Now() time.Time { return m.current }

// Add advances the mock clock by the given duration.
func (m *MockClock) Add(d time.Duration) { m.current = m.current.Add(d) }
