package core

import "time"

// TaskTiming is when a Task crossed each lifecycle boundary, read from the
// run's Clock (§39 optimization data). The runtime stamps it; callers never
// assemble it. A zero field means the Task never crossed that boundary — a
// NotStarted Task has no StartedAt — and every span touching it is zero.
type TaskTiming struct {
	// DeclaredAt is when the Task was declared.
	DeclaredAt time.Time
	// EligibleAt is when every predecessor had settled and the scheduler
	// could have started the Task, capacity permitting.
	EligibleAt time.Time
	// StartedAt is when the Task entered Running.
	StartedAt time.Time
	// SettledAt is when the Task reached a terminal state.
	SettledAt time.Time
}

// DependencyWait is the time spent waiting on predecessors: declaration to
// eligibility.
func (t TaskTiming) DependencyWait() time.Duration { return span(t.DeclaredAt, t.EligibleAt) }

// SchedulerWait is the time spent eligible but waiting for scheduler
// capacity: eligibility to start.
func (t TaskTiming) SchedulerWait() time.Duration { return span(t.EligibleAt, t.StartedAt) }

// Queued is the whole wait before the Task ran: declaration to start.
func (t TaskTiming) Queued() time.Duration { return span(t.DeclaredAt, t.StartedAt) }

// Running is the time spent executing: start to settlement.
func (t TaskTiming) Running() time.Duration { return span(t.StartedAt, t.SettledAt) }

// Total is the Task's whole lifetime: declaration to settlement.
func (t TaskTiming) Total() time.Duration { return span(t.DeclaredAt, t.SettledAt) }

// span is to-from when both boundaries were crossed in order, else zero: an
// uncrossed boundary or an out-of-order pair (Doing promotes Running before
// Define makes the Task eligible) is no time spent waiting, never negative.
func span(from, to time.Time) time.Duration {
	if from.IsZero() || to.IsZero() || to.Before(from) {
		return 0
	}
	return to.Sub(from)
}
