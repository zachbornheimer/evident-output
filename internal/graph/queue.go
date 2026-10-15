package graph

import "container/heap"

// queue holds the submitted Tasks whose predecessors all succeeded and that
// nobody has started, earliest declaration first. A Task still waiting on a
// predecessor is parked off the queue (see place), so the start pass never
// rescans work that cannot start. An entry goes stale when its Task starts
// or resolves some other way; head drops stale entries as it passes them.
//
// It is a min-heap on declaration, not a sorted slice: a mass wake pushes
// Tasks declared before ones already queued, and a sorted insert shifted
// every later entry under the lock, O(n·m) for n wakes past m queued Tasks.
type queue struct {
	tasks declarationHeap
	// visits counts the entries examined, so a test can prove scheduling
	// work stays linear without timing it.
	visits int
}

// push queues t.
func (q *queue) push(t *Task) {
	heap.Push(&q.tasks, t)
}

// head returns the earliest-declared live entry, or nil, dropping the
// stale entries before it.
func (q *queue) head() *Task {
	for q.tasks.Len() > 0 {
		q.visits++
		t := q.tasks.entries[0]
		if t.sched.phase == PhaseQueued && t.awaitingStartLocked() {
			return t
		}
		q.dropHead()
	}
	return nil
}

// dropHead removes the earliest entry.
func (q *queue) dropHead() {
	heap.Pop(&q.tasks)
}

// declarationHeap is container/heap's view of the queue's entries.
type declarationHeap struct {
	entries []*Task
	// moved counts the swaps pushes and pops made, so a test can prove
	// either costs O(log n), not the length of the queue.
	moved int
}

func (h *declarationHeap) Len() int { return len(h.entries) }

func (h *declarationHeap) Less(i, j int) bool {
	return h.entries[i].Declaration < h.entries[j].Declaration
}

func (h *declarationHeap) Swap(i, j int) {
	h.entries[i], h.entries[j] = h.entries[j], h.entries[i]
	h.moved++
}

func (h *declarationHeap) Push(x any) { h.entries = append(h.entries, x.(*Task)) }

func (h *declarationHeap) Pop() any {
	last := len(h.entries) - 1
	t := h.entries[last]
	h.entries[last] = nil
	h.entries = h.entries[:last]
	return t
}
