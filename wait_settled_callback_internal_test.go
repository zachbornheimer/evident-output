package evo

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"
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

// A and B each wait on the other after A failed its own row. B's Wait on the
// failed A cannot hold for A's callback, which is blocked on B: it answers
// from the row, and both callbacks return.
func TestWait_TwoCallbacksWaitingOnEachOtherAfterOneFailedItselfBothReturn(t *testing.T) {
	for _, ceiling := range []int{1, 4} {
		t.Run(fmt.Sprintf("ceiling %d", ceiling), func(t *testing.T) {
			out := newOutput("job", to(io.Discard), maxConcurrency(ceiling))
			a, b := out.Task("a"), out.Task("b")
			a.Define(func(context.Context) error { a.fail("x"); return b.wait() })
			b.Define(func(context.Context) error { return a.wait() })

			if _, ok := answersWithinLimit(t, "Wait on a", a.wait); !ok {
				return
			}
		})
	}
}

func TestWait_AnObserverOfTwoCallbacksWaitingOnEachOtherAfterOneFailedItselfReturns(t *testing.T) {
	out := newOutput("job", to(io.Discard), maxConcurrency(4))
	a, b := out.Task("a"), out.Task("b")
	bStarted := make(chan struct{})
	a.Define(func(context.Context) error { <-bStarted; a.fail("x"); return b.wait() })
	b.Define(func(context.Context) error { close(bStarted); time.Sleep(10 * time.Millisecond); return a.wait() })
	observer := out.Task("observer").Define(func(context.Context) error { return a.wait() })

	_, _ = answersWithinLimit(t, "Wait on the observer", observer.wait)
}
