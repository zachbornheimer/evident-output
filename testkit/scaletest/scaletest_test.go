package scaletest

import (
	"testing"
	"time"
)

func TestFastestReturnsSmallestOfSamples(t *testing.T) {
	costs := []time.Duration{5, 2, 9, 3}
	i := 0
	got := Fastest(len(costs), func() time.Duration { i++; return costs[i-1] })
	if got != 2 || i != len(costs) {
		t.Fatalf("Fastest = %v after %d runs, want 2 after %d", got, i, len(costs))
	}
}
