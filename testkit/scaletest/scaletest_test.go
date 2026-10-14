package scaletest

import (
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/v2/internal/process"
)

func TestFastestReturnsSmallestOfSamples(t *testing.T) {
	costs := []time.Duration{5, 2, 9, 3}
	i := 0
	got := Fastest(len(costs), func() time.Duration { i++; return costs[i-1] })
	if got != 2 || i != len(costs) {
		t.Fatalf("Fastest = %v after %d runs, want 2 after %d", got, i, len(costs))
	}
}

func TestFastestUnderStopsAtTheFirstSampleWithinTheBound(t *testing.T) {
	costs := []time.Duration{9, 4, 2, 1}
	i := 0
	got := FastestUnder(len(costs), 5, func() time.Duration { i++; return costs[i-1] })
	if got != 4 || i != 2 {
		t.Fatalf("FastestUnder = %v after %d runs, want 4 after 2", got, i)
	}
}

func TestFastestUnderRunsEverySampleWhenNoneFits(t *testing.T) {
	costs := []time.Duration{9, 7, 8}
	i := 0
	got := FastestUnder(len(costs), 5, func() time.Duration { i++; return costs[i-1] })
	if got != 7 || i != len(costs) {
		t.Fatalf("FastestUnder = %v after %d runs, want 7 after %d", got, i, len(costs))
	}
}

func TestHostLoadExceedsOnlyAboveItsLimit(t *testing.T) {
	if _, ok := process.LoadAverage(); !ok {
		t.Skip("this host does not report a load average")
	}
	if busy, _ := HostLoadExceeds(1e9); busy {
		t.Error("a limit no host reaches reported the host busy")
	}
	busy, reason := HostLoadExceeds(-1)
	if !busy || reason == "" {
		t.Errorf("a limit below any load = (%v, %q), want busy with a reason", busy, reason)
	}
}
