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
