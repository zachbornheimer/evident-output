package evo

import "time"

// TimeSource provides the current time for deterministic tests.
// Option constructor is Clock(TimeSource) to match the public API examples.
type TimeSource interface {
	Now() time.Time
}

// SystemClock uses the real wall clock.
type SystemClock struct{}

// Now returns the system time.
func (SystemClock) Now() time.Time { return time.Now() }

// FixedClock always returns the same instant.
type FixedClock struct {
	T time.Time
}

// Now returns the fixed instant.
func (c FixedClock) Now() time.Time { return c.T }
