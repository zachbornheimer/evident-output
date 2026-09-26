package ordered

import (
	"math/bits"
	"slices"
	"testing"
)

// TestInsertScalesWithItems guards the one-Task-per-item shape: items
// arrive roughly in key order, so each insert used to scan the whole list
// before appending, O(N^2) key reads (40k Effect Tasks: 6.4s of CPU).
// Inserting N items must read at most O(N log N) keys.
func TestInsertScalesWithItems(t *testing.T) {
	const n = 20000
	reads := 0
	key := func(v int) int { reads++; return v }
	var items []int
	for i := range n {
		items = Insert(items, i, key)
	}
	budget := n * (bits.Len(n) + 2)
	if reads > budget {
		t.Fatalf("inserting %d in-order items read key %d times, want at most %d", n, reads, budget)
	}
}

// TestInsertKeepsTiesInArrivalOrder pins the placement rule.
func TestInsertKeepsTiesInArrivalOrder(t *testing.T) {
	type sec struct{ order, arrival int }
	byOrder := func(s sec) int { return s.order }
	var got []sec
	for i, o := range []int{5, 1, 5, 3, 1, 9, 0} {
		got = Insert(got, sec{o, i}, byOrder)
	}
	want := []sec{{0, 6}, {1, 1}, {1, 4}, {3, 3}, {5, 0}, {5, 2}, {9, 5}}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
