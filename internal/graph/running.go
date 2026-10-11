package graph

// goroutineOwnership says whose the goroutines are that a running callback's
// goroutine started: a goroutine that ran no work before the callback began
// (a pooled worker, started for it) can only have started them for it, but a
// plain caller that runs the callback inline already had goroutines running
// then, and the callback did not start those. Go names no start time, and
// goroutine ids are not issued in start order, so the scheduler lists the
// plain caller's goroutines alive when the callback began, and charges the
// callback with none of them, nor with any goroutine one of them started
// since: a wait or read from those is a plain caller's, never refused or
// released as the callback's.
type goroutineOwnership int

const (
	// ownsStarted: every goroutine the running goroutine starts, or started,
	// is the callback's.
	ownsStarted goroutineOwnership = iota
	// mayPredateClaim: the running goroutine started goroutines before the
	// callback began; only the ones it starts from now on are the callback's.
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
type goroutineLoad struct {
	callbacks, builders int
	// predating is the goroutines the running goroutine had started, directly
	// or through others, when it began its work (see mayPredateClaim); nil
	// when it had none to set apart.
	predating map[GoroutineID]struct{}
}

func (l goroutineLoad) total() int { return l.callbacks + l.builders }

// runsAny reports whether the goroutine runs a task callback or builder.
func (l goroutineLoad) runsAny() bool { return l.total() > 0 }

// owns reports whether the work this load runs is the one that started
// goroutine child, which the goroutine running it started or whose creator
// chain passes through a goroutine it started.
func (l goroutineLoad) owns(child GoroutineID) bool {
	_, predates := l.predating[child]
	return l.runsAny() && !predates
}

// startingGoroutine is the nearest goroutine running a task callback or
// builder that started self, through any number of goroutines in between,
// and zero when none did. creator is the goroutine that started self. A
// goroutine on the way that started its child before its own work began does
// not count: the work did not start that child. Walking starts from creator,
// and stops the world only when creator is one nothing has asked about yet
// (see lineage), so every goroutine one creator starts shares a single dump,
// and a callback's goroutines cost a bounded number of dumps in all (see
// refreshesPerCallback); with no goroutine running anything it reads nothing.
func (g *Graph) startingGoroutine(self, creator GoroutineID) GoroutineID {
	if creator == 0 {
		return 0
	}
	g.lockRead()
	direct, working := g.exec.runningGoroutines[creator].owns(self), len(g.exec.runningGoroutines) > 0
	g.unlockRead()
	if direct {
		return creator
	}
	if !working {
		return 0
	}
	chain := processLineage.chain(creator, &g.exec.refreshes)
	g.lockRead()
	defer g.unlockRead()
	child := self
	for _, id := range chain {
		if g.exec.runningGoroutines[id].owns(child) {
			return id
		}
		child = id
	}
	return 0
}

// trackRunning adds added to the calling goroutine's load, and returns the
// func that takes it back out. A goroutine that may have started goroutines
// before its work began (see goroutineOwnership) is listed with the ones it
// had alive then, which takes one dump of every goroutine, so the work is
// not charged with them; the dump is taken before any lock.
func (g *Graph) trackRunning(added goroutineLoad, own goroutineOwnership) (done func()) {
	id := CurrentGoroutine()
	if own == mayPredateClaim {
		added.predating = goroutinesStartedBy(id)
	}
	g.exec.refreshes.refill()
	g.lock()
	g.addLoadLocked(id, added)
	g.unlock()
	return func() {
		g.lock()
		defer g.unlock()
		g.removeLoadLocked(id, added)
	}
}

// addLoadLocked lists added as running on goroutine id. The goroutines set
// apart (see mayPredateClaim) are those of the work that began first: work
// nested inside it, on the same goroutine, adds none.
func (g *Graph) addLoadLocked(id GoroutineID, added goroutineLoad) {
	if g.exec.runningGoroutines == nil {
		g.exec.runningGoroutines = make(map[GoroutineID]goroutineLoad)
	}
	load := g.exec.runningGoroutines[id]
	if !load.runsAny() {
		load.predating = added.predating
	}
	load.callbacks += added.callbacks
	load.builders += added.builders
	g.exec.buildersRunning += added.builders
	g.exec.runningGoroutines[id] = load
}

// removeLoadLocked takes removed back out of what runs on goroutine id.
func (g *Graph) removeLoadLocked(id GoroutineID, removed goroutineLoad) {
	load := g.exec.runningGoroutines[id]
	load.callbacks -= removed.callbacks
	load.builders -= removed.builders
	g.exec.buildersRunning -= removed.builders
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
