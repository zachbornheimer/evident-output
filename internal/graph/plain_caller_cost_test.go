package graph

import (
	"testing"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

const (
	idleGoroutines = 5000
	plainCalls     = 200
	// fewDumps is the stops of the world a stretch of calls may take
	// together: the first one learns what is alive, a spare covers a
	// goroutine the runtime starts meanwhile. A call that stops the world
	// each time takes plainCalls of them, however fast the machine.
	fewDumps = 3
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

// dumpsDuring is the stops of the world the process's lineage took while body
// ran. Counting them instead of timing body keeps the verdict the same on a
// slow machine, under the race detector, and next to other test packages.
func dumpsDuring(body func()) int64 {
	before := processLineage.dumps.Load()
	body()
	return processLineage.dumps.Load() - before
}

// A plain caller's Computed read while any callback runs must not stop the
// world to dump every goroutine each time: its cost must not scale with the
// process's goroutine count.
func TestPlainCallerReadCostIsIndependentOfGoroutineCount(t *testing.T) {
	var dumps int64
	withBusyCallback(t, idleGoroutines, func(g *Graph) {
		dumps = dumpsDuring(func() {
			for range plainCalls {
				if r := g.CurrentReader(); r.Task != nil {
					t.Fatalf("plain caller read as %v", r.Task.Name)
				}
			}
		})
	})
	if dumps > fewDumps {
		t.Fatalf("%d plain-caller reads with %d idle goroutines took %d dumps, over %d: each read stops the world",
			plainCalls, idleGoroutines, dumps, fewDumps)
	}
}
