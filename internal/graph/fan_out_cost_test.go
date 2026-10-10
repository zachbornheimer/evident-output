package graph

import (
	"testing"
	"time"
)

// A plain caller that starts a fresh goroutine for every read, while a
// callback runs, asks about a goroutine nothing has seen before each time.
// The goroutine that started them is one, though, and the process knows it
// after the first: no read may stop the world again.
func TestFreshGoroutineReadCostIsIndependentOfGoroutineCount(t *testing.T) {
	var took time.Duration
	withBusyCallback(t, idleGoroutines, func(g *Graph) {
		start := time.Now()
		for range plainCalls {
			done := make(chan Reader)
			go func() { done <- g.CurrentReader() }()
			if r := <-done; r.Task != nil {
				t.Fatalf("plain read as %v", r.Task.Name)
			}
		}
		took = time.Since(start)
	})
	if took > plainCallBudget {
		t.Fatalf("%d fresh-goroutine reads with %d idle goroutines took %v, over %v: each read stops the world",
			plainCalls, idleGoroutines, took, plainCallBudget)
	}
}
