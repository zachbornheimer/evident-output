package graph

import (
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// runOnOneSlot submits one callback per name on a graph whose ceiling is a
// single slot, so the callbacks run one after another, and returns the
// goroutine each ran on in submission order.
func runOnOneSlot(t *testing.T, bodies ...func()) []GoroutineID {
	t.Helper()
	g := New(record.NewRun(), WithMaxConcurrency(1))
	t.Cleanup(g.Close)
	ids := make([]GoroutineID, len(bodies))
	for i, body := range bodies {
		task := declare(g, string(rune('a'+i)))
		submitWork(g, task, settlingWork(g, task, func() error {
			ids[i] = CurrentGoroutine()
			body()
			return nil
		}))
	}
	g.Drain()
	return ids
}

func TestWorkerRunsTheNextClaimWhenItsCallbackStartedNoGoroutine(t *testing.T) {
	trivial := func() {}

	ids := runOnOneSlot(t, trivial, trivial, trivial)

	if ids[0] != ids[1] || ids[1] != ids[2] {
		t.Errorf("callbacks ran on goroutines %v, want one worker for all three", ids)
	}
}

// A goroutine a callback started may outlive it and ask who started it. Its
// creator must then still be nobody's but that callback's, so the worker that
// ran the callback takes no further work.
func TestWorkerEndsWhenItsCallbackStartedAGoroutine(t *testing.T) {
	release := make(chan struct{})
	var started sync.WaitGroup
	started.Add(1)
	spawning := func() {
		go func() {
			started.Done()
			<-release
		}()
		started.Wait()
	}
	defer close(release)

	ids := runOnOneSlot(t, spawning, func() {})

	if ids[0] == ids[1] {
		t.Errorf("both callbacks ran on goroutine %d, want a fresh worker after one that started a goroutine", ids[0])
	}
}

func TestNextClaimStillRunsWhenACallbackExitsItsGoroutine(t *testing.T) {
	exiting := func() { runtime.Goexit() }
	ran := false

	ids := runOnOneSlot(t, exiting, func() { ran = true })

	if !ran {
		t.Errorf("the claim after a callback that exited its goroutine never ran (ids %v)", ids)
	}
}

func TestDrainEndsTheWorkersWaitingForAClaim(t *testing.T) {
	before := runtime.NumGoroutine()
	g := New(record.NewRun(), WithMaxConcurrency(1))
	task := declare(g, "only")
	submitWork(g, task, settlingWork(g, task, func() error { return nil }))

	g.Drain()

	deadline := time.Now().Add(executorTimeout)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if after := runtime.NumGoroutine(); after > before {
		t.Errorf("%d goroutines after the drain, want the %d from before the run", after, before)
	}
}
