package engine

import "time"

// TimeSource provides the current time for deterministic tests.
// Option constructor is withClock(TimeSource) to match the public aPI examples.
type TimeSource interface {
	Now() time.Time
}

// systemClock uses the real wall clock.
type systemClock struct{}

// Now returns the system time.
func (systemClock) Now() time.Time { return time.Now() }

// fixedClock always returns the same instant.
type fixedClock struct {
	T time.Time
}

// Now returns the fixed instant.
func (c fixedClock) Now() time.Time { return c.T }
