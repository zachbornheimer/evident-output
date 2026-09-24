package engine

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestPlainWaiterNeverRaisesTheCeiling: a caller outside any callback
// holds no slot, so the work it runs while waiting must take one. With
// MaxConcurrency 1 and A holding the only slot, a plain Wait on X (After A)
// must not run the queued B beside A.
func TestPlainWaiterNeverRaisesTheCeiling(t *testing.T) {
	out := quietOutput(Config{MaxConcurrency: 1})
	t.Cleanup(func() { _ = out.Close() })

	var running, peak atomic.Int32
	enter := func() {
		n := running.Add(1)
		for p := peak.Load(); n > p && !peak.CompareAndSwap(p, n); p = peak.Load() {
		}
	}
	aStarted, bStarted := make(chan struct{}), make(chan struct{})

	a := out.Task("a").Define(func(context.Context) error {
		enter()
		defer running.Add(-1)
		close(aStarted)
		select {
		case <-bStarted:
		case <-time.After(200 * time.Millisecond):
		}
		return nil
	})
	<-aStarted
	out.Task("b").Define(func(context.Context) error {
		enter()
		defer running.Add(-1)
		close(bStarted)
		return nil
	})
	x := out.Task("x").After(a).Define(func(context.Context) error { return nil })

	if err := x.Wait(); err != nil {
		t.Fatalf("x.Wait() = %v", err)
	}
	_ = out.Finish()
	if got := peak.Load(); got != 1 {
		t.Fatalf("peak running callbacks = %d at MaxConcurrency 1, want 1", got)
	}
}

// TestSpawnedWaiterRunsItsAwaitedTaskWithoutASlot: a callback that starts a
// goroutine which Waits, and then blocks on that goroutine (the errgroup
// shape), holds the only slot at MaxConcurrency 1. The goroutine has no
// callback frame, so it has no slot to lend, but it is blocked anyway: it
// must still run the one task it awaits, or the run never finishes.
func TestSpawnedWaiterRunsItsAwaitedTaskWithoutASlot(t *testing.T) {
	out := quietOutput(Config{MaxConcurrency: 1})
	// Close drains the run; on the bug it hangs, so it gets the same bound
	// as the Wait instead of hanging the suite.
	t.Cleanup(func() {
		closed := make(chan struct{})
		go func() { _ = out.Close(); close(closed) }()
		select {
		case <-closed:
		case <-time.After(5 * time.Second):
			t.Error("Close hung")
		}
	})

	done := make(chan error, 1)
	go func() {
		a := out.Task("a").Define(func(context.Context) error {
			errc := make(chan error, 1)
			go func() {
				errc <- out.Task("b").Define(func(context.Context) error { return nil }).Wait()
			}()
			return <-errc
		})
		done <- a.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("a.Wait() = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a.Wait() hung: the spawned waiter never ran the task it awaits")
	}
}
