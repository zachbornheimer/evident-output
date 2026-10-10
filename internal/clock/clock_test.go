package clock_test

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/clock"
)

// Compile-time proof of which capabilities each clock has.
var (
	_ clock.Clock     = clock.Wall{}
	_ clock.Clock     = clock.Fixed{}
	_ clock.Scheduler = clock.Wall{}
)

const (
	// shortWait is long enough to measure and short enough to keep the suite fast.
	shortWait = 10 * time.Millisecond
	// firingBudget bounds how long a test waits for a timer it expects to fire.
	firingBudget = 5 * time.Second
)

func TestFixedAlwaysReturnsItsInstant(t *testing.T) {
	instant := time.Date(2026, time.October, 9, 12, 0, 0, 0, time.UTC)
	fixed := clock.Fixed{T: instant}
	for range 3 {
		if got := fixed.Now(); !got.Equal(instant) {
			t.Fatalf("Fixed.Now() = %v, want %v", got, instant)
		}
	}
}

func TestFixedIsNotAScheduler(t *testing.T) {
	if _, schedules := any(clock.Fixed{}).(clock.Scheduler); schedules {
		t.Fatal("Fixed implements Scheduler; scheduled work would arm against a clock that never advances")
	}
}

func TestFixedDiffersByInstant(t *testing.T) {
	first := time.Date(2026, time.October, 9, 12, 0, 0, 0, time.UTC)
	second := first.Add(time.Hour)
	if got := (clock.Fixed{T: second}).Now().Sub((clock.Fixed{T: first}).Now()); got != time.Hour {
		t.Fatalf("difference between two Fixed clocks = %v, want 1h", got)
	}
}

func TestWallNowNeverGoesBackwards(t *testing.T) {
	wall := clock.System()
	previous := wall.Now()
	for range 1000 {
		current := wall.Now()
		if current.Before(previous) {
			t.Fatalf("Now() went backwards: %v after %v", current, previous)
		}
		previous = current
	}
}

func TestWallSleepAndSinceMeasureAtLeastTheWait(t *testing.T) {
	wall := clock.System()
	start := wall.Now()
	wall.Sleep(shortWait)
	if elapsed := wall.Since(start); elapsed < shortWait {
		t.Fatalf("Since(start) after Sleep(%v) = %v", shortWait, elapsed)
	}
}

func TestWallAfterTimerAndTickerFire(t *testing.T) {
	wall := clock.System()
	select {
	case <-wall.After(shortWait):
	case <-time.After(firingBudget):
		t.Fatal("After never fired")
	}
	timer := wall.NewTimer(shortWait)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-time.After(firingBudget):
		t.Fatal("NewTimer never fired")
	}
	ticker := wall.NewTicker(shortWait)
	defer ticker.Stop()
	for range 2 {
		select {
		case <-ticker.C:
		case <-time.After(firingBudget):
			t.Fatal("NewTicker stopped ticking")
		}
	}
}

func TestWallAfterFuncRunsOnceUnlessCancelled(t *testing.T) {
	wall := clock.System()
	fired := make(chan struct{}, 1)
	wall.AfterFunc(shortWait, func() { fired <- struct{}{} })
	select {
	case <-fired:
	case <-time.After(firingBudget):
		t.Fatal("AfterFunc never ran")
	}

	var cancelledRan atomic.Bool
	cancel := wall.AfterFunc(time.Hour, func() { cancelledRan.Store(true) })
	cancel()
	wall.Sleep(shortWait)
	if cancelledRan.Load() {
		t.Fatal("a cancelled AfterFunc ran")
	}
}
