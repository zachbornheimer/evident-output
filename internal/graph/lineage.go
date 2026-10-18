package graph

import (
	"maps"
	"slices"
	"sync"
	"sync/atomic"
)

// processLineage covers every goroutine in the process: who spawned a
// goroutine is a fact about the process, not about one Graph.
var processLineage lineage

// lineage remembers which goroutine spawned each goroutine it has seen. A
// goroutine's creator is fixed when it starts and ids are never reused, so a
// remembered creator never goes stale. Reading creators means a traceback of
// every goroutine, which stops the world: one dump teaches the creator of
// every goroutine alive then, so a creator pays for at most one, and the
// goroutines it spawned, asking about it afterwards, pay for none. While the
// process starts no goroutine (see goroutinesCreated) the last dump is still
// the whole truth, and a question about a goroutine it did not name, or about
// who spawned which, is answered without another.
type lineage struct {
	mu       sync.Mutex
	creators map[GoroutineID]GoroutineID
	// workers is the goroutines among creators that serve pooled claims. A
	// chain ends at one: the goroutine that spawned a worker never authored
	// the work the worker runs, so what that creator is running says nothing
	// about a goroutine the worker's work spawned, whether the worker is still
	// there or has gone. The dump names them, so they are kept exactly as long
	// as creators keeps them.
	workers map[GoroutineID]struct{}
	// created is how many goroutines the process had spawned when the last
	// dump began; counted is false where the toolchain keeps no such count.
	created uint64
	counted bool
	// dumps counts the stops of the world this lineage took.
	dumps atomic.Int64
}

// completeLocked reports whether no goroutine spawned since the last dump
// began, so every goroutine alive now was alive for it.
func (l *lineage) completeLocked() bool {
	created, counted := goroutinesCreated()
	return counted && l.counted && created == l.created
}

// refreshesPerCallback is how many dumps asking about unknown creators each
// running callback adds to what the run may take. A creator that spawned after
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

// chain lists from, then the goroutines that spawned it, nearest first, as
// far as the runtime named them: a creator that exited before anything saw it
// ends the chain, and so does one still unknown once budget has no dump left.
// A pooled worker is the last of its chain (see lineage.workers).
// Only a goroutine the lineage has never seen costs a dump, so every goroutine
// one creator spawned pays nothing once the creator is known. A nil budget has
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
			// Nothing spawned since the dump, so a goroutine it did not
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
// alive now. A goroutine whose creator has exited is pointed at its nearest
// ancestor that is alive or a pooled worker and the exited generations between
// are dropped: an exited goroutine carries no frame, so it never changes whom
// a chain attributes to, and a relay that starts its successor and exits would
// otherwise leave one remembered ancestor per generation. A chain ends at a
// pooled worker, so nothing above one is ever read: a live worker's ancestors
// are not kept, and the walk up from any other goroutine stops at the first
// worker, which is kept so the chain still ends there.
func (l *lineage) learnLocked() {
	l.dumps.Add(1)
	l.created, l.counted = goroutinesCreated()
	live, workers := goroutineOrigins(allGoroutineTraces())
	for _, child := range slices.Collect(maps.Keys(live)) {
		if _, worker := workers[child]; worker {
			continue
		}
		live[child] = l.nearestKeptAncestorLocked(child, live[child], live, workers)
	}
	l.creators, l.workers = live, workers
}

// nearestKeptAncestorLocked is the nearest ancestor of child that is alive in
// live or is a pooled worker, 0 when the walk reaches a goroutine nothing
// remembers first. creator is who started child. A pooled worker found on the
// way is added to workers and live, so the chain still ends there. The walk
// starts from the shortcut an earlier dump left for child when creator has
// since exited, and every hop lands on a goroutine an earlier dump saw alive,
// so the walk is as long as the generations since the last dump, not since the
// run began.
func (l *lineage) nearestKeptAncestorLocked(child, creator GoroutineID, live map[GoroutineID]GoroutineID, workers map[GoroutineID]struct{}) GoroutineID {
	id := creator
	if _, alive := live[id]; !alive {
		if shortcut, remembered := l.creators[child]; remembered {
			id = shortcut
		}
	}
	for id != 0 {
		if _, alive := live[id]; alive {
			return id
		}
		if l.isWorkerLocked(id) {
			workers[id] = struct{}{}
			live[id] = 0
			return id
		}
		older, remembered := l.creators[id]
		if !remembered {
			return 0
		}
		id = older
	}
	return 0
}

// GoroutineDumps is how many stops of the world the process's lineage has
// taken, for tests in other packages that count them instead of timing.
func GoroutineDumps() int64 { return processLineage.dumps.Load() }
