package evo

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

// waitOutcomeTimeout bounds how long a test waits for Task.Wait to
// return before concluding the scheduler left it hanging — generous enough
// that a slow CI runner never produces a false red, but far below "the test
// suite hung" territory.
const waitOutcomeTimeout = 2 * time.Second

func newAfterFailTestOutput(t *testing.T, maxConcurrency int) *Run {
	t.Helper()
	out := initRun(Config{MaxConcurrency: maxConcurrency, Facades: Facades{Stdout: io.Discard, Stderr: io.Discard}})
	t.Cleanup(func() { _ = out.close() })
	return out
}

// TestAfterPredecessorFails_BeforeDependentIsSubmitted_SettlesImmediately is
// the second red-first repro: a predecessor that has already failed before
// its dependent is even Defined (submitted) must still resolve the
// dependent NotStarted immediately on submission — not only once the
// scheduler happens to notice via a waiter's registration. An unrelated
// concurrent task (busy) is kept running throughout so the old
// progressPossibleLocked heuristic ("something is executing, so waiting is
// not yet a deadlock") cannot mask the missing settle path — real repros
// hit this because unrelated work elsewhere in the run was still in flight.
func TestAfterPredecessorFails_BeforeDependentIsSubmitted_SettlesImmediately(t *testing.T) {
	for range 20 {
		t.Run("", func(t *testing.T) {
			out := newAfterFailTestOutput(t, 4)

			busyRelease := make(chan struct{})
			busyStarted := make(chan struct{})
			busy := out.Task("unrelated")
			busy.Define(func(ctx context.Context) error {
				close(busyStarted)
				<-busyRelease
				return nil
			})
			<-busyStarted // the unrelated task is genuinely executing before pred fails

			pred := out.Task("classify")
			pred.fail("boom") // resolved before the dependent is ever submitted

			dependent := out.Task("worktree")
			dependent.After(pred).Define(func(ctx context.Context) error { return nil })

			waitOutcome := make(chan error, 1)
			go func() { waitOutcome <- dependent.wait() }()

			select {
			case err := <-waitOutcome:
				if !errors.Is(err, errNotSucceeded) {
					t.Fatalf("Wait() = %v, want a not-succeeded settle", err)
				}
			case <-time.After(waitOutcomeTimeout):
				t.Fatal("dependent.Wait() never returned even though its predecessor had already failed before submission (deadlock)")
			}

			close(busyRelease)
			if err := out.finish(); err != nil {
				t.Fatalf("Finish: %v", err)
			}
		})
	}
}
