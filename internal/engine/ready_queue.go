package engine

import "container/heap"

// readyQueue holds submitted Tasks whose predecessors have all settled,
// ordered by declaration. The scheduler starts them in that order, the same
// order a scan of the whole run would pick, without rescanning every Task
// each time a slot frees.
type readyQueue struct{ items readyHeap }

// push queues st once; a Task already queued stays where it is.
func (q *readyQueue) push(st *taskState) {
	if st.readied {
		return
	}
	st.readied = true
	heap.Push(&q.items, st)
}

// pop removes and returns the earliest-declared queued Task, or nil.
func (q *readyQueue) pop() *taskState {
	if q.items.Len() == 0 {
		return nil
	}
	st := heap.Pop(&q.items).(*taskState)
	st.readied = false
	return st
}

// readyHeap is readyQueue's container/heap storage.
type readyHeap []*taskState

func (h readyHeap) Len() int           { return len(h) }
func (h readyHeap) Less(i, j int) bool { return h[i].declaration < h[j].declaration }
func (h readyHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *readyHeap) Push(x any)        { *h = append(*h, x.(*taskState)) }
func (h *readyHeap) Pop() any {
	old := *h
	last := old[len(old)-1]
	old[len(old)-1] = nil
	*h = old[:len(old)-1]
	return last
}
