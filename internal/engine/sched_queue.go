package engine

import (
	"cmp"
	"slices"

	"github.com/zachbornheimer/evident-output/internal/core"
)

// schedQueue holds the submitted Tasks nobody has started, in declaration
// order: the only Tasks the scheduler's start and NotStarted-cascade
// passes ever need to look at, so their cost follows the queue, not every
// Task the run ever declared. An entry goes stale when its Task starts or
// resolves some other way; readers drop stale entries as they pass them.
//
// A Sequence step whose only precondition is its previous step waits off
// the queue, parked on that step (see enqueueLocked): otherwise every
// later step of a long Sequence would sit in the queue ineligible, and
// each scheduling pass would rescan all of them.
type schedQueue struct {
	tasks []*taskState
	// visits counts the entries every pass has examined, so a test can
	// prove scheduling work stays linear without timing it.
	visits int
}

// awaitingStart reports whether st is submitted work nobody has started
// or resolved — a live queue entry.
func awaitingStart(st *taskState) bool {
	return st.submitted && !st.runningWork && !core.IsTerminalTask(st.state)
}

// push queues st in declaration order (almost always at the end: Define
// usually follows declaration order).
func (q *schedQueue) push(st *taskState) {
	i, _ := slices.BinarySearchFunc(q.tasks, st.declaration, func(e *taskState, decl int) int {
		return cmp.Compare(e.declaration, decl)
	})
	q.tasks = slices.Insert(q.tasks, i, st)
}

// first returns the earliest-declared live entry ok accepts, or nil. It
// drops the stale entries it passes, so repeated calls stay proportional
// to the live queue.
func (q *schedQueue) first(ok func(*taskState) bool) *taskState {
	for len(q.tasks) > 0 && !awaitingStart(q.tasks[0]) {
		q.visits++
		q.tasks[0] = nil
		q.tasks = q.tasks[1:]
	}
	for _, st := range q.tasks {
		q.visits++
		if awaitingStart(st) && ok(st) {
			return st
		}
	}
	return nil
}

// live compacts the queue and returns its live entries. The slice is only
// valid until the next push.
func (q *schedQueue) live() []*taskState {
	q.visits += len(q.tasks)
	q.tasks = slices.DeleteFunc(q.tasks, func(st *taskState) bool { return !awaitingStart(st) })
	return q.tasks
}

// enqueueLocked queues newly submitted st, or parks it on its previous
// Sequence step when that step is its only precondition and is still
// unresolved; releaseNextStepLocked queues it once that step resolves.
// Nothing parks once the run is draining, so the drain's cascade sees
// every submitted Task.
func (o *Output) enqueueLocked(st *taskState) {
	if prev := st.prevSibling; prev != nil && !o.schedDraining && len(st.preds) == 0 &&
		st.collection.sequential && !core.IsTerminalTask(prev.state) {
		st.parkedOnPrev = true
		return
	}
	o.schedQueue.push(st)
}

// releaseNextStepLocked queues the Sequence step parked on st, now that st
// has resolved.
func (o *Output) releaseNextStepLocked(st *taskState) {
	next := st.nextSibling
	if next == nil || !next.parkedOnPrev {
		return
	}
	next.parkedOnPrev = false
	if awaitingStart(next) {
		o.schedQueue.push(next)
	}
}

// unparkAllLocked queues every parked step. Draining calls it once so the
// cascade can settle a step whose previous step will now never run.
func (o *Output) unparkAllLocked() {
	for _, st := range o.tasks {
		if st.parkedOnPrev {
			st.parkedOnPrev = false
			if awaitingStart(st) {
				o.schedQueue.push(st)
			}
		}
	}
}
