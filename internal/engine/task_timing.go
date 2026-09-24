package engine

import (
	"time"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/wire"
)

// Lifecycle timing (§39): each boundary is stamped from the run's Clock at
// the moment the runtime itself moves the Task across it. The stamps are
// the one runtime truth TaskSnapshot.Timing, Conclusion.Metrics, the final
// JSON, and the JSONL stream all read.

// markDeclared stamps declaration.
func (st *taskState) markDeclared(now time.Time) { st.timing.DeclaredAt = now }

// markSubmitted stamps Define's submission of the Task's work.
func (st *taskState) markSubmitted(now time.Time) { st.timing.SubmittedAt = now }

// markStarted stamps entry into Running. It is the start boundary only for
// a Task that never submits work; markClaimed overrides it when the
// scheduler starts submitted work.
func (st *taskState) markStarted(now time.Time) {
	if st.timing.StartedAt.IsZero() {
		st.timing.StartedAt = now
	}
}

// markClaimed stamps the scheduler starting the Task's submitted work. A
// Doing before Define promotes the row to Running earlier, but the work
// itself starts here, so this stamp replaces that one.
func (st *taskState) markClaimed(now time.Time) { st.timing.StartedAt = now }

// markSettled stamps the first terminal state; a later re-resolution
// (misuse) never moves it.
func (st *taskState) markSettled(now time.Time) {
	if st.timing.SettledAt.IsZero() {
		st.timing.SettledAt = now
	}
}

// noteEligibleLocked records that cand's submitted work has every
// predecessor settled: it stamps EligibleAt and emits task.eligible (§38)
// once per Task, when eligibility is observed and before any wait for
// scheduler capacity, so DependencyWait and SchedulerWait stay distinct.
func (o *Output) noteEligibleLocked(cand *taskState) {
	if !cand.timing.EligibleAt.IsZero() {
		return
	}
	cand.timing.EligibleAt = o.cfg.clock.Now()
	o.emitWireEventLocked(wire.EventTaskEligible, cand.id, nil)
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
