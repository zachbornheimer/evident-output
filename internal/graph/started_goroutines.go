package graph

// startedGoroutines is the goroutines one goroutine had started, directly or
// through others, as of a moment, with the process's goroutine count at that
// moment: while the count has not moved, the goroutine has started no more.
type startedGoroutines struct {
	ids     map[GoroutineID]struct{}
	created uint64
	counted bool
}

// noneStarted is the goroutines of a goroutine that has just begun: none, as
// of now.
func noneStarted() startedGoroutines {
	created, counted := goroutinesCreated()
	return startedGoroutines{created: created, counted: counted}
}

// current reports whether no goroutine has started in the process since s was
// taken, so s still names everything its goroutine has started.
func (s startedGoroutines) current() bool {
	created, counted := goroutinesCreated()
	return s.counted && counted && created == s.created
}

// startedBeforeClaim is the goroutines the calling goroutine had started when
// work of its own began (see goroutineOwnership). Listing them stops the
// world, so it is called before any lock is taken, and is skipped whenever the
// answer is already known: a callback that lends its slot to one nested Task
// after another knows what it had started when the last one ended, and starts
// nothing while it waits for the next to begin.
func (g *Graph) startedBeforeClaim(own goroutineOwnership) startedGoroutines {
	if own != mayPredateClaim {
		return noneStarted()
	}
	id := CurrentGoroutine()
	if known, ok := g.knownStartedBy(id); ok && known.current() {
		return known
	}
	return goroutinesStartedBy(id)
}
