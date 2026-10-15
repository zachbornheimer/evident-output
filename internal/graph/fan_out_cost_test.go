package graph

import "testing"

// A plain caller that starts a fresh goroutine for every read, while a
// callback runs, asks about a goroutine nothing has seen before each time.
// The goroutine that started them is one, though, and the process knows it
// after the first: no read may stop the world again.
func TestFreshGoroutineReadCostIsIndependentOfGoroutineCount(t *testing.T) {
	var dumps int64
	withBusyCallback(t, idleGoroutines, func(g *Graph) {
		dumps = dumpsDuring(func() {
			for range plainCalls {
				done := make(chan Reader)
				go func() { done <- g.CurrentReader() }()
				if r := <-done; r.Task != nil {
					t.Fatalf("plain read as %v", r.Task.Name)
				}
			}
		})
	})
	if dumps > fewDumps {
		t.Fatalf("%d fresh-goroutine reads with %d idle goroutines took %d dumps, over %d: each read stops the world",
			plainCalls, idleGoroutines, dumps, fewDumps)
	}
}
