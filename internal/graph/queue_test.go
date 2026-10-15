package graph

import (
	"math/bits"
	"testing"
)

// TestQueuePushOutOfOrderIsNotQuadratic: a mass wake pushes Tasks declared
// before others already queued, so each lands mid-queue. That must cost
// O(log n) per push, not a shift of every later entry under the lock.
func TestQueuePushOutOfOrderIsNotQuadratic(t *testing.T) {
	const n = 4096
	var q queue
	for decl := n; decl > 0; decl-- {
		q.push(&Task{Declaration: decl, sched: schedule{phase: PhaseQueued}})
	}
	if limit := 2 * n * bits.Len(n); q.tasks.moved > limit {
		t.Fatalf("%d out-of-order pushes moved %d entries, want at most %d", n, q.tasks.moved, limit)
	}
	for want := 1; want <= n; want++ {
		task := q.tasks.entries[0]
		if task.Declaration != want {
			t.Fatalf("head declaration = %d, want %d", task.Declaration, want)
		}
		q.dropHead()
	}
}
