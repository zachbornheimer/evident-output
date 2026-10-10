package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/graph"
)

// settledWaitHangLimit is how long a test waits for a Wait that must answer
// before it calls the Wait hung.
const settledWaitHangLimit = 3 * time.Second

// answersWithinLimit runs wait and reports its answer, or fails the test when
// it does not answer within settledWaitHangLimit.
func answersWithinLimit(t *testing.T, what string, wait func() error) (answer error, answered bool) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- wait() }()
	select {
	case err := <-done:
		return err, true
	case <-time.After(settledWaitHangLimit):
		t.Errorf("%s: Wait hung for more than %v", what, settledWaitHangLimit)
		return nil, false
	}
}

// A callback that fails its own row and then waits on that row waits on
// itself: the callback cannot return until Wait does. Wait answers from the
// row, as it did before it learned to wait for a returning callback.
func TestWait_OnTheRowItsOwnCallbackFailedAnswersFromTheRow(t *testing.T) {
	out := newOutput("job", to(io.Discard))
	var task *TaskHandle
	got := make(chan error, 1)
	task = out.Task("self")
	task.Define(func(context.Context) error { task.Fail("x"); got <- task.Wait(); return nil })

	select {
	case err := <-got:
		if !errors.Is(err, graph.ErrWaitFailed) {
			t.Errorf("self Wait = %v, want ErrWaitFailed", err)
		}
	case <-time.After(settledWaitHangLimit):
		t.Error("a callback waiting on the row it failed itself hung")
	}
}

// A and B each wait on the other after A failed its own row. B's Wait on the
// failed A cannot hold for A's callback, which is blocked on B: it answers
// from the row, and both callbacks return.
func TestWait_TwoCallbacksWaitingOnEachOtherAfterOneFailedItselfBothReturn(t *testing.T) {
	for _, ceiling := range []int{1, 4} {
		t.Run(fmt.Sprintf("ceiling %d", ceiling), func(t *testing.T) {
			out := newOutput("job", to(io.Discard), maxConcurrency(ceiling))
			a, b := out.Task("a"), out.Task("b")
			a.Define(func(context.Context) error { a.Fail("x"); return b.Wait() })
			b.Define(func(context.Context) error { return a.Wait() })

			if _, ok := answersWithinLimit(t, "Wait on a", a.Wait); !ok {
				return
			}
		})
	}
}

func TestWait_AnObserverOfTwoCallbacksWaitingOnEachOtherAfterOneFailedItselfReturns(t *testing.T) {
	out := newOutput("job", to(io.Discard), maxConcurrency(4))
	a, b := out.Task("a"), out.Task("b")
	bStarted := make(chan struct{})
	a.Define(func(context.Context) error { <-bStarted; a.Fail("x"); return b.Wait() })
	b.Define(func(context.Context) error { close(bStarted); time.Sleep(10 * time.Millisecond); return a.Wait() })
	observer := out.Task("observer").Define(func(context.Context) error { return a.Wait() })

	_, _ = answersWithinLimit(t, "Wait on the observer", observer.Wait)
}

// An interrupt cancels the row at once, and a callback that ignores its
// context must not keep a waiter from learning that.
func TestWait_OnACancelledRowWhoseCallbackIgnoresItsContextAnswersCancelled(t *testing.T) {
	out := newOutput("job", to(io.Discard))
	never := make(chan struct{})
	defer close(never)
	stuck := out.Task("stuck").Define(func(context.Context) error { <-never; return nil })
	for out.graph.Executing() < 1 {
		time.Sleep(time.Millisecond)
	}
	out.interrupt("x")

	err, ok := answersWithinLimit(t, "Wait on the interrupted task", stuck.Wait)

	if ok && !errors.Is(err, graph.ErrWaitCancelled) {
		t.Errorf("Wait = %v, want ErrWaitCancelled", err)
	}
}

// A callback that honors the interrupt and returns ctx.Err() still leaves a
// Cancelled row, and Wait reports the cancellation, not the context error.
func TestWait_OnACancelledRowWhoseCallbackReturnedItsContextErrorAnswersCancelled(t *testing.T) {
	out := newOutput("job", to(io.Discard))
	task := out.Task("honors ctx").Define(func(ctx context.Context) error {
		<-ctx.Done()
		time.Sleep(5 * time.Millisecond)
		return ctx.Err()
	})
	for out.graph.Executing() < 1 {
		time.Sleep(time.Millisecond)
	}
	out.interrupt("x")

	err, ok := answersWithinLimit(t, "Wait on the interrupted task", task.Wait)

	if ok && !errors.Is(err, graph.ErrWaitCancelled) {
		t.Errorf("Wait = %v, want it to wrap ErrWaitCancelled", err)
	}
}
