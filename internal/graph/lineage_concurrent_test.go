package graph

import (
	"runtime"
	"sync"
	"testing"
)

// A goroutine asking about a creator that is alive, and that no dump has seen,
// is always told who started the creator, however many processors start
// goroutines at once: the count that stands in for a dump (see
// lineage.completeLocked) must already include the goroutine that asks.
func TestLineageNamesALiveCreatorWhileOtherProcessorsStartGoroutines(t *testing.T) {
	if _, counted := goroutinesCreated(); !counted {
		t.Skip("this toolchain does not count the goroutines the process started")
	}
	const rounds = 100
	var wg sync.WaitGroup
	for range 2 * runtime.GOMAXPROCS(0) {
		wg.Go(func() {
			worker := CurrentGoroutine()
			for range rounds {
				chains, release := make(chan []GoroutineID), make(chan struct{})
				go func() {
					go func() {
						_, creator := CurrentGoroutineLineage()
						chains <- processLineage.chain(creator, nil)
					}()
					<-release
				}()
				chain := <-chains
				close(release)
				if len(chain) < 2 || chain[1] != worker {
					t.Errorf("chain = %v, want the live creator, then worker %d that started it", chain, worker)
					return
				}
			}
		})
	}
	wg.Wait()
}
