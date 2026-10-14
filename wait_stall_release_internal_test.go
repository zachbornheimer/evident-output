package evo

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/graph"
	"github.com/zachbornheimer/evident-output/internal/misuse"
)

// stallCeilings are the concurrency ceilings a stall release must hold at:
// one slot, a pool narrower than the parked waiters, and a wide pool.
var stallCeilings = []int{1, 2, 4}

// A parked wait on a Task that already settled owes no error to the run:
// the row answers it. Releasing it must not take the unrelated waits with
// it. Here b waits on a, whose row is Blocked while its callback waits on b;
// b gets Blocked's row answer, and only the wait on b is the cycle.
func TestStallReleaseAnswersWaitOnSettledTaskFromItsRow(t *testing.T) {
	for _, ceiling := range stallCeilings {
		t.Run(fmt.Sprintf("ceiling=%d", ceiling), func(t *testing.T) {
			out := quietOutput(Config{MaxConcurrency: ceiling})
			t.Cleanup(func() { _ = out.close() })
			a, b := out.Task("a"), out.Task("b")
			bSawA := make(chan error, 1)
			a.Define(func(context.Context) error {
				a.block("blocked")
				time.Sleep(20 * time.Millisecond)
				return b.wait()
			})
			b.Define(func(context.Context) error {
				err := a.wait()
				bSawA <- err
				return err
			})
			_ = waitWithin(t, "Finish", out.finish)
			err := <-bSawA
			if errors.Is(err, misuse.ErrWaitDeadlock) {
				t.Fatalf("b.Wait(a) = %v, want the Blocked row's answer, not a deadlock", err)
			}
			if !errors.Is(err, graph.ErrWaitFailed) {
				t.Fatalf("b.Wait(a) = %v, want ErrWaitFailed from the Blocked row", err)
			}
		})
	}
}

// A plain outer Wait on main, where main waits on a whose callback waits
// back on b, must return an answer at every ceiling instead of hanging.
func TestStallReleaseOuterWaitReturnsWhenCallbacksWaitOnSettledTask(t *testing.T) {
	for _, ceiling := range stallCeilings {
		t.Run(fmt.Sprintf("ceiling=%d", ceiling), func(t *testing.T) {
			out := quietOutput(Config{MaxConcurrency: ceiling})
			t.Cleanup(func() { _ = out.close() })
			a, b := out.Task("a"), out.Task("b")
			a.Define(func(context.Context) error { a.fail("x"); return b.wait() })
			b.Define(func(context.Context) error {
				time.Sleep(10 * time.Millisecond)
				return a.wait()
			})
			main := out.Task("main").Define(func(context.Context) error {
				time.Sleep(5 * time.Millisecond)
				return a.wait()
			})
			err := waitWithin(t, "main.Wait", main.wait)
			if err == nil {
				t.Fatalf("main.Wait() = nil, want the failure of a")
			}
			_ = waitWithin(t, "Finish", out.finish)
		})
	}
}
