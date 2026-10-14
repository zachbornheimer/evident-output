package graph

import (
	"maps"
	"slices"
	"sync"
	"sync/atomic"
)

// processLineage covers every goroutine in the process: who started a
// goroutine is a fact about the process, not about one Graph.
var processLineage lineage

// lineage remembers which goroutine started each goroutine it has seen. A
// goroutine's creator is fixed when it starts and ids are never reused, so a
// remembered creator never goes stale. Reading creators means a traceback of
// every goroutine, which stops the world: one dump teaches the creator of
// every goroutine alive then, so a creator pays for at most one, and the
// goroutines it started, asking about it afterwards, pay for none. While the
// process starts no goroutine (see goroutinesCreated) the last dump is still
// the whole truth, and a question about a goroutine it did not name, or about
// who started which, is answered without another.
type lineage struct {
	mu       sync.Mutex
	creators map[GoroutineID]GoroutineID
	// started inverts creators as of the last dump: the goroutines each
	// goroutine started.
	started map[GoroutineID][]GoroutineID
	// workers is the goroutines among creators that serve pooled claims. A
	// chain ends at one: the goroutine that started a worker never authored
	// the work the worker runs, so what that creator is running says nothing
	// about a goroutine the worker's work started, whether the worker is still
	// there or has gone. The dump names them, so they are kept exactly as long
	// as creators keeps them.
	workers map[GoroutineID]struct{}
	// created is how many goroutines the process had started when the last
	// dump began; counted is false where the toolchain keeps no such count.
	created uint64
	counted bool
	// dumps counts the stops of the world this lineage took.
	dumps atomic.Int64
}

// completeLocked reports whether no goroutine started since the last dump
// began, so every goroutine alive now was alive for it.
func (l *lineage) completeLocked() bool {
	created, counted := goroutinesCreated()
	return counted && l.counted && created == l.created
}

// refreshesPerCallback is how many dumps asking about unknown creators each
// running callback adds to what the run may take. A creator that started after
// the last dump is unknown, and the dump that would name it cannot if it
// already exited, so a fan-out of short-lived intermediate goroutines would
// pay one dump each; the allowance caps the run's cost at a handful of dumps
// per running callback, and a creator still unknown past it ends the chain.
// The dump that names a creator is taken before the callback it speaks for is
// known, so the callbacks running together hold the allowance together, not
// each its own: one callback's lookups use up the others' share, but never
// more than the running callbacks earned between them.
const refreshesPerCallback = 8

// refreshBudget is the dumps a run may still take for unknown creators (see
// refreshesPerCallback). Its zero value allows none.
type refreshBudget struct{ left atomic.Int64 }

// settle adds earned dumps to the budget, then trims it to the allowance of
// the running callbacks and builders: work that finished leaves nothing behind
// for the work still running to spend.
func (b *refreshBudget) settle(running int, earned int64) {
	limit := int64(running) * refreshesPerCallback
	for {
		left := b.left.Load()
		if b.left.CompareAndSwap(left, min(left+earned, limit)) {
			return
		}
	}
}

// take spends one dump and reports whether one was left. A nil budget has no
// limit.
func (b *refreshBudget) take() bool {
	if b == nil {
		return true
	}
	for {
		left := b.left.Load()
		if left <= 0 {
			return false
		}
		if b.left.CompareAndSwap(left, left-1) {
			return true
		}
	}
}

// chain lists from, then the goroutines that started it, nearest first, as
// far as the runtime named them: a creator that exited before anything saw it
// ends the chain, and so does one still unknown once budget has no dump left.
// A pooled worker is the last of its chain (see lineage.workers).
// Only a goroutine the lineage has never seen costs a dump, so every goroutine
// one creator started pays nothing once the creator is known. A nil budget has
// no limit.
func (l *lineage) chain(from GoroutineID, budget *refreshBudget) []GoroutineID {
	if from == 0 {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, known := l.creators[from]; !known {
		switch {
		case l.completeLocked():
			// Nothing started since the dump, so a goroutine it did not
			// name is gone.
			l.creators[from] = 0
		case budget.take():
			l.learnLocked()
			if _, seen := l.creators[from]; !seen {
				// Gone before the dump: ids are never reused, so nothing will
				// ever name its creator, and asking again need not dump again.
				l.creators[from] = 0
			}
		}
	}
	chain := []GoroutineID{from}
	for id := l.creators[from]; id != 0 && len(chain) <= len(l.creators) && !l.isWorkerLocked(chain[len(chain)-1]); id = l.creators[id] {
		chain = append(chain, id)
	}
	return chain
}

// isWorkerLocked reports whether the last dump that named id showed it to be a
// pooled worker.
func (l *lineage) isWorkerLocked(id GoroutineID) bool {
	_, ok := l.workers[id]
	return ok
}

// learnLocked replaces what is remembered with one dump of every goroutine
// alive now, keeping the exited goroutines a live one still names as its
// ancestor, so a chain through an exited creator survives while it is useful
// and the memory follows the live goroutines rather than every goroutine ever.
// A chain ends at a pooled worker, so nothing above one is ever read: a live
// worker's ancestors are not kept, and the walk up from any other goroutine
// stops at the first worker, which is kept so the chain still ends there.
func (l *lineage) learnLocked() {
	l.dumps.Add(1)
	l.created, l.counted = goroutinesCreated()
	live, workers := goroutineOrigins(allGoroutineTraces())
	for _, child := range slices.Collect(maps.Keys(live)) {
		if _, worker := workers[child]; worker {
			continue
		}
		for id := live[child]; id != 0; {
			if _, seen := live[id]; seen {
				break
			}
			older, remembered := l.creators[id]
			if !remembered {
				break
			}
			live[id] = older
			if l.isWorkerLocked(id) {
				workers[id] = struct{}{}
				break
			}
			id = older
		}
	}
	l.creators, l.workers = live, workers
	l.started = make(map[GoroutineID][]GoroutineID, len(live))
	for id, creator := range live {
		l.started[creator] = append(l.started[creator], id)
	}
}

// GoroutineDumps is how many stops of the world the process's lineage has
// taken, for tests in other packages that count them instead of timing.
func GoroutineDumps() int64 { return processLineage.dumps.Load() }
