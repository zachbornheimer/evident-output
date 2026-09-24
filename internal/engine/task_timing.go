package engine

import (
	"time"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/wire"
)

// Lifecycle timing (§39): each boundary is stamped once, from the run's
// Clock, at the moment the runtime itself moves the Task across it. The
// stamps are the one runtime truth TaskSnapshot.Timing, Conclusion.Metrics,
// the final JSON, and the JSONL stream all read — no projection keeps a
// second stopwatch.

// markDeclared stamps declaration.
func (st *taskState) markDeclared(now time.Time) { st.timing.DeclaredAt = now }

// markStarted stamps entry into Running.
func (st *taskState) markStarted(now time.Time) {
	if st.timing.StartedAt.IsZero() {
		st.timing.StartedAt = now
	}
}

// markSettled stamps the first terminal state; a later re-resolution
// (misuse) never moves it.
func (st *taskState) markSettled(now time.Time) {
	if st.timing.SettledAt.IsZero() {
		st.timing.SettledAt = now
	}
}

// noteEligibleLocked records that cand's predecessors have all settled:
// it stamps EligibleAt and emits task.eligible (§38), once per Task, at the
// moment eligibility is observed — before any wait for scheduler capacity,
// so DependencyWait and SchedulerWait stay distinct.
func (o *Output) noteEligibleLocked(cand *taskState) {
	if !cand.timing.EligibleAt.IsZero() {
		return
	}
	cand.timing.EligibleAt = o.cfg.clock.Now()
	o.emitWireEventLocked(wire.EventTaskEligible, cand.id, nil)
}

// noteNewlyEligible observes eligibility for every queued Task, whether or
// not a scheduler slot is free. kick calls it on every submission and
// settlement — the only events that can make a Task eligible.
func (o *Output) noteNewlyEligible() {
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, cand := range o.tasks {
		if cand.timing.EligibleAt.IsZero() && o.claimableLocked(cand) {
			o.noteEligibleLocked(cand)
		}
	}
}

// settleUnstampedLocked stamps SettledAt on every terminal Task Finish's
// own sweeps resolved (Incomplete, NotStarted, amnesty Done, abnormal
// Cancelled): Finish is the moment those Tasks settled.
func (o *Output) settleUnstampedLocked() {
	now := o.cfg.clock.Now()
	for _, t := range o.tasks {
		if isSettled(t.state) {
			t.markSettled(now)
		}
	}
}

// isSettled reports whether a Task has stopped changing: terminal, or
// Incomplete (Finish's reading of a Task no verb ever resolved).
func isSettled(s EntityState) bool {
	return s == Incomplete || core.IsTerminalTask(s)
}
