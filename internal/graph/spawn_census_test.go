package graph

import (
	"sync"
	"testing"
)

// A goroutine the ledger starts takes its census with its own spawn already
// counted: the census is consistent and names the spawn as the ledger's.
// Otherwise the first claim of every worker would read an inconsistent
// census whenever its creator is slower to finish than it is to start, and
// the worker would decline reuse.
func TestSpawnedGoroutineSeesItsOwnSpawnCounted(t *testing.T) {
	const spawns = 20000
	ledger := ownSpawnsOnly()
	var wg sync.WaitGroup
	inconsistent := make(chan census, spawns)
	for range spawns {
		wg.Add(1)
		ledger.start(func() {
			defer wg.Done()
			if c := ledger.count(); !c.consistent {
				inconsistent <- c
			}
		})
		wg.Wait()
	}
	if n := len(inconsistent); n > 0 {
		t.Errorf("%d of %d spawned goroutines read an inconsistent census of their own spawn", n, spawns)
	}
}
