package evo

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
	out := initRun(Config{State: State{Dir: tb.TempDir()}, Facades: Facades{Stdout: io.Discard, Stderr: io.Discard}})
	defer func() { _ = out.close() }()
	var task func(string) *Task
	var wait func() error
	if sequential {
		s := out.Sequence("steps")
		task, wait = s.Task, s.wait
	} else {
		g := out.Group("items")
		task, wait = g.Task, g.wait
	}
	for i := range n {
		task(fmt.Sprintf("item %d", i)).Define(func(context.Context) error { return nil })
	}
	if err := wait(); err != nil {
		tb.Fatalf("Wait: %v", err)
	}
	return out.graph.QueueVisits()
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
	out := initRun(Config{MaxConcurrency: 1, State: State{Dir: tb.TempDir()}, Facades: Facades{Stdout: io.Discard, Stderr: io.Discard}})
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
			draining := out.graph.Draining()
			if draining {
				close(release)
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()
	_ = out.close()
	return out.graph.QueueVisits()
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

// fanInTasks declares n Tasks and one more After every one of them — the
// After(t1…tn) shape. It returns the predecessor outcomes scheduling read.
func fanInTasks(tb testing.TB, n int) int {
	tb.Helper()
	out := initRun(Config{MaxConcurrency: 1, State: State{Dir: tb.TempDir()}, Facades: Facades{Stdout: io.Discard, Stderr: io.Discard}})
	defer func() { _ = out.close() }()
	preds := make([]Predecessor, n)
	for i := range preds {
		preds[i] = out.Task(fmt.Sprintf("item %d", i)).Define(func(context.Context) error { return nil })
	}
	if err := out.Task("fan-in").After(preds...).Define(func(context.Context) error { return nil }).wait(); err != nil {
		tb.Fatalf("Wait: %v", err)
	}
	return out.graph.PredecessorChecks()
}

// TestFanInOverTasksIsLinear guards After(t1…tn): each time one
// predecessor settled, the waiting Task used to reread every predecessor
// from the first, Done ones included (n=32000: 14.7s).
func TestFanInOverTasksIsLinear(t *testing.T) {
	const n = 2000
	checks := fanInTasks(t, n)
	t.Logf("fan-in over Tasks: n=%d predecessor checks=%d", n, checks)
	if checks > predChecksPerPredecessor*n {
		t.Errorf("fan-in read %d predecessor outcomes for %d predecessors (want <= %d)", checks, n, predChecksPerPredecessor*n)
	}
}

// predChecksPerPredecessor bounds how often scheduling reads one
// predecessor's outcome: a constant, never once per sibling that settles.
const predChecksPerPredecessor = 4
