package freshness

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// gates is a BarrierTable the tests drive directly.
type gates struct {
	mu    sync.Mutex
	table map[string]chan struct{}
}

func newGates() *gates { return &gates{table: map[string]chan struct{}{}} }

func (g *gates) OpenBarrier(path string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.table[path]; !ok {
		g.table[path] = make(chan struct{})
	}
}

func (g *gates) SettleBarrier(path string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if ch, ok := g.table[path]; ok {
		select {
		case <-ch:
		default:
			close(ch)
		}
	}
}

func (g *gates) Barrier(path string) (<-chan struct{}, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	ch, ok := g.table[path]
	return ch, ok
}

// awaitSettle is how long a test waits to prove a wait is blocked.
const awaitSettle = 50 * time.Millisecond

// TestAwaitNeverBlocksOnAPathNobodyClaimed proves spec §11.6: a Basis input
// no Task in this Run produces is not delayed.
func TestAwaitNeverBlocksOnAPathNobodyClaimed(t *testing.T) {
	barrier := NewOutputBarrier(newGates())
	if err := barrier.Await(t.Context(), "/unclaimed"); err != nil {
		t.Fatalf("Await = %v, want nil", err)
	}
}

// TestAwaitWaitsForTheProducerToSettle proves spec §11.6: a consumer waits
// until the producer's gate closes, and Settle twice never panics.
func TestAwaitWaitsForTheProducerToSettle(t *testing.T) {
	barrier := NewOutputBarrier(newGates())
	barrier.Open("/out")
	done := make(chan error, 1)
	go func() { done <- barrier.Await(t.Context(), "/out") }()
	select {
	case err := <-done:
		t.Fatalf("Await returned %v before the producer settled", err)
	case <-time.After(awaitSettle):
	}
	barrier.Settle("/out")
	barrier.Settle("/out")
	if err := <-done; err != nil {
		t.Fatalf("Await = %v after settle, want nil", err)
	}
}

// TestAwaitStopsWhenTheContextEnds proves a waiting consumer is never stuck
// behind a producer that never settles.
func TestAwaitStopsWhenTheContextEnds(t *testing.T) {
	barrier := NewOutputBarrier(newGates())
	barrier.Open("/out")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := barrier.Await(ctx, "/out"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Await = %v, want context.Canceled", err)
	}
}
