package engine

import (
	"math/rand/v2"
	"slices"
	"testing"
)

// TestPositionSetMatchesSortedSlice drives a positionSet and a plain
// membership map through the same random adds and removes, over spans that
// need one, two, and three tree levels, and compares the ascending members.
func TestPositionSetMatchesSortedSlice(t *testing.T) {
	for _, span := range []int{40, 5000, 300000} {
		rng := rand.New(rand.NewPCG(uint64(span), 7))
		var set positionSet
		want := map[int]bool{}
		for step := range 4000 {
			i := rng.IntN(span)
			if step%3 == 2 {
				set.remove(i)
				delete(want, i)
			} else {
				set.add(i)
				want[i] = true
			}
		}
		var members []int
		set.each(func(p int) { members = append(members, p) })
		var expect []int
		for p := range want {
			expect = append(expect, p)
		}
		slices.Sort(expect)
		if !slices.Equal(members, expect) {
			t.Fatalf("span %d: members differ: got %d want %d", span, len(members), len(expect))
		}
		if set.len() != len(expect) {
			t.Fatalf("span %d: len %d, want %d", span, set.len(), len(expect))
		}
	}
}

// TestPositionSetAscendingAddsTerminate guards the sequential fill a
// collection makes as it declares Tasks: the tree must stop at its root.
func TestPositionSetAscendingAddsTerminate(t *testing.T) {
	var set positionSet
	for i := range 16000 {
		set.add(i)
	}
	if got := set.appendFirst(nil, 3); !slices.Equal(got, []int{0, 1, 2}) {
		t.Fatalf("first three = %v", got)
	}
	if len(set.levels) > 4 {
		t.Fatalf("tree height %d for 16000 positions", len(set.levels))
	}
}
