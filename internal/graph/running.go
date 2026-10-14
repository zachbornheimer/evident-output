package graph

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
// builder that started creator's child, through any number of goroutines in
// between, and the work it runs that did; the zero starter when none did.
// Walking starts from creator, and stops the world only when creator
// is one nothing has asked about yet (see lineage), so every goroutine one
// creator starts shares a single dump, and the running callbacks' goroutines
// cost a bounded number of dumps in all (see refreshesPerCallback); with no
// goroutine running anything it reads nothing.
func (g *Graph) startingGoroutine(creator GoroutineID) starter {
	if creator == 0 {
		return starter{}
	}
	g.lockRead()
	direct, working := g.frameStartedLocked(creator), len(g.exec.runningGoroutines) > 0
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
	for _, id := range chain {
		if task := g.frameStartedLocked(id); task != nil {
			return starter{goroutine: id, task: task}
		}
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
