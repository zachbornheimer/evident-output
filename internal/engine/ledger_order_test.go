package engine

import (
	"math/bits"
	"slices"
	"testing"
)

// TestInsertAfterOrderScalesWithSections guards the one-Task-per-item
// shape: sections open roughly in declaration order, so each insert used
// to scan the whole list before appending, O(N^2) order reads under o.mu
// (40k Effect Tasks: 6.4s of CPU). Inserting N sections must read at
// most O(N log N) orders.
func TestInsertAfterOrderScalesWithSections(t *testing.T) {
	const n = 20000
	reads := 0
	order := func(v int) int { reads++; return v }
	var items []int
	for i := range n {
		items = insertAfterOrder(items, i, order)
	}
	budget := n * (bits.Len(n) + 2)
	if reads > budget {
		t.Fatalf("inserting %d in-order sections read order %d times, want at most %d", n, reads, budget)
	}
}

// TestInsertAfterOrderKeepsTiesInArrivalOrder pins the placement rule.
func TestInsertAfterOrderKeepsTiesInArrivalOrder(t *testing.T) {
	type sec struct{ order, arrival int }
	byOrder := func(s sec) int { return s.order }
	var got []sec
	for i, o := range []int{5, 1, 5, 3, 1, 9, 0} {
		got = insertAfterOrder(got, sec{o, i}, byOrder)
	}
	want := []sec{{0, 6}, {1, 1}, {1, 4}, {3, 3}, {5, 0}, {5, 2}, {9, 5}}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
