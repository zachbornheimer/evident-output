package evo_test

import (
	"errors"
	"io"
	"testing"

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
