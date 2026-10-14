package graph

import (
	"fmt"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// lineageTree is a parent goroutine and the child it started, both alive until
// their exit channels close. The child asked for its own lineage before the
// tree was returned, so the lineage has seen both.
type lineageTree struct {
	parent, child GoroutineID
	exitParent    chan struct{}
	exitChild     chan struct{}
}

func startLineageTree(l *lineage) lineageTree {
	return startLineageTreeVia(l, func(f func()) { go f() })
}

// startLineageTreeVia is startLineageTree with the child started by start, as
// a pooled worker is started by its ledger.
func startLineageTreeVia(l *lineage, start func(func())) lineageTree {
	tr := lineageTree{exitParent: make(chan struct{}), exitChild: make(chan struct{})}
	ids := make(chan [2]GoroutineID)
	go func() {
		parent := CurrentGoroutine()
		start(func() {
			l.chain(CurrentGoroutine(), nil)
			ids <- [2]GoroutineID{parent, CurrentGoroutine()}
			<-tr.exitChild
		})
		<-tr.exitParent
	}()
	got := <-ids
	tr.parent, tr.child = got[0], got[1]
	return tr
}

// remembers reports whether the lineage holds a creator for goroutine id.
func (l *lineage) remembers(id GoroutineID) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, known := l.creators[id]
	return known
}

// dumpFromANewGoroutine makes l take a fresh dump: a goroutine started now is
// one l cannot have seen.
func dumpFromANewGoroutine(l *lineage) {
	done := make(chan struct{})
	go func() { l.chain(CurrentGoroutine(), nil); close(done) }()
	<-done
}

// goneFromDumps waits until goroutine id no longer appears in a dump of every
// goroutine.
func goneFromDumps(t *testing.T, id GoroutineID) {
	t.Helper()
	for range 1000 {
		creators, _ := goroutineOrigins(allGoroutineTraces())
		if _, alive := creators[id]; !alive {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("goroutine %d is still alive", id)
}

// A goroutine whose creator has exited keeps the chain it was first seen
// with, however many later goroutines trigger a fresh dump.
func TestLineageKeepsAnExitedCreatorALiveGoroutineNames(t *testing.T) {
	var l lineage
	tr := startLineageTree(&l)
	defer close(tr.exitChild)
	want := l.chain(tr.child, nil)
	if len(want) < 3 || want[0] != tr.child || want[1] != tr.parent {
		t.Fatalf("chain(child) = %v, want the child, then the parent %d, then who started it", want, tr.parent)
	}

	close(tr.exitParent)
	goneFromDumps(t, tr.parent)
	dumpFromANewGoroutine(&l)

	if got := l.chain(tr.child, nil); !slices.Equal(got, want) {
		t.Fatalf("chain(child) after the parent exited = %v, want %v", got, want)
	}
}

// Memory follows the live goroutines: an exited goroutine nothing alive names
// is forgotten at the next dump.
func TestLineageForgetsExitedGoroutinesNobodyNames(t *testing.T) {
	var l lineage
	tr := startLineageTree(&l)
	close(tr.exitChild)
	close(tr.exitParent)
	goneFromDumps(t, tr.child)
	goneFromDumps(t, tr.parent)
	if !l.remembers(tr.child) {
		t.Fatalf("goroutine %d unknown right after it asked", tr.child)
	}

	dumpFromANewGoroutine(&l)
	if l.remembers(tr.child) || l.remembers(tr.parent) {
		t.Fatalf("exited goroutines %d and %d are still remembered after a fresh dump", tr.parent, tr.child)
	}
}

// A goroutine seen in an earlier dump is answered without another: its
// creator is already known.
func TestLineageAnswersAKnownGoroutineFromMemory(t *testing.T) {
	var l lineage
	self := CurrentGoroutine()
	first := l.chain(self, nil)
	l.mu.Lock()
	l.creators[GoroutineID(1<<62)] = 0 // a dump would drop an id nothing runs as
	l.mu.Unlock()

	if got := l.chain(self, nil); !slices.Equal(got, first) || !l.remembers(GoroutineID(1<<62)) {
		t.Fatalf("a known goroutine asked again: chain = %v (was %v), marker kept = %v: it dumped again",
			got, first, l.remembers(GoroutineID(1<<62)))
	}
}

// Every goroutine a known creator started is answered from memory: the
// creator was seen once, and how many fresh goroutines ask about it does not
// matter.
func TestLineageAnswersFreshGoroutinesOfAKnownCreatorFromMemory(t *testing.T) {
	var l lineage
	creator := CurrentGoroutine()
	want := l.chain(creator, nil)
	l.mu.Lock()
	l.creators[GoroutineID(1<<62)] = 0 // a dump would drop an id nothing runs as
	l.mu.Unlock()

	for range 3 {
		got := make(chan []GoroutineID)
		go func() { got <- l.chain(creator, nil) }()
		if chain := <-got; !slices.Equal(chain, want) || !l.remembers(GoroutineID(1<<62)) {
			t.Fatalf("a fresh goroutine asked about the known creator: chain = %v (was %v), marker kept = %v: it dumped",
				chain, want, l.remembers(GoroutineID(1<<62)))
		}
	}
}

// A creator that exited before anything saw it ends the chain, and is
// answered from memory the next time: ids are never reused, so asking again
// can learn nothing a dump would not have.
func TestLineageAnswersAnExitedUnseenCreatorFromMemory(t *testing.T) {
	var l lineage
	ids := make(chan GoroutineID)
	go func() { ids <- CurrentGoroutine() }()
	gone := <-ids
	goneFromDumps(t, gone)
	if chain := l.chain(gone, nil); !slices.Equal(chain, []GoroutineID{gone}) {
		t.Fatalf("chain(exited) = %v, want only %d", chain, gone)
	}
	l.mu.Lock()
	l.creators[GoroutineID(1<<62)] = 0 // a dump would drop an id nothing runs as
	l.mu.Unlock()

	if l.chain(gone, nil); !l.remembers(GoroutineID(1 << 62)) {
		t.Fatal("an exited creator asked again: it dumped again")
	}
}

// A chain ends at a pooled worker: its creator is not named, however the
// worker's own creator is known.
func TestChainEndsAtAPooledWorker(t *testing.T) {
	var l lineage
	var ledger spawnLedger
	tr := startLineageTreeVia(&l, ledger.start)
	defer close(tr.exitParent)
	defer close(tr.exitChild)
	if got := l.chain(tr.child, nil); !slices.Equal(got, []GoroutineID{tr.child}) {
		t.Fatalf("chain = %v, want only the worker %d", got, tr.child)
	}
}

// Only a goroutine the ledger started is a pooled worker; one started any
// other way is a creator the chain goes through.
func TestChainPassesThroughAGoroutineTheLedgerDidNotStart(t *testing.T) {
	var l lineage
	tr := startLineageTree(&l)
	defer close(tr.exitParent)
	defer close(tr.exitChild)
	if got := l.chain(tr.child, nil); len(got) < 2 || got[1] != tr.parent {
		t.Fatalf("chain = %v, want the child, then the parent %d", got, tr.parent)
	}
}

// A worker keeps ending the chain after it exits, while a live goroutine still
// names it as its creator and later dumps run.
func TestExitedPooledWorkerStillEndsAChain(t *testing.T) {
	var l lineage
	var ledger spawnLedger
	tr := startLineageTree(&l)
	defer close(tr.exitChild)
	grandchild := make(chan GoroutineID)
	exitWorker := make(chan struct{})
	ledger.start(func() {
		go func() {
			grandchild <- CurrentGoroutine()
			<-tr.exitParent
		}()
		<-exitWorker
	})
	g := <-grandchild
	want := l.chain(g, nil)
	if len(want) != 2 {
		t.Fatalf("chain(grandchild) = %v, want the grandchild and its worker", want)
	}
	worker := want[1]
	close(exitWorker)
	goneFromDumps(t, worker)
	dumpFromANewGoroutine(&l)
	defer close(tr.exitParent)

	if got := l.chain(g, nil); !slices.Equal(got, want) {
		t.Fatalf("chain(grandchild) after its worker exited = %v, want %v", got, want)
	}
}

// What marks a worker is the ledger's own frame in the traceback, so that the
// name the dump is read for is the function that starts workers.
func TestPooledWorkerCreatorNamesTheLedgerStart(t *testing.T) {
	var ledger spawnLedger
	found := make(chan bool)
	ledger.start(func() {
		_, workers := goroutineOrigins(allGoroutineTraces())
		_, ok := workers[CurrentGoroutine()]
		found <- ok
	})
	if !<-found {
		t.Fatal("a goroutine started through spawnLedger.start is not read as a pooled worker")
	}
}

// A pooled worker nobody names any more is forgotten at the next dump, along
// with the goroutines.
func TestLineageForgetsPooledWorkersWithTheirGoroutines(t *testing.T) {
	var l lineage
	var ledger spawnLedger
	tr := startLineageTreeVia(&l, ledger.start)
	close(tr.exitChild)
	close(tr.exitParent)
	goneFromDumps(t, tr.child)
	goneFromDumps(t, tr.parent)

	dumpFromANewGoroutine(&l)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.isWorkerLocked(tr.child) {
		t.Fatalf("exited worker %d is still remembered after a fresh dump", tr.child)
	}
}

// retained is how many goroutine ids the lineage holds knowledge about.
func (l *lineage) retained() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.creators) + len(l.workers)
}

// What the lineage holds follows the goroutines alive, not the workers ever
// started: graphs whose workers are never reused, and whose callbacks never
// read a Computed from a goroutine they started (so no dump ever replaces what
// is held), leave it no larger than the goroutines alive after them.
func TestLineageIsBoundedByLiveGoroutines(t *testing.T) {
	noReuse := &spawnLedger{created: func() (uint64, bool) { return 0, false }}
	const graphs, claims = 4, 2500
	for range graphs {
		g := New(record.NewRun(), WithMaxConcurrency(4), withSpawnLedger(noReuse))
		for i := range claims {
			task := declare(g, fmt.Sprintf("t%d", i))
			submitWork(g, task, settlingWork(g, task, func() error { return nil }))
		}
		g.Drain()
		g.Close()
	}
	retained, alive := processLineage.retained(), runtime.NumGoroutine()
	if limit := 4*alive + 64; retained > limit {
		t.Fatalf("the lineage holds %d goroutine ids after %d claims on %d live goroutines (limit %d)",
			retained, graphs*claims, alive, limit)
	}
}
