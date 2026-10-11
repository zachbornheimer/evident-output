package graph

import (
	"slices"
	"testing"
	"time"
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
	tr := lineageTree{exitParent: make(chan struct{}), exitChild: make(chan struct{})}
	ids := make(chan [2]GoroutineID)
	go func() {
		parent := CurrentGoroutine()
		go func() {
			l.chain(CurrentGoroutine(), nil)
			ids <- [2]GoroutineID{parent, CurrentGoroutine()}
			<-tr.exitChild
		}()
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
		if _, alive := goroutineCreators(allGoroutineTraces())[id]; !alive {
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

// The goroutines a goroutine started, directly or through others, are its
// descendants; the goroutine itself and the ones it did not start are not.
func TestLineageDescendantsAreWhatARootStartedThroughOthers(t *testing.T) {
	var l lineage
	tr := startLineageTree(&l)
	defer close(tr.exitChild)
	defer close(tr.exitParent)

	underParent := l.descendantsOf(tr.parent)
	if _, ok := underParent[tr.child]; !ok {
		t.Fatalf("descendantsOf(parent) = %v, want the child %d", underParent, tr.child)
	}
	if _, ok := underParent[tr.parent]; ok {
		t.Fatalf("descendantsOf(parent) = %v lists the parent itself", underParent)
	}
	if underChild := l.descendantsOf(tr.child); len(underChild) != 0 {
		t.Fatalf("descendantsOf(child) = %v, want none: the child started no goroutine", underChild)
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
