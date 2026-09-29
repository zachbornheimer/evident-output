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

// Scheduler is TimeSource's optional companion capability: schedule fn to
// run once at least d has elapsed on that clock. Only the plain/
// non-interactive §40 heartbeat (plain_heartbeat.go) depends on it — a
// TimeSource that does not implement Scheduler (e.g. fixedClock) simply
// never arms a heartbeat, rather than a broken one. systemClock schedules a
// real timer; testkit.Clock fires deterministically when its fake time is
// advanced past the deadline, so the heartbeat is testable without a sleep.
type Scheduler interface {
	AfterFunc(d time.Duration, fn func()) func()
}

// AfterFunc implements Scheduler for the real wall clock. The returned func
// cancels fn if it has not already fired.
func (systemClock) AfterFunc(d time.Duration, fn func()) func() {
	t := time.AfterFunc(d, fn)
	return func() { t.Stop() }
}
