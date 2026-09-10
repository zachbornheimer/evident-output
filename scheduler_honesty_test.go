package evo_test

import (
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// errProbe is the failure a probe callback returns — one shared sentinel so
// an assertion can say "the callback's outcome survived", not "some error
// happened".
var errProbe = errors.New("probe failure")

// TestScheduler_P2_DoneAfterVerbKeepsCallbackOutcome pins the P2 probe: a
// caller that follows a mutation verb with its own Done() must not launder
// the callback's failure into a green row. Done asserts a success the
// scheduler has not observed, so it is recorded misuse and resolves nothing.
func TestScheduler_P2_DoneAfterVerbKeepsCallbackOutcome(t *testing.T) {
	t.Parallel()
	out := isolatedScheduler(t, 1, io.Discard, false)
	task := out.Task("cleanup")
	release := make(chan struct{})
	task.Delete("worktree", func() error {
		<-release
		return errProbe
	})
	task.Done()
	close(release)
	// Finish surfaces the recorded misuse; the row underneath it must still
	// carry the callback's outcome.
	if err := out.Finish(); !errors.Is(err, evo.ErrAlreadyResolved) {
		t.Fatalf("Finish = %v, want ErrAlreadyResolved", err)
	}

	if got := task.Snapshot().State; got != evo.Failed {
		t.Fatalf("task state = %v, want Failed (the callback's outcome)", got)
	}
	if err := out.Err(); !errors.Is(err, evo.ErrAlreadyResolved) {
		t.Fatalf("misuse = %v, want ErrAlreadyResolved recorded at the Done call", err)
	}
}

// TestScheduler_P13_FailfInsideDefineDoesNotDoubleResolve pins the P13
// probe: a callback that resolves itself and returns that error is one
// outcome, not two — the scheduler must not re-Fail it or record misuse.
func TestScheduler_P13_FailfInsideDefineDoesNotDoubleResolve(t *testing.T) {
	t.Parallel()
	out := isolatedScheduler(t, 1, io.Discard, false)
	task := out.Task("lint")
	task.Define(func() error {
		return task.Failf("lint failed: %w", errProbe)
	})
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	if got := task.Snapshot().State; got != evo.Failed {
		t.Fatalf("task state = %v, want Failed", got)
	}
	if err := out.Err(); err != nil {
		t.Fatalf("misuse = %v, want none: the callback resolved itself once", err)
	}
	testkit.RequireConclusion(t, out, evo.StateFailed)
}

// waitBudget bounds a test that would otherwise hang forever on the defect
// it pins. It is a deadlock detector, not a timing dependency: the green
// path never reaches it.
const waitBudget = 5 * time.Second

// withinBudget runs work and fails the test if it has not returned inside
// waitBudget — the shape P15 (hang) and P16 (deadlock) both need.
func withinBudget(t *testing.T, what string, work func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		work()
	}()
	select {
	case <-done:
	case <-time.After(waitBudget):
		t.Fatalf("%s did not return within %s", what, waitBudget)
	}
}

// TestScheduler_P15_WaitOnTerminalTaskReturnsInsteadOfHanging pins the P15
// probe: defining work on an already-resolved task never runs that work, so
// a caller waiting on it must be told immediately rather than blocked
// forever.
func TestScheduler_P15_WaitOnTerminalTaskReturnsInsteadOfHanging(t *testing.T) {
	t.Parallel()
	out := isolatedScheduler(t, 1, io.Discard, false)
	task := out.Task("venv")
	task.Skipped(evo.Reason("present"))

	var ran atomic.Bool
	var waitErr error
	withinBudget(t, "Define+Wait on a resolved task", func() {
		task.Define(func() error {
			ran.Store(true)
			return nil
		})
		waitErr = task.Wait()
	})

	if ran.Load() {
		t.Fatal("callback ran on an already-resolved task")
	}
	if waitErr != nil {
		t.Fatalf("Wait = %v, want nil", waitErr)
	}
	if err := out.Err(); !errors.Is(err, evo.ErrAlreadyResolved) {
		t.Fatalf("misuse = %v, want ErrAlreadyResolved", err)
	}
	if got := task.Snapshot().State; got != evo.Skipped {
		t.Fatalf("task state = %v, want Skipped", got)
	}
}

// TestScheduler_P16_NestedWaitAtCeilingCompletes pins the P16 probe: a
// goroutine blocked inside Wait is not doing work, so the concurrency
// ceiling must never be the reason the task it waits on cannot start.
func TestScheduler_P16_NestedWaitAtCeilingCompletes(t *testing.T) {
	t.Parallel()
	out := isolatedScheduler(t, 1, io.Discard, false)
	group := out.Group("setup")

	var innerRan atomic.Bool
	var innerErr error
	outer := group.Task("outer")
	outer.Define(func() error {
		inner := group.Task("inner")
		inner.Define(func() error {
			innerRan.Store(true)
			return nil
		})
		innerErr = inner.Wait()
		return innerErr
	})

	withinBudget(t, "nested Define+Wait at MaxConcurrency 1", func() {
		if err := out.Finish(); err != nil {
			t.Errorf("Finish: %v", err)
		}
	})

	if !innerRan.Load() {
		t.Fatal("nested task never ran")
	}
	if innerErr != nil {
		t.Fatalf("inner Wait = %v, want nil", innerErr)
	}
	if got := outer.Snapshot().State; got != evo.Done {
		t.Fatalf("outer state = %v, want Done", got)
	}
}
