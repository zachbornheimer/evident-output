package graph

import "runtime"

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

// afterNested is what the goroutine knows it has started once the work that
// began with s ended, where liveAtEntry is how many goroutines the process
// had running when it began. The work's own goroutines are not in s, so s
// names everything only if the work left none running. A goroutine that
// started and exited while the work ran is no loss, and a count of running
// goroutines that did not rise shows none was left, without stopping the
// world. A count that rose may hide one left running, so the result is then
// stamped with the count s was taken at: the next claim sees the process has
// started goroutines since and lists them afresh. Other goroutines ending in
// the same moment can hide a left one from the count (see CONTRACT.md,
// section 2).
func (s startedGoroutines) afterNested(liveAtEntry int) startedGoroutines {
	ended := startedGoroutines{ids: s.ids}
	ended.created, ended.counted = goroutinesCreated()
	if ended.created != s.created && runtime.NumGoroutine() > liveAtEntry {
		ended.created = s.created
	}
	return ended
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
