package graph

import (
	"runtime"
	"testing"
)

// maxRelayChain bounds the chain of the last relay generation: itself, the
// test goroutine that started the first generation, and what started that.
const maxRelayChain = 16

// A relay is a goroutine that starts its successor and exits, as a
// self-restarting loop does. The successor asks for its lineage while its
// predecessor is still alive, so a dump sees every generation alive once. An
// exited goroutine carries no frame, so it never changes whom a chain
// attributes to: each generation points at its nearest live or pooled-worker
// ancestor, and what the lineage holds follows the goroutines alive however
// many generations have passed.
func TestLineageRelayGenerationsStayBounded(t *testing.T) {
	const generations = 2000
	var l lineage
	ready := make(chan GoroutineID)
	hold := make(chan struct{})
	defer close(hold)

	var peakAlive int
	var relay func(n int, handedOver chan struct{})
	relay = func(n int, handedOver chan struct{}) {
		l.chain(CurrentGoroutine(), nil)
		peakAlive = max(peakAlive, runtime.NumGoroutine())
		if handedOver != nil {
			close(handedOver)
		}
		if n == generations {
			ready <- CurrentGoroutine()
			<-hold
			return
		}
		next := make(chan struct{})
		go relay(n+1, next)
		<-next
	}
	go relay(1, nil)
	last := <-ready

	retained := l.retained()
	chain := l.chain(last, nil)
	t.Logf("%d relay generations: retained %d on a peak of %d live goroutines, chain of the last %d", generations, retained, peakAlive, len(chain))
	if limit := 4*peakAlive + 64; retained > limit {
		t.Errorf("the lineage held %d goroutine ids across %d relay generations on at most %d live goroutines (limit %d)",
			retained, generations, peakAlive, limit)
	}
	if len(chain) > maxRelayChain {
		t.Errorf("the chain of the last generation is %d ids long (limit %d): a dump walks every generation that already exited", len(chain), maxRelayChain)
	}
	if len(chain) == 0 || chain[0] != last {
		t.Errorf("the chain of the last generation is %v, want it to start at %d", chain, last)
	}
}
