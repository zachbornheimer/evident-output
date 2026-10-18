package graph

import (
	"runtime"
	"testing"
)

// A callback that starts a goroutine is not reused, so a run of such claims
// builds a chain of workers, each started by the one before. A chain ends at
// the first worker (see lineage.workers), so nothing above a worker is ever
// read: what the lineage holds must follow the goroutines alive however many
// worker generations a run has passed through, and a dump must not walk the
// generations that already exited.
func TestLineageRetainedStaysBoundedAcrossWorkerGenerations(t *testing.T) {
	const generations = 1500
	var l lineage
	var ledger spawnLedger
	ready := make(chan GoroutineID)
	hold := make(chan struct{})
	defer close(hold)

	var peakRetained, peakAlive, earlyRetained int
	var generation func(n int)
	generation = func(n int) {
		dumpFromANewGoroutine(&l)
		peakRetained = max(peakRetained, l.retained())
		peakAlive = max(peakAlive, runtime.NumGoroutine())
		if n == generations/10 {
			earlyRetained = l.retained()
		}
		if n == generations {
			ready <- CurrentGoroutine()
			<-hold
			return
		}
		ledger.start(func() { generation(n + 1) })
	}
	ledger.start(func() { generation(1) })
	last := <-ready

	t.Logf("%d generations: retained peak %d, live peak %d", generations, peakRetained, peakAlive)
	if limit := 4*peakAlive + 64; peakRetained > limit {
		t.Errorf("the lineage held %d goroutine ids across %d worker generations on at most %d live goroutines (limit %d)",
			peakRetained, generations, peakAlive, limit)
	}
	if late := l.retained(); late > earlyRetained+16 {
		t.Errorf("what the lineage holds grew from %d ids after %d generations to %d after %d",
			earlyRetained, generations/10, late, generations)
	}
	if chain := l.chain(last, nil); len(chain) != 1 || chain[0] != last {
		t.Errorf("the chain of the last worker is %v, want just [%d]: a chain ends at its first worker", chain, last)
	}
}
