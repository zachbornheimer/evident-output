package graph

// goroutineOwnership says whose the goroutines are that a running callback's
// goroutine started: a goroutine that ran no work before the callback began
// (a pooled worker, started for it) can only have started them for it, but a
// waiter that runs the callback inline already had goroutines running then
// (a plain caller's own, or the goroutines of the callback that lends its
// slot), and the inline callback did not start those. Go names no start time,
// and goroutine ids are not issued in start order, so the scheduler lists the
// waiter's goroutines alive when the callback began, and charges the callback
// with none of them, nor with any goroutine one of them started since: a wait
// or read from those belongs to the work underneath, never refused or
// released as the inline callback's.
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
func (g *Graph) runTrackedCallback(fn func() error) error {
	defer g.trackRunning(goroutineLoad{callbacks: 1})()
	return runCallback(fn)
}

// runTrackedBuilder is runBuilder while the scheduler knows which goroutine is
// running the builder, as it does for a callback, so a goroutine the builder
// starts is traced back to it.
func (g *Graph) runTrackedBuilder(work func() error) error {
	defer g.trackRunning(goroutineLoad{builders: 1})()
	return runBuilder(work)
}

// goroutineLoad is what one goroutine is running right now: task callbacks
// and container builders.
type goroutineLoad struct {
	callbacks, builders int
}

func (l goroutineLoad) total() int { return l.callbacks + l.builders }

// runsAny reports whether the goroutine runs a task callback or builder.
func (l goroutineLoad) runsAny() bool { return l.total() > 0 }

// starter is the work that started a goroutine: the goroutine running it, and
// the Task or builder gate it is running for. The zero starter is none.
type starter struct {
	goroutine GoroutineID
	task      *Task
}

// startingGoroutine is the nearest goroutine running a task callback or
// builder that started self, through any number of goroutines in between,
// and the work it runs that did; the zero starter when none did. creator is
// the goroutine that started self. A goroutine on the way that started its
// child before its own work began does not count: the work did not start that
// child. Walking starts from creator, and stops the world only when creator
// is one nothing has asked about yet (see lineage), so every goroutine one
// creator starts shares a single dump, and the running callbacks' goroutines
// cost a bounded number of dumps in all (see refreshesPerCallback); with no
// goroutine running anything it reads nothing.
func (g *Graph) startingGoroutine(self, creator GoroutineID) starter {
	if creator == 0 {
		return starter{}
	}
	g.lockRead()
	direct, working := g.frameStartedLocked(creator, self), len(g.exec.runningGoroutines) > 0
	g.unlockRead()
	if direct != nil {
		return starter{goroutine: creator, task: direct}
	}
	if !working {
		return starter{}
	}
	chain := processLineage.chain(creator, &g.exec.refreshes)
	g.lockRead()
	defer g.unlockRead()
	child := self
	for _, id := range chain {
		if task := g.frameStartedLocked(id, child); task != nil {
			return starter{goroutine: id, task: task}
		}
		child = id
	}
	return starter{}
}

// trackRunning adds added to the calling goroutine's load, and returns the
// func that takes it back out. Work beginning also earns the run the dumps
// asking about unknown creators may cost, and work ending gives back the share
// it held (see refreshesPerCallback).
func (g *Graph) trackRunning(added goroutineLoad) (done func()) {
	id := CurrentGoroutine()
	g.lock()
	g.addLoadLocked(id, added)
	g.exec.refreshes.settle(g.exec.loaded, int64(added.total())*refreshesPerCallback)
	g.unlock()
	return func() {
		g.lock()
		defer g.unlock()
		g.removeLoadLocked(id, added)
		g.exec.refreshes.settle(g.exec.loaded, 0)
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
	g.exec.buildersRunning += added.builders
	g.exec.loaded += added.total()
	g.exec.runningGoroutines[id] = load
}

// removeLoadLocked takes removed back out of what runs on goroutine id.
func (g *Graph) removeLoadLocked(id GoroutineID, removed goroutineLoad) {
	load := g.exec.runningGoroutines[id]
	load.callbacks -= removed.callbacks
	load.builders -= removed.builders
	g.exec.buildersRunning -= removed.builders
	g.exec.loaded -= removed.total()
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
