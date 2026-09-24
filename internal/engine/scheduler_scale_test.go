package engine

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"
)

// scheduleContainer declares n no-op Tasks under one Group or Sequence,
// Defines each, and waits for the container. It returns the queue entries
// scheduling examined.
func scheduleContainer(tb testing.TB, n int, sequential bool) int {
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
	out.mu.Lock()
	defer out.mu.Unlock()
	return out.schedQueue.visits
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

// TestSchedulingWorkIsLinear guards against a super-linear scheduler by
// counting the queue entries it examines, not by timing it: a timed 4x
// ratio flaked under a loaded `go test ./...`.
func TestSchedulingWorkIsLinear(t *testing.T) {
	const n = 4000
	for _, sequential := range []bool{false, true} {
		visits := scheduleContainer(t, n, sequential)
		t.Logf("sequential=%v: n=%d visits=%d", sequential, n, visits)
		if visits > scheduleVisitsPerTask*n {
			t.Errorf("sequential=%v: scheduling examined %d queue entries for %d Tasks (want <= %d)", sequential, visits, n, scheduleVisitsPerTask*n)
		}
	}
}

// scheduleVisitsPerTask bounds scheduling work per Task: a constant number
// of passes over each entry, never a rescan of the queue per start.
const scheduleVisitsPerTask = 4

// drainContainer is the canonical evo.Main shape: declare and Define n
// Tasks, return, and let Close's drain run them. The first Task holds the
// only slot until the drain has started, so every other Task is still
// queued when it does. It returns the queue entries the cascade examined.
func drainContainer(tb testing.TB, n int) int {
	tb.Helper()
	out := Init(Config{Isolated: true, StateDir: tb.TempDir(), Stdout: io.Discard, Stderr: io.Discard, MaxConcurrency: 1})
	release := make(chan struct{})
	g := out.Group("items")
	g.Task("gate").Define(func(context.Context) error {
		<-release
		return nil
	})
	for i := range n {
		g.Task(fmt.Sprintf("item %d", i)).Define(func(context.Context) error { return nil })
	}
	go func() {
		for {
			out.mu.Lock()
			draining := out.schedDraining
			out.mu.Unlock()
			if draining {
				close(release)
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()
	_ = out.Close()
	out.mu.Lock()
	defer out.mu.Unlock()
	return out.schedQueue.visits
}

// TestDrainWorkIsLinear guards the drain path TestSchedulingScalesLinearly
// cannot see: while draining, every Task completion used to rescan the
// whole live queue for unreachable work (n=16000: 127.9M visits, 3.57s).
// It counts the queue entries scheduling examined instead of timing it, so
// load cannot flake it.
func TestDrainWorkIsLinear(t *testing.T) {
	const n = 4000
	visits := drainContainer(t, n)
	t.Logf("drain: n=%d visits=%d", n, visits)
	if visits > drainVisitsPerTask*n {
		t.Errorf("drain examined %d queue entries for %d Tasks (want <= %d): the cascade rescans the queue per Task", visits, n, drainVisitsPerTask*n)
	}
}

// drainVisitsPerTask bounds scheduling work per Task: the drain's opening
// cascade visits each queued Task once, and starting it visits it again.
const drainVisitsPerTask = 4
