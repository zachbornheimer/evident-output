package engine

import (
	"context"
	"fmt"
	"io"
	"testing"
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

// TestSchedulingWorkIsLinear guards against a super-linear scheduler
// deterministically: the queue entries every scheduling pass examines must
// stay within a constant per submitted Task, for a Group and a Sequence,
// at 1k and 4k Tasks. (Wall-clock ratios are too noisy under a loaded
// `go test ./...`; BenchmarkScheduleGroup/Sequence report the timings.)
func TestSchedulingWorkIsLinear(t *testing.T) {
	for _, sequential := range []bool{false, true} {
		for _, n := range []int{1000, 4000} {
			if visits := scheduleVisits(t, n, sequential); visits > visitsPerTaskCeiling*n {
				t.Errorf("sequential=%v n=%d: scheduler examined %d queue entries (%.1f per Task), want <= %d per Task",
					sequential, n, visits, float64(visits)/float64(n), visitsPerTaskCeiling)
			}
		}
	}
}

// visitsPerTaskCeiling is the most queue entries the scheduler may examine
// per submitted Task. A linear scheduler stays at a small constant; the
// quadratic one this replaced examined O(n) per Task.
const visitsPerTaskCeiling = 8

// scheduleVisits runs scheduleContainer's workload and returns how many
// queue entries the scheduler examined.
func scheduleVisits(t *testing.T, n int, sequential bool) int {
	t.Helper()
	out := Init(Config{Isolated: true, StateDir: t.TempDir(), Stdout: io.Discard, Stderr: io.Discard})
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
		t.Fatalf("Wait: %v", err)
	}
	out.mu.Lock()
	defer out.mu.Unlock()
	return out.schedQueue.visits
}
