package schedule

import "container/heap"

// Queue holds the submitted Tasks whose predecessors all succeeded and
// that nobody has started, earliest declaration first. A Task still
// waiting on a predecessor is parked off the queue, so the start pass
// never rescans work that cannot start. An entry goes stale when its Task
// starts or resolves some other way; Head drops stale entries as it
// passes them.
//
// It is a min-heap on declaration, not a sorted slice: a mass wake pushes
// Tasks declared before ones already queued, and a sorted insert shifted
// every later entry under the caller's lock, O(n·m) for n wakes past m
// queued Tasks.
type Queue[T Member] struct {
	tasks heap_[T]
	// visits counts the entries examined, so a test can prove scheduling
	// work stays linear without timing it.
	visits int
}

// Push queues x.
func (q *Queue[T]) Push(x T) {
	heap.Push(&q.tasks, x)
}

// Head returns the earliest entry live reports true for, dropping the
// stale entries before it. live runs under the caller's lock and must not
// lock.
func (q *Queue[T]) Head(live func(T) bool) (T, bool) {
	for q.tasks.Len() > 0 {
		q.visits++
		x := q.tasks.entries[0]
		if live(x) {
			return x, true
		}
		q.DropHead()
	}
	var zero T
	return zero, false
}

// DropHead removes the earliest entry.
func (q *Queue[T]) DropHead() {
	heap.Pop(&q.tasks)
}

// Visits is a test counter (was schedQueue.visits).
func (q *Queue[T]) Visits() int { return q.visits }

// Moved is a test counter (was declarationHeap.moved).
func (q *Queue[T]) Moved() int { return q.tasks.moved }

// heap_ is container/heap's view of the queue's entries.
type heap_[T Member] struct {
	entries []T
	// moved counts the swaps pushes and pops made, so a test can prove
	// either costs O(log n), not the length of the queue.
	moved int
}

func (h *heap_[T]) Len() int { return len(h.entries) }

func (h *heap_[T]) Less(i, j int) bool {
	return h.entries[i].Declaration() < h.entries[j].Declaration()
}

func (h *heap_[T]) Swap(i, j int) {
	h.entries[i], h.entries[j] = h.entries[j], h.entries[i]
	h.moved++
}

func (h *heap_[T]) Push(x any) { h.entries = append(h.entries, x.(T)) }

func (h *heap_[T]) Pop() any {
	last := len(h.entries) - 1
	x := h.entries[last]
	var zero T
	h.entries[last] = zero
	h.entries = h.entries[:last]
	return x
}
