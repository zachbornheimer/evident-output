package evo_test

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"

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
