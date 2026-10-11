package graph

import "testing"

// A goroutine two levels below its asker's known ancestor (each item goes to
// a fresh goroutine, which hands it to another fresh goroutine that reads once,
// as an errgroup per item does) asks about a creator nothing has seen. The
// dump that would name that creator cannot once it exited, so the number of
// dumps while a callback runs is capped, not one per item.
func TestTwoLevelFreshGoroutineReadsTakeAtMostTheRefreshAllowanceInDumps(t *testing.T) {
	var dumps int64
	withBusyCallback(t, idleGoroutines, func(g *Graph) {
		before := processLineage.dumps.Load()
		for range plainCalls {
			got := make(chan Reader)
			go func() { go func() { got <- g.CurrentReader() }() }()
			if r := <-got; r.Task != nil {
				t.Fatalf("plain reader read as %s", r.Task.Name)
			}
		}
		dumps = processLineage.dumps.Load() - before
	})
	if dumps > refreshesPerCallback {
		t.Fatalf("%d two-level fresh-goroutine reads took %d dumps, over the allowance of %d",
			plainCalls, dumps, refreshesPerCallback)
	}
}
