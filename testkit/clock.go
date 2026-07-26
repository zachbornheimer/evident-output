// Package testkit provides deterministic clocks, screens, and assertions for evo tests.
package testkit

import (
	"sync"
	"time"
)

// Clock is an advanceable fake clock for evo.TimeSource injection.
type Clock struct {
	mu sync.Mutex
	t  time.Time
}

// NewClock returns a clock fixed at a deterministic instant.
func NewClock() *Clock {
	return &Clock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
}

// Now implements evo.TimeSource.
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

// Advance moves the clock forward.
func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}
