// Package clock is the one place Evo reads the wall clock or waits on it.
// Every other internal package takes a Clock for domain time, or System() for
// the few waits that must be real (spinner cadence, paint cost, lock backoff).
package clock

import "time"

// Clock provides the current time. Tests inject a fixed or advanceable one.
type Clock interface {
	Now() time.Time
}

// Scheduler is Clock's optional companion capability: schedule fn to run once
// at least d has elapsed on that clock, and return a func that cancels fn if it
// has not fired. A Clock that does not implement Scheduler (such as Fixed)
// simply never arms scheduled work, rather than arming broken work.
type Scheduler interface {
	AfterFunc(d time.Duration, fn func()) func()
}

// Wall is the real wall clock: time, timers, tickers and sleep.
type Wall struct{}

// System returns the real wall clock.
func System() Wall { return Wall{} }

// Now returns the system time.
func (Wall) Now() time.Time { return time.Now() }

// Since returns the wall-clock time elapsed since t.
func (Wall) Since(t time.Time) time.Duration { return time.Since(t) }

// Sleep pauses the calling goroutine for d.
func (Wall) Sleep(d time.Duration) { time.Sleep(d) }

// After returns a channel that receives once d has elapsed.
func (Wall) After(d time.Duration) <-chan time.Time { return time.After(d) }

// NewTimer returns a timer that fires once after d.
func (Wall) NewTimer(d time.Duration) *time.Timer { return time.NewTimer(d) }

// NewTicker returns a ticker that fires every d.
func (Wall) NewTicker(d time.Duration) *time.Ticker { return time.NewTicker(d) }

// AfterFunc implements Scheduler on the wall clock. The returned func cancels
// fn if it has not already fired.
func (Wall) AfterFunc(d time.Duration, fn func()) func() {
	t := time.AfterFunc(d, fn)
	return func() { t.Stop() }
}

// Fixed always returns the same instant.
type Fixed struct {
	T time.Time
}

// Now returns the fixed instant.
func (c Fixed) Now() time.Time { return c.T }
