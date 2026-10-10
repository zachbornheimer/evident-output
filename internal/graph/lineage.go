package graph

import (
	"maps"
	"slices"
	"sync"
)

// processLineage covers every goroutine in the process: who started a
// goroutine is a fact about the process, not about one Graph.
var processLineage lineage

// lineage remembers which goroutine started each goroutine it has seen. A
// goroutine's creator is fixed when it starts and ids are never reused, so a
// remembered creator never goes stale. Reading creators means a traceback of
// every goroutine, which stops the world: one dump teaches the creator of
// every goroutine alive then, so a goroutine pays for at most one, and one it
// shares a dump with pays for none.
type lineage struct {
	mu       sync.Mutex
	creators map[GoroutineID]GoroutineID
}

// ancestors lists the goroutines that started self, nearest first, as far as
// the runtime named them: a creator that exited before anything saw it ends
// the chain. self must be the calling goroutine, which a dump always lists.
func (l *lineage) ancestors(self GoroutineID) []GoroutineID {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, known := l.creators[self]; !known {
		l.learnLocked()
	}
	var chain []GoroutineID
	for id := l.creators[self]; id != 0 && len(chain) <= len(l.creators); id = l.creators[id] {
		chain = append(chain, id)
	}
	return chain
}

// learnLocked replaces what is remembered with one dump of every goroutine
// alive now, keeping the exited goroutines a live one still names as its
// ancestor, so a chain through an exited creator survives while it is useful
// and the memory follows the live goroutines rather than every goroutine ever.
func (l *lineage) learnLocked() {
	live := goroutineCreators(allGoroutineTraces())
	for _, creator := range slices.Collect(maps.Values(live)) {
		for id := creator; id != 0; {
			if _, seen := live[id]; seen {
				break
			}
			older, remembered := l.creators[id]
			if !remembered {
				break
			}
			live[id] = older
			id = older
		}
	}
	l.creators = live
}
