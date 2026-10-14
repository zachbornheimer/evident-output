package evo

import (
	"context"
	"fmt"
	"sync"
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
	t.Cleanup(func() { _ = out.close() })

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

	if err := x.wait(); err != nil {
		t.Fatalf("x.Wait() = %v", err)
	}
	_ = out.finish()
	if got := peak.Load(); got != 1 {
		t.Fatalf("peak running callbacks = %d at MaxConcurrency 1, want 1", got)
	}
}

// TestSpawnedWaiterRunsOnceItsCallbackFreesTheSlot: a goroutine a callback
// started may Wait while that callback keeps working; the wait completes
// once the callback returns and frees the slot.
func TestSpawnedWaiterRunsOnceItsCallbackFreesTheSlot(t *testing.T) {
	out := quietOutput(Config{MaxConcurrency: 2})
	t.Cleanup(func() { _ = out.close() })

	waited := make(chan error, 1)
	a := out.Task("a").Define(func(context.Context) error {
		go func() {
			waited <- out.Task("b").Define(func(context.Context) error { return nil }).wait()
		}()
		return nil
	})
	if err := a.wait(); err != nil {
		t.Fatalf("a.Wait() = %v", err)
	}
	select {
	case err := <-waited:
		if err != nil {
			t.Fatalf("b.Wait() = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("b.Wait() hung")
	}
}

// peakCounter tracks how many callbacks run at once.
type peakCounter struct{ running, peak atomic.Int32 }

func (p *peakCounter) enter() {
	n := p.running.Add(1)
	for q := p.peak.Load(); n > q && !p.peak.CompareAndSwap(q, n); q = p.peak.Load() {
	}
}

func (p *peakCounter) leave() { p.running.Add(-1) }

// TestPlainWaitOnItsOwnTaskKeepsTheCeiling: at MaxConcurrency 1, with A
// holding the only slot, a plain caller that Defines B and Waits on it must
// wait for the slot, not run B beside A.
func TestPlainWaitOnItsOwnTaskKeepsTheCeiling(t *testing.T) {
	out := quietOutput(Config{MaxConcurrency: 1})
	t.Cleanup(func() { _ = out.close() })

	var p peakCounter
	aStarted := make(chan struct{})
	out.Task("a").Define(func(context.Context) error {
		p.enter()
		defer p.leave()
		close(aStarted)
		time.Sleep(100 * time.Millisecond)
		return nil
	})
	<-aStarted
	b := out.Task("b").Define(func(context.Context) error {
		p.enter()
		defer p.leave()
		return nil
	})
	if err := b.wait(); err != nil {
		t.Fatalf("b.Wait() = %v", err)
	}
	if got := p.peak.Load(); got != 1 {
		t.Fatalf("peak running callbacks = %d at MaxConcurrency 1, want 1", got)
	}
}

// TestSpawnedPlainWaitersRespectTheCeiling: the Go fan-out idiom — many
// plain goroutines each Define+Wait their own Task — must still run at most
// MaxConcurrency callbacks at once.
func TestSpawnedPlainWaitersRespectTheCeiling(t *testing.T) {
	const ceiling, waiters = 2, 8
	out := quietOutput(Config{MaxConcurrency: ceiling})
	t.Cleanup(func() { _ = out.close() })

	var p peakCounter
	var wg sync.WaitGroup
	for i := range waiters {
		wg.Go(func() {
			err := out.Task(fmt.Sprintf("t%d", i)).Define(func(context.Context) error {
				p.enter()
				defer p.leave()
				time.Sleep(20 * time.Millisecond)
				return nil
			}).wait()
			if err != nil {
				t.Errorf("t%d.Wait() = %v", i, err)
			}
		})
	}
	wg.Wait()
	if got := p.peak.Load(); got > ceiling {
		t.Fatalf("peak running callbacks = %d at MaxConcurrency %d, want <= %d", got, ceiling, ceiling)
	}
}
