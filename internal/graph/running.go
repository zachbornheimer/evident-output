package graph

// goroutineOwnership says whose the goroutines are that a running callback's
// goroutine started: a goroutine that ran no work before the callback began
// (a pooled worker, started for it) can only have started them for it, but a
// plain caller that runs the callback inline already had goroutines running
// then, and the callback did not start those. Go names no start time, so the
// scheduler cannot tell which of a plain caller's goroutines came after the
// claim began, and charges none of them to it: a wait or read from one of
// them is a plain caller's, never refused or released as the callback's.
type goroutineOwnership int

const (
	// ownsStarted: every goroutine the running goroutine starts, or started,
	// is the callback's.
	ownsStarted goroutineOwnership = iota
	// mayPredateClaim: the running goroutine started goroutines before the
	// callback began, so none is charged to it.
	mayPredateClaim
)

// runTrackedCallback runs fn while the scheduler knows which goroutine is
// running it, so a goroutine fn starts and then parks in a wait can be traced
// back to the callback it may be holding still.
func (g *Graph) runTrackedCallback(fn func() error, own goroutineOwnership) error {
	defer g.trackRunning(goroutineLoad{callbacks: 1}, own)()
	return runCallback(fn)
}

// runTrackedBuilder is runBuilder while the scheduler knows which goroutine is
// running the builder, as it does for a callback, so a goroutine the builder
// starts is traced back to it.
func (g *Graph) runTrackedBuilder(work func() error, own goroutineOwnership) error {
	defer g.trackRunning(goroutineLoad{builders: 1}, own)()
	return runBuilder(work)
}

// goroutineLoad is what one goroutine is running right now: task callbacks
// and container builders.
type goroutineLoad struct{ callbacks, builders int }

func (l goroutineLoad) total() int { return l.callbacks + l.builders }

// runsAny reports whether the goroutine runs a task callback or builder.
func (l goroutineLoad) runsAny() bool { return l.total() > 0 }

// startingGoroutine is the nearest goroutine running a task callback or
// builder that started the calling goroutine, through any number of
// goroutines in between: creator itself when it qualifies, else the nearest
// ancestor that does. With none, or no goroutine running anything, it is
// creator, so the common case reads no more than the direct creator. The
// ancestor walk may stop the world, once per goroutine (see lineage); it is
// reached only by a goroutine another goroutine started while work runs.
func (g *Graph) startingGoroutine(creator GoroutineID) GoroutineID {
	if creator == 0 {
		return 0
	}
	g.lockRead()
	direct, working := g.exec.runningGoroutines[creator].runsAny(), len(g.exec.runningGoroutines) > 0
	g.unlockRead()
	if direct || !working {
		return creator
	}
	ancestors := GoroutineAncestors()
	g.lockRead()
	defer g.unlockRead()
	for _, id := range ancestors {
		if g.exec.runningGoroutines[id].runsAny() {
			return id
		}
	}
	return creator
}

// trackRunning adds added to the calling goroutine's load, and returns the
// func that takes it back out. A builder is always counted in buildersRunning,
// but a goroutine that may have started goroutines before its work began (see
// goroutineOwnership) is not listed, so nothing is charged to it.
func (g *Graph) trackRunning(added goroutineLoad, own goroutineOwnership) (done func()) {
	id := CurrentGoroutine()
	listed := own == ownsStarted
	g.lock()
	g.exec.buildersRunning += added.builders
	if listed {
		g.addLoadLocked(id, added)
	}
	g.unlock()
	return func() {
		g.lock()
		defer g.unlock()
		g.exec.buildersRunning -= added.builders
		if listed {
			g.removeLoadLocked(id, added)
		}
	}
}

// addLoadLocked lists added as running on goroutine id.
func (g *Graph) addLoadLocked(id GoroutineID, added goroutineLoad) {
	if g.exec.runningGoroutines == nil {
		g.exec.runningGoroutines = make(map[GoroutineID]goroutineLoad)
	}
	load := g.exec.runningGoroutines[id]
	load.callbacks += added.callbacks
	load.builders += added.builders
	g.exec.runningGoroutines[id] = load
}

// removeLoadLocked takes removed back out of what runs on goroutine id.
func (g *Graph) removeLoadLocked(id GoroutineID, removed goroutineLoad) {
	load := g.exec.runningGoroutines[id]
	load.callbacks -= removed.callbacks
	load.builders -= removed.builders
	if load.total() <= 0 {
		delete(g.exec.runningGoroutines, id)
		return
	}
	g.exec.runningGoroutines[id] = load
}

// anyBuilderRunning reports whether any goroutine is running a container
// builder right now.
func (g *Graph) anyBuilderRunning() bool {
	g.lockRead()
	defer g.unlockRead()
	return g.exec.buildersRunning > 0
}

// isRunningBuilder reports whether goroutine id is listed as running a
// container builder.
func (g *Graph) isRunningBuilder(id GoroutineID) bool {
	g.lockRead()
	defer g.unlockRead()
	return g.exec.runningGoroutines[id].builders > 0
}
