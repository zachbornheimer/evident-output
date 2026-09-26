package schedule

import (
	"math/bits"
	"testing"
)

// TestQueuePushOutOfOrderIsNotQuadratic: a mass wake pushes Tasks declared
// before others already queued, so each lands mid-queue. That must cost
// O(log n) per push, not a shift of every later entry under the caller's
// lock.
func TestQueuePushOutOfOrderIsNotQuadratic(t *testing.T) {
	const n = 4096
	var q Queue[fakeMember]
	for decl := n; decl > 0; decl-- {
		q.Push(fakeMember{decl: decl})
	}
	if limit := 2 * n * bits.Len(n); q.Moved() > limit {
		t.Fatalf("%d out-of-order pushes moved %d entries, want at most %d", n, q.Moved(), limit)
	}
	for want := 1; want <= n; want++ {
		x, ok := q.Head(func(fakeMember) bool { return true })
		if !ok {
			t.Fatalf("Head() ok = false, want true")
		}
		if x.decl != want {
			t.Fatalf("head declaration = %d, want %d", x.decl, want)
		}
		q.DropHead()
	}
}
