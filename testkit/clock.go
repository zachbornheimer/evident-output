// Package testkit provides deterministic clocks, screens, and assertions for evo tests.
package testkit

import (
	"sync"
	"time"
)

// Clock is an advanceable fake clock for evo.TimeSource injection.
type Clock struct {
	mu     sync.Mutex
	t      time.Time
	timers []*pendingTimer
}

// pendingTimer is one scheduled callback armed via Clock.AfterFunc.
type pendingTimer struct {
	deadline time.Time
	fn       func()
	fired    bool
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

// AfterFunc implements engine.Scheduler for the fake clock: fn runs once
// Advance moves this clock's time to or past deadline (Now()+d) — the
// deterministic counterpart of time.AfterFunc for code under test (e.g. the
// §40 plain heartbeat) that schedules future work off the injected
// TimeSource instead of a real wall timer. The returned func cancels fn if
// it has not fired yet.
func (c *Clock) AfterFunc(d time.Duration, fn func()) func() {
	c.mu.Lock()
	pt := &pendingTimer{deadline: c.t.Add(d), fn: fn}
	c.timers = append(c.timers, pt)
	c.mu.Unlock()
	return func() {
		c.mu.Lock()
		pt.fired = true
		c.mu.Unlock()
	}
}

// Advance moves the clock forward by d and fires — in declaration order,
// synchronously on the calling goroutine, outside the lock — every timer
// armed via AfterFunc whose deadline is now due. A fired callback is safe to
// call AfterFunc again on this same Clock (e.g. to reschedule itself)
// without deadlocking, because the lock is released before any callback
// runs.
func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	now := c.t
	var due []func()
	remaining := c.timers[:0]
	for _, pt := range c.timers {
		if pt.fired {
			continue
		}
		if !pt.deadline.After(now) {
			pt.fired = true
			due = append(due, pt.fn)
			continue
		}
		remaining = append(remaining, pt)
	}
	c.timers = remaining
	c.mu.Unlock()
	for _, fn := range due {
		fn()
	}
}
