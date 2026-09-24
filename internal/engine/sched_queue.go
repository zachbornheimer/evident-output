package engine

import (
	"cmp"
	"slices"
)

// schedQueue holds the submitted Tasks whose predecessors all succeeded
// and that nobody has started, in declaration order. A Task still waiting
// on a predecessor is parked off the queue (see placeLocked), so the start
// pass never rescans work that cannot start. An entry goes stale when its
// Task starts or resolves some other way; head drops stale entries as it
// passes them.
type schedQueue struct {
	tasks []*taskState
	// visits counts the entries examined, so a test can prove scheduling
	// work stays linear without timing it.
	visits int
}

// push queues st in declaration order (almost always at the end: Define
// usually follows declaration order).
func (q *schedQueue) push(st *taskState) {
	i, _ := slices.BinarySearchFunc(q.tasks, st.declaration, func(e *taskState, decl int) int {
		return cmp.Compare(e.declaration, decl)
	})
	q.tasks = slices.Insert(q.tasks, i, st)
}

// head returns the earliest-declared live entry, or nil, dropping the
// stale entries before it.
func (q *schedQueue) head() *taskState {
	for len(q.tasks) > 0 {
		q.visits++
		st := q.tasks[0]
		if st.sched.phase == phaseQueued && st.awaitingStart() {
			return st
		}
		q.dropHead()
	}
	return nil
}

// dropHead removes the earliest entry.
func (q *schedQueue) dropHead() {
	q.tasks[0] = nil
	q.tasks = q.tasks[1:]
}
