package evo_test

import (
	"context"
	"io"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// The §39 lifecycle boundaries must attribute each span to its real cause.
// The AGENTS.md dialect predeclares Tasks and Defines them later, so the
// caller's own gap between declaration and Define is routine, and it is
// never time spent waiting on predecessors.

func newTimingOutput(t *testing.T) (*evo.Output, *testkit.Clock) {
	t.Helper()
	clock := testkit.NewClock()
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Stderr: io.Discard, Plain: true, Clock: clock, MaxConcurrency: 1})
	t.Cleanup(func() { _ = out.Close() })
	return out, clock
}

func TestTiming_PredeclaredTaskDefinedLaterHasNoDependencyWait(t *testing.T) {
	t.Parallel()
	out, clock := newTimingOutput(t)
	const callerGap = 5 * time.Second
	task := out.Task("check config")
	clock.Advance(callerGap)
	task.Define(func(context.Context) error { return nil })
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	timing := task.Snapshot().Timing
	if got := timing.DependencyWait(); got != 0 {
		t.Fatalf("DependencyWait = %v, want 0: the Task has no predecessors (timing %+v)", got, timing)
	}
	if got := timing.AwaitingDefinition(); got != callerGap {
		t.Fatalf("AwaitingDefinition = %v, want %v", got, callerGap)
	}
	if got := out.Conclusion().Metrics().DependencyWait; got != 0 {
		t.Fatalf("Metrics().DependencyWait = %v, want 0", got)
	}
}

// Doing before Define promotes the row to Running while its work still
// waits on a predecessor. Queued stays exactly the dependency wait plus
// the scheduler wait, and Total splits into awaiting definition, queued,
// and running with nothing lost or double-counted.
func TestTiming_DoingBeforeDefineKeepsQueuedTheSumOfItsWaits(t *testing.T) {
	t.Parallel()
	out, clock := newTimingOutput(t)
	release := make(chan struct{})
	pred := out.Task("fetch").Define(func(context.Context) error {
		<-release
		clock.Advance(2 * time.Second)
		return nil
	})
	task := out.Task("prune")
	task.Doing("reading refs")
	clock.Advance(time.Second)
	task.After(pred).Define(func(context.Context) error {
		clock.Advance(3 * time.Second)
		return nil
	})
	close(release)
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	timing := task.Snapshot().Timing
	want := map[string][2]time.Duration{
		"AwaitingDefinition": {timing.AwaitingDefinition(), time.Second},
		"DependencyWait":     {timing.DependencyWait(), 2 * time.Second},
		"SchedulerWait":      {timing.SchedulerWait(), 0},
		"Queued":             {timing.Queued(), timing.DependencyWait() + timing.SchedulerWait()},
		"Running":            {timing.Running(), 3 * time.Second},
		"Total":              {timing.Total(), timing.AwaitingDefinition() + timing.Queued() + timing.Running()},
	}
	for name, got := range want {
		if got[0] != got[1] {
			t.Errorf("%s = %v, want %v (timing %+v)", name, got[0], got[1], timing)
		}
	}
}

// A Task resolved on the caller's stack never submits work: it has no
// submission, eligibility, or scheduler boundary, so every wait is zero
// and Queued still equals their sum.
func TestTiming_TaskResolvedWithoutWorkHasNoQueuedTime(t *testing.T) {
	t.Parallel()
	out, clock := newTimingOutput(t)
	task := out.Task("probe")
	clock.Advance(time.Second)
	task.Fail("probe unavailable")
	_ = out.Finish()
	timing := task.Snapshot().Timing
	if timing.Queued() != 0 || timing.DependencyWait() != 0 || timing.SchedulerWait() != 0 {
		t.Fatalf("waits = (%v, %v, %v), want all zero (timing %+v)", timing.Queued(), timing.DependencyWait(), timing.SchedulerWait(), timing)
	}
	if timing.Total() != time.Second {
		t.Fatalf("Total = %v, want 1s", timing.Total())
	}
}
