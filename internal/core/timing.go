package core

import "time"

// TaskTiming is when a Task crossed each lifecycle boundary and how long it
// spent in each runtime phase, read from the run's Clock (§39 optimization
// data). The runtime stamps it; callers never assemble it.
//
// For a Task that ran submitted work the boundaries are ordered
// DeclaredAt ≤ SubmittedAt ≤ EligibleAt ≤ StartedAt ≤ SettledAt, so its
// Total splits exactly into AwaitingDefinition, Queued, and Running. A
// zero boundary means the Task never crossed it (a Task resolved on the
// caller's stack never submits work), and every span touching it is zero.
type TaskTiming struct {
	// DeclaredAt is when the Task was declared.
	DeclaredAt time.Time
	// SubmittedAt is when Define submitted the Task's work.
	SubmittedAt time.Time
	// EligibleAt is when the Task's work was submitted and every
	// predecessor had settled: the scheduler could start it, capacity
	// permitting.
	EligibleAt time.Time
	// StartedAt is when the Task's work started: the scheduler starting its
	// submitted work, or, for a Task that never submitted work, its entry
	// into Running.
	StartedAt time.Time
	// SettledAt is when the Task reached a terminal state.
	SettledAt time.Time

	// Definition is time inside the Define callback. Evo enters a callback
	// only when no current proof exists (§9), so this is time spent
	// revalidating uncertain definitions.
	Definition PhaseTime
	// Evidence is time evaluating Verify, before and after the callback.
	Evidence PhaseTime
	// Provenance is time fingerprinting Basis inputs and consulting and
	// recording the operation manifest: checking provenance. It counts one
	// entry per tracked operation (File, Exec).
	Provenance PhaseTime
	// TrackedState is time inspecting tracked resources live on disk,
	// including digesting a fresh output. It counts one entry per tracked
	// operation that inspected anything.
	TrackedState PhaseTime
}

// PhaseTime is how often a Task entered one runtime phase and how long it
// spent there in total. Provenance and TrackedState run inside the Define
// callback, so their time is also part of Definition.
type PhaseTime struct {
	Entries  int
	Duration time.Duration
}

// AwaitingDefinition is declaration to submission: the caller's own gap
// before calling Define. It is never a wait on Evo.
func (t TaskTiming) AwaitingDefinition() time.Duration { return span(t.DeclaredAt, t.SubmittedAt) }

// DependencyWait is submitted work waiting on predecessors: submission to
// eligibility.
func (t TaskTiming) DependencyWait() time.Duration { return span(t.SubmittedAt, t.EligibleAt) }

// SchedulerWait is eligible work waiting for scheduler capacity:
// eligibility to start.
func (t TaskTiming) SchedulerWait() time.Duration { return span(t.EligibleAt, t.StartedAt) }

// Queued is the whole wait between submission and start: DependencyWait
// plus SchedulerWait.
func (t TaskTiming) Queued() time.Duration { return t.DependencyWait() + t.SchedulerWait() }

// Running is start to settlement.
func (t TaskTiming) Running() time.Duration { return span(t.StartedAt, t.SettledAt) }

// Total is the Task's whole lifetime: declaration to settlement.
func (t TaskTiming) Total() time.Duration { return span(t.DeclaredAt, t.SettledAt) }

// span is to-from when both boundaries were crossed in order, else zero:
// an uncrossed boundary is no time spent, and a span is never negative.
func span(from, to time.Time) time.Duration {
	if from.IsZero() || to.IsZero() || to.Before(from) {
		return 0
	}
	return to.Sub(from)
}
