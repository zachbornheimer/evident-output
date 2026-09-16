package engine

import (
	"context"
	"errors"
	"io"
	"runtime"
	"testing"
	"time"
)

// waitOutcomeTimeout bounds how long a test waits for TaskHandle.Wait to
// return before concluding the scheduler left it hanging — generous enough
// that a slow CI runner never produces a false red, but far below "the test
// suite hung" territory.
const waitOutcomeTimeout = 2 * time.Second

func newAfterFailTestOutput(t *testing.T, maxConcurrency int) *Output {
	t.Helper()
	out := Init(Config{
		Isolated:       true,
		Stdout:         io.Discard,
		Stderr:         io.Discard,
		MaxConcurrency: maxConcurrency,
	})
	t.Cleanup(func() { _ = out.Close() })
	return out
}

// waitForParkedWaiter busy-polls (no sleep) until a goroutine has registered
// itself in o.schedWaits — the same registration TaskHandle.Wait's
// waitSubmitted performs just before it parks — so a test can prove a
// waiter is already asleep before triggering the event under test, instead
// of guessing at timing.
func waitForParkedWaiter(t *testing.T, o *Output, want int) {
	t.Helper()
	deadline := time.Now().Add(waitOutcomeTimeout)
	for time.Now().Before(deadline) {
		o.mu.Lock()
		n := len(o.schedWaits)
		o.mu.Unlock()
		if n >= want {
			return
		}
		runtime.Gosched()
	}
	t.Fatalf("timed out waiting for %d parked waiter(s)", want)
}

// TestAfterPredecessorFails_WhileDependentAlreadyParkedInWait_SettlesImmediately
// is the first red-first repro for the v0.5.2 hang (zq axis report): a task
// blocked on After(pred) parked in Wait must resolve NotStarted the instant
// pred fails — not only after pred's own callback goroutine happens to
// return. The predecessor's callback is held open past its own Fail() call
// (via holdReturn) specifically to prove the dependent settles synchronously
// under the same critical section that fails the predecessor, not via the
// scheduler's post-return kick()/cascade path — the old code only cascaded
// from kick(), so it left the dependent parked for as long as holdReturn
// stayed closed (in production: however long the predecessor's goroutine
// took to actually return from the callback call).
func TestAfterPredecessorFails_WhileDependentAlreadyParkedInWait_SettlesImmediately(t *testing.T) {
	for i := 0; i < 20; i++ {
		t.Run("", func(t *testing.T) {
			out := newAfterFailTestOutput(t, 4)

			release := make(chan struct{})
			holdReturn := make(chan struct{})

			pred := out.Task("classify")
			pred.Define(func(ctx context.Context) error {
				<-release
				pred.Fail("boom")
				<-holdReturn
				return nil
			})

			dependent := out.Task("worktree")
			dependent.After(pred).Define(func(ctx context.Context) error { return nil })

			waitOutcome := make(chan error, 1)
			go func() { waitOutcome <- dependent.Wait() }()
			waitForParkedWaiter(t, out, 1)

			close(release)

			select {
			case err := <-waitOutcome:
				if !errors.Is(err, ErrNotStarted) {
					t.Fatalf("Wait() = %v, want ErrNotStarted", err)
				}
			case <-time.After(waitOutcomeTimeout):
				t.Fatal("dependent.Wait() never returned after its predecessor failed, even though the predecessor's own callback goroutine was still executing (deadlock)")
			}

			close(holdReturn)
			if err := out.Finish(); err != nil {
				t.Fatalf("Finish: %v", err)
			}
		})
	}
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
	for i := 0; i < 20; i++ {
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
			pred.Fail("boom") // resolved before the dependent is ever submitted

			dependent := out.Task("worktree")
			dependent.After(pred).Define(func(ctx context.Context) error { return nil })

			waitOutcome := make(chan error, 1)
			go func() { waitOutcome <- dependent.Wait() }()

			select {
			case err := <-waitOutcome:
				if !errors.Is(err, ErrNotStarted) {
					t.Fatalf("Wait() = %v, want ErrNotStarted", err)
				}
			case <-time.After(waitOutcomeTimeout):
				t.Fatal("dependent.Wait() never returned even though its predecessor had already failed before submission (deadlock)")
			}

			close(busyRelease)
			if err := out.Finish(); err != nil {
				t.Fatalf("Finish: %v", err)
			}
		})
	}
}
