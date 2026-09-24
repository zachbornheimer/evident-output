package engine

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"
)

// scheduleContainer declares n no-op Tasks under one Group or Sequence,
// Defines each, and waits for the container.
func scheduleContainer(tb testing.TB, n int, sequential bool) {
	tb.Helper()
	out := Init(Config{Isolated: true, StateDir: tb.TempDir(), Stdout: io.Discard, Stderr: io.Discard})
	defer func() { _ = out.Close() }()
	var task func(string) *TaskHandle
	var wait func() error
	if sequential {
		s := out.Sequence("steps")
		task, wait = s.Task, s.Wait
	} else {
		g := out.Group("items")
		task, wait = g.Task, g.Wait
	}
	for i := range n {
		task(fmt.Sprintf("item %d", i)).Define(func(context.Context) error { return nil })
	}
	if err := wait(); err != nil {
		tb.Fatalf("Wait: %v", err)
	}
}

func BenchmarkScheduleGroup(b *testing.B) {
	for _, n := range []int{1000, 4000, 16000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			for range b.N {
				scheduleContainer(b, n, false)
			}
		})
	}
}

func BenchmarkScheduleSequence(b *testing.B) {
	for _, n := range []int{1000, 4000, 16000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			for range b.N {
				scheduleContainer(b, n, true)
			}
		})
	}
}

// TestSchedulingScalesLinearly guards against a super-linear scheduler:
// four times the Tasks must cost well under sixteen times the time.
func TestSchedulingScalesLinearly(t *testing.T) {
	if testing.Short() {
		t.Skip("timing guard")
	}
	for _, sequential := range []bool{false, true} {
		small := timeSchedule(t, 1000, sequential)
		large := timeSchedule(t, 4000, sequential)
		ratio := float64(large) / float64(small)
		t.Logf("sequential=%v: 1000=%v 4000=%v x%.1f", sequential, small, large, ratio)
		if ratio > linearScaleCeiling {
			t.Errorf("sequential=%v: 4000 Tasks took %v, 1000 took %v (x%.1f, want <= x%.0f)", sequential, large, small, ratio, linearScaleCeiling)
		}
	}
}

// linearScaleCeiling is the largest 4x-input cost ratio accepted: 4 is
// linear, 16 is quadratic; the margin absorbs timer and GC noise.
const linearScaleCeiling = 8.0

func timeSchedule(t *testing.T, n int, sequential bool) time.Duration {
	t.Helper()
	best := time.Duration(1<<63 - 1)
	for range 3 {
		start := time.Now()
		scheduleContainer(t, n, sequential)
		best = min(best, time.Since(start))
	}
	return best
}
