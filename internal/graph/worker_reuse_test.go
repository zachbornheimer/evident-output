package graph

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// withSpawnLedger runs the graph's workers against l.
func withSpawnLedger(l *spawnLedger) Option {
	return func(g *Graph) { g.workers.spawns = l }
}

// ownSpawnsOnly is a ledger that counts only the goroutines it starts, so the
// worker census cannot see a goroutine any other code in the process starts.
func ownSpawnsOnly() *spawnLedger {
	l := &spawnLedger{}
	l.created = func() (uint64, bool) { return l.done.Load(), true }
	return l
}

// runOnOneSlot submits one callback per body on a graph whose ceiling is a
// single slot, so the callbacks run one after another, and returns the
// goroutine each ran on in submission order. Every body is queued before the
// first one runs, so each claim after the first is taken by the worker that
// ends the claim before it, never by one that has not yet gone idle.
func runOnOneSlot(t *testing.T, ledger *spawnLedger, bodies ...func()) []GoroutineID {
	t.Helper()
	g := New(record.NewRun(), WithMaxConcurrency(1), withSpawnLedger(ledger))
	t.Cleanup(g.Close)
	queued := make(chan struct{})
	ids := make([]GoroutineID, len(bodies))
	for i, body := range bodies {
		task := declare(g, string(rune('a'+i)))
		submitWork(g, task, settlingWork(g, task, func() error {
			if i == 0 {
				<-queued
			}
			ids[i] = CurrentGoroutine()
			body()
			return nil
		}))
	}
	close(queued)
	g.Drain()
	return ids
}

// Reuse is read off the ledger, not off a scheduling race: three claims that
// start no goroutine cost exactly one worker start, however many processors
// the host has and whatever else the process is doing.
func TestWorkerReuseServesConsecutiveClaimsOnOneWorker(t *testing.T) {
	trivial := func() {}
	ledger := ownSpawnsOnly()

	ids := runOnOneSlot(t, ledger, trivial, trivial, trivial)

	if starts := ledger.done.Load(); starts != 1 {
		t.Errorf("3 claims started %d workers (callbacks ran on goroutines %v), want 1", starts, ids)
	}
	if ids[0] != ids[1] || ids[1] != ids[2] {
		t.Errorf("callbacks ran on goroutines %v, want one worker for all three", ids)
	}
}

// Without the runtime's goroutine count (Go before 1.27) the census proves
// nothing, so no worker is reusable: every callback still runs, each on a
// goroutine of its own.
func TestWorkerReuseFallbackRunsEachCallbackOnItsOwnGoroutine(t *testing.T) {
	const callbacks = 20
	ledger := &spawnLedger{created: func() (uint64, bool) { return 0, false }}
	var ran atomic.Int64
	bodies := make([]func(), callbacks)
	for i := range bodies {
		bodies[i] = func() { ran.Add(1) }
	}

	ids := runOnOneSlot(t, ledger, bodies...)

	if got := ran.Load(); got != callbacks {
		t.Errorf("%d of %d callbacks ran", got, callbacks)
	}
	seen := make(map[GoroutineID]bool, callbacks)
	for _, id := range ids {
		if seen[id] {
			t.Errorf("goroutine %d ran two callbacks, want one goroutine per callback (ids %v)", id, ids)
		}
		seen[id] = true
	}
	if starts := ledger.done.Load(); starts != callbacks {
		t.Errorf("%d worker starts for %d callbacks, want one each", starts, callbacks)
	}
}

// A goroutine any code starts while a claim runs ends the worker, so a busy
// process declines reuse; it never serves a claim a goroutine's creator should
// not.
func TestWorkerReuseIsDeclinedWhenTheProcessStartsAGoroutine(t *testing.T) {
	ledger := &spawnLedger{}
	var stray sync.WaitGroup
	stray.Add(1)
	startsOne := func() {
		go stray.Done()
		stray.Wait()
	}

	ids := runOnOneSlot(t, ledger, startsOne, func() {})

	if ids[0] == ids[1] {
		t.Errorf("both callbacks ran on goroutine %d, want a fresh worker after a goroutine started", ids[0])
	}
}

// A goroutine a callback started may outlive it and ask who started it. Its
// creator must then still be nobody's but that callback's, so the worker that
// ran the callback takes no further work while that goroutine is alive.
func TestWorkerEndsWhileItsCallbacksGoroutineIsAlive(t *testing.T) {
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
	var nextRanBeside atomic.Bool
	next := func() {
		select {
		case <-release:
		default:
			nextRanBeside.Store(true)
		}
	}

	ids := runOnOneSlot(t, &spawnLedger{}, spawning, next)

	if ids[0] == ids[1] {
		t.Errorf("both callbacks ran on goroutine %d, want a fresh worker after one that started a goroutine", ids[0])
	}
	if !nextRanBeside.Load() {
		t.Error("the second claim never ran while the first callback's goroutine was alive")
	}
}

// A callback that exits through runtime.Goexit ends its worker: the claim after
// it runs on another goroutine.
func TestWorkerEndsWhenItsCallbackExitsItsGoroutine(t *testing.T) {
	exiting := func() { runtime.Goexit() }
	ran := false

	ids := runOnOneSlot(t, ownSpawnsOnly(), exiting, func() { ran = true })

	if !ran {
		t.Fatalf("the claim after a callback that exited its goroutine never ran (ids %v)", ids)
	}
	if ids[0] == ids[1] {
		t.Errorf("both callbacks ran on goroutine %d, want a fresh worker after a Goexit", ids[0])
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
