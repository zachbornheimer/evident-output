package evo_test

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

// TestWait_NotStartedDependencyIsAnError pins the honesty of Wait's return
// value: a task whose callback never ran owes its waiter an error, not the
// zero value of "what the callback returned". Without it a waiter resolves
// Done — a green row directly above the "- not started" line that says the
// work it claims to have awaited never happened.
func TestWait_NotStartedDependencyIsAnError(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := isolatedScheduler(t, 1, &buf, false)

	gate := out.Task("gate")
	gate.Define(func() error { return errProbe })

	blocked := out.Task("blocked").After(gate)
	var blockedRan atomic.Bool
	blocked.Define(func() error {
		blockedRan.Store(true)
		return nil
	})

	var waitErr error
	waiter := out.Task("waiter")
	waiter.Define(func() error {
		waitErr = blocked.Wait()
		return waitErr
	})

	withinBudget(t, "Finish with a waiter on a task that never starts", func() {
		_ = out.Finish()
	})

	if blockedRan.Load() {
		t.Fatal("the blocked task ran even though its predecessor failed")
	}
	if got := blocked.Snapshot().State; got != evo.NotStarted {
		t.Fatalf("blocked state = %v, want NotStarted", got)
	}
	if !errors.Is(waitErr, evo.ErrNotStarted) {
		t.Fatalf("Wait = %v, want ErrNotStarted", waitErr)
	}
	if got := waiter.Snapshot().State; got != evo.Failed {
		t.Fatalf("waiter state = %v, want Failed: it awaited work that never ran", got)
	}
	if rendered := buf.String(); !strings.Contains(rendered, "✗ waiter") {
		t.Fatalf("the waiter's row must fail, got:\n%s", rendered)
	}
}

// TestWait_ReleasesSlotWhileBlocked pins that a blocked waiter never holds
// the concurrency ceiling hostage. At MaxConcurrency 1 the waiter's own slot
// was the only thing keeping the task that unblocks its dependency queued,
// so the wait ended only when the run drained and abandoned that dependency.
func TestWait_ReleasesSlotWhileBlocked(t *testing.T) {
	t.Parallel()
	out := isolatedScheduler(t, 1, io.Discard, false)

	// waiter is declared first so the scheduler's only slot goes to it, and
	// holds that slot until unlock and locked are both queued behind it.
	waiter := out.Task("waiter")
	unlock := out.Task("unlock")
	locked := out.Task("locked").After(unlock)

	waiterHoldsTheSlot := make(chan struct{})
	chainQueued := make(chan struct{})
	var waitErr error
	waiter.Define(func() error {
		close(waiterHoldsTheSlot)
		<-chainQueued
		waitErr = locked.Wait()
		return waitErr
	})

	<-waiterHoldsTheSlot
	unlock.Define(func() error { return nil })
	locked.Define(func() error { return nil })
	close(chainQueued)

	withinBudget(t, "Finish at MaxConcurrency 1 with a blocked waiter", func() {
		if err := out.Finish(); err != nil {
			t.Errorf("Finish: %v", err)
		}
	})

	if waitErr != nil {
		t.Fatalf("Wait = %v, want nil", waitErr)
	}
	for name, task := range map[string]*evo.TaskHandle{
		"unlock": unlock, "locked": locked, "waiter": waiter,
	} {
		if got := task.Snapshot().State; got != evo.Done {
			t.Fatalf("%s state = %v, want Done", name, got)
		}
	}
	if got := out.SchedulerMaxObserved(); got != 1 {
		t.Fatalf("scheduler max observed = %d, want 1: a waiter must not raise the ceiling", got)
	}
}

// TestWait_OnSelfIsMisuse pins the smallest unsatisfiable wait: a callback
// awaiting its own task is the only executor that could ever resolve it, so
// the wait can never be satisfied and must be answered the moment it is
// made — not by hanging Finish forever.
func TestWait_OnSelfIsMisuse(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := isolatedScheduler(t, 1, &buf, false)

	var waitErr error
	task := out.Task("mirror")
	withinBudget(t, "a callback waiting on its own task", func() {
		task.Define(func() error {
			waitErr = task.Wait()
			return waitErr
		})
		_ = out.Finish()
	})

	if !errors.Is(waitErr, evo.ErrWaitDeadlock) {
		t.Fatalf("Wait = %v, want ErrWaitDeadlock", waitErr)
	}
	if !strings.Contains(waitErr.Error(), "mirror") {
		t.Fatalf("Wait = %q, want it to name the awaited task", waitErr)
	}
	if got := task.Snapshot().State; got != evo.Failed {
		t.Fatalf("task state = %v, want Failed", got)
	}
	if err := out.Err(); !errors.Is(err, evo.ErrWaitDeadlock) {
		t.Fatalf("misuse = %v, want ErrWaitDeadlock recorded", err)
	}
	if rendered := buf.String(); !strings.Contains(rendered, "✗ mirror") {
		t.Fatalf("the self-waiting task's row must fail, got:\n%s", rendered)
	}
}

// TestWait_MutualCycleIsMisuse pins the same rule across two tasks: once
// every running callback is parked in Wait and nothing is left to start,
// no order of events can satisfy any of them, so each is released with the
// task it awaited named.
func TestWait_MutualCycleIsMisuse(t *testing.T) {
	t.Parallel()
	out := isolatedScheduler(t, 2, io.Discard, false)

	first := out.Task("first")
	second := out.Task("second")
	// Neither may wait before both are queued: waiting on a task that has
	// no callback yet returns at once, and would test nothing.
	bothQueued := make(chan struct{})
	var firstErr, secondErr error
	first.Define(func() error {
		<-bothQueued
		firstErr = second.Wait()
		return firstErr
	})
	second.Define(func() error {
		<-bothQueued
		secondErr = first.Wait()
		return secondErr
	})
	close(bothQueued)

	withinBudget(t, "two callbacks waiting on each other", func() {
		_ = out.Finish()
	})

	for name, err := range map[string]error{"first": firstErr, "second": secondErr} {
		if !errors.Is(err, evo.ErrWaitDeadlock) {
			t.Fatalf("%s Wait = %v, want ErrWaitDeadlock", name, err)
		}
	}
	for name, task := range map[string]*evo.TaskHandle{"first": first, "second": second} {
		if got := task.Snapshot().State; got != evo.Failed {
			t.Fatalf("%s state = %v, want Failed", name, got)
		}
	}
}

// TestWait_FromOutsideAnyCallbackStillBlocks pins the pattern the deadlock
// rule must never mistake for a cycle: a plain caller — no callback of its
// own to hold still — waiting on a task the scheduler is genuinely running.
// Nobody is stuck, so the wait must block until the work finishes.
func TestWait_FromOutsideAnyCallbackStillBlocks(t *testing.T) {
	t.Parallel()
	out := isolatedScheduler(t, 1, io.Discard, false)

	running := make(chan struct{})
	release := make(chan struct{})
	var ran atomic.Bool
	task := out.Task("slow")
	task.Define(func() error {
		close(running)
		<-release
		ran.Store(true)
		return nil
	})

	<-running
	waited := make(chan error, 1)
	go func() { waited <- task.Wait() }()

	select {
	case err := <-waited:
		t.Fatalf("Wait returned %v while the task was still running", err)
	case <-time.After(waitProbe):
	}

	close(release)
	var waitErr error
	withinBudget(t, "Wait on a running task from outside any callback", func() {
		waitErr = <-waited
		if err := out.Finish(); err != nil {
			t.Errorf("Finish: %v", err)
		}
	})

	if waitErr != nil {
		t.Fatalf("Wait = %v, want nil", waitErr)
	}
	if !ran.Load() {
		t.Fatal("the awaited callback never completed")
	}
	if got := task.Snapshot().State; got != evo.Done {
		t.Fatalf("task state = %v, want Done", got)
	}
}

// waitProbe is how long a wait that must still be blocking is observed
// before the test accepts that it is. Short: it costs every run this long,
// and a deadlock rule that fires at all fires immediately.
const waitProbe = 100 * time.Millisecond
