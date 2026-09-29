package engine

import "container/heap"

// schedQueue holds the submitted Tasks whose predecessors all succeeded
// and that nobody has started, earliest declaration first. A Task still
// waiting on a predecessor is parked off the queue (see placeLocked), so
// the start pass never rescans work that cannot start. An entry goes stale
// when its Task starts or resolves some other way; head drops stale
// entries as it passes them.
//
// It is a min-heap on declaration, not a sorted slice: a mass wake pushes
// Tasks declared before ones already queued, and a sorted insert shifted
// every later entry under o.mu, O(n·m) for n wakes past m queued Tasks.
type schedQueue struct {
	tasks declarationHeap
	// visits counts the entries examined, so a test can prove scheduling
	// work stays linear without timing it.
	visits int
}

// push queues st.
func (q *schedQueue) push(st *taskState) {
	heap.Push(&q.tasks, st)
}

// head returns the earliest-declared live entry, or nil, dropping the
// stale entries before it.
func (q *schedQueue) head() *taskState {
	for q.tasks.Len() > 0 {
		q.visits++
		st := q.tasks.entries[0]
		if st.sched.phase == phaseQueued && st.awaitingStart() {
			return st
		}
		q.dropHead()
	}
	return nil
}

// dropHead removes the earliest entry.
func (q *schedQueue) dropHead() {
	heap.Pop(&q.tasks)
}

// declarationHeap is container/heap's view of the queue's entries.
type declarationHeap struct {
	entries []*taskState
	// moved counts the swaps pushes and pops made, so a test can prove
	// either costs O(log n), not the length of the queue.
	moved int
}

func (h *declarationHeap) Len() int { return len(h.entries) }

func (h *declarationHeap) Less(i, j int) bool {
	return h.entries[i].declaration < h.entries[j].declaration
}

func (h *declarationHeap) Swap(i, j int) {
	h.entries[i], h.entries[j] = h.entries[j], h.entries[i]
	h.moved++
}

func (h *declarationHeap) Push(x any) { h.entries = append(h.entries, x.(*taskState)) }

func (h *declarationHeap) Pop() any {
	last := len(h.entries) - 1
	st := h.entries[last]
	h.entries[last] = nil
	h.entries = h.entries[:last]
	return st
}
