package graph

import (
	"strings"
	"sync"
	"testing"
)

// runClaimed is the marked function: it enters a hold first thing, as the
// code that grants a claim does.
func runClaimed(holds *Holds, work func()) {
	defer holds.Enter().Leave()
	work()
}

func TestHoldsCountsClaimsWhileTheyAreHeld(t *testing.T) {
	var holds Holds
	if holds.Any() {
		t.Fatal("Any() = true before any claim")
	}
	runClaimed(&holds, func() {
		if !holds.Any() {
			t.Error("Any() = false inside a claim")
		}
	})
	if holds.Any() {
		t.Error("Any() = true after the claim left")
	}
}

func TestHoldsBlockedReportsOnlyTheHoldingGoroutine(t *testing.T) {
	var holds Holds
	self := CurrentGoroutine()
	if holds.Blocked(self) || holds.Blocked(0) {
		t.Fatal("Blocked reported a goroutine before any claim")
	}
	runClaimed(&holds, func() {
		if !holds.Blocked(self) {
			t.Error("Blocked(self) = false inside a claim")
		}
		if holds.Blocked(0) {
			t.Error("Blocked(0) = true: the unknown goroutine never holds")
		}
		other := make(chan GoroutineID)
		go func() { other <- CurrentGoroutine() }()
		if holds.Blocked(<-other) {
			t.Error("Blocked(other goroutine) = true, want false")
		}
	})
	if holds.Blocked(self) {
		t.Error("Blocked(self) = true after the claim left")
	}
}

func TestHoldsNestedClaimsOnOneGoroutineLeaveOneAtATime(t *testing.T) {
	var holds Holds
	self := CurrentGoroutine()
	outer := holds.Enter()
	inner := holds.Enter()
	inner.Leave()
	if !holds.Blocked(self) || !holds.Any() {
		t.Error("the outer claim stopped counting when the inner one left")
	}
	outer.Leave()
	if holds.Blocked(self) || holds.Any() {
		t.Error("a claim still counts after every Leave")
	}
}

func TestHoldsFrameNamesTheFunctionThatEntered(t *testing.T) {
	var holds Holds
	if holds.Frame() != nil {
		t.Fatal("Frame() named a function before any claim")
	}
	runClaimed(&holds, func() {})
	frame := holds.Frame()
	if frame == nil || !strings.HasSuffix(*frame, ".runClaimed") {
		t.Errorf("Frame() = %v, want the function that called Enter (runClaimed)", frame)
	}
}

func TestHoldsStayConsistentUnderConcurrentClaims(t *testing.T) {
	var holds Holds
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			for range 100 {
				runClaimed(&holds, func() {})
			}
		})
	}
	wg.Wait()
	if holds.Any() || len(holds.byGoroutine) != 0 {
		t.Errorf("Any()=%t, tracked goroutines=%d after every claim left", holds.Any(), len(holds.byGoroutine))
	}
}

func TestWalkStackVisitsTheMarkedFrame(t *testing.T) {
	var marker FrameMarker
	marked := func() int {
		marker.Note()
		return countMarked(&marker)
	}
	if got := marked(); got != 1 {
		t.Errorf("marked frame seen %d times from inside it, want 1", got)
	}
}

// countMarked counts the marked frames above its own caller, as the engine's
// stack reads do: WalkStack leaves out the frame that called it.
func countMarked(marker *FrameMarker) int {
	count := 0
	WalkStack(func(function string) {
		if name := marker.Name(); name != nil && function == *name {
			count++
		}
	})
	return count
}
