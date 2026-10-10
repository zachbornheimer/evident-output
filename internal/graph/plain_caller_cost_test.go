package graph

import (
	"fmt"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/record"
)

const (
	idleGoroutines = 5000
	plainCalls     = 200
	// plainCallBudget is all the time plainCalls reads may take together:
	// generous for a read that costs microseconds, far under one that stops
	// the world per call (about 10ms with idleGoroutines alive).
	plainCallBudget = 100 * time.Millisecond
)

// withBusyCallback runs body from the test goroutine (a plain caller, outside
// every callback) while one callback is running and idle goroutines are
// alive in the process.
func withBusyCallback(t *testing.T, idle int, body func(g *Graph)) {
	t.Helper()
	stop := make(chan struct{})
	defer close(stop)
	for range idle {
		go func() { <-stop }()
	}
	g := New(record.NewRun(), WithMaxConcurrency(8))
	busy := declare(g, "busy")
	started, release := make(chan struct{}), make(chan struct{})
	submitWork(g, busy, settlingWork(g, busy, func() error { close(started); <-release; return nil }))
	g.Kick()
	<-started
	body(g)
	close(release)
	g.Drain()
}

// A plain caller's Computed read while any callback runs must not stop the
// world to dump every goroutine each time: its cost must not scale with the
// process's goroutine count.
func TestPlainCallerReadCostIsIndependentOfGoroutineCount(t *testing.T) {
	var took time.Duration
	withBusyCallback(t, idleGoroutines, func(g *Graph) {
		start := time.Now()
		for range plainCalls {
			if r := g.CurrentReader(); r.Task != nil {
				t.Fatalf("plain caller read as %v", r.Task.Name)
			}
		}
		took = time.Since(start)
	})
	if took > plainCallBudget {
		t.Fatalf("%d plain-caller reads with %d idle goroutines took %v, over %v: each read stops the world",
			plainCalls, idleGoroutines, took, plainCallBudget)
	}
}

// A plain caller's parked Wait while a callback runs must not stop the world
// each time either.
func TestPlainCallerWaitCostIsIndependentOfGoroutineCount(t *testing.T) {
	var took time.Duration
	withBusyCallback(t, idleGoroutines, func(g *Graph) {
		start := time.Now()
		for i := range plainCalls {
			task := declare(g, fmt.Sprint("w", i))
			submitWork(g, task, settlingWork(g, task, func() error { time.Sleep(100 * time.Microsecond); return nil }))
			g.Kick()
			if err := g.Wait(task); err != nil {
				t.Fatal(err)
			}
		}
		took = time.Since(start)
	})
	budget := plainCallBudget + plainCalls*time.Millisecond
	if took > budget {
		t.Fatalf("%d plain-caller Waits with %d idle goroutines took %v, over %v: each Wait stops the world",
			plainCalls, idleGoroutines, took, budget)
	}
}
