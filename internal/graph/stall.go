package graph

// resolveStall ends a stall: nothing in the run can move on its own, yet a
// wait is parked or the drain is running. Left alone, either would hang the
// run forever, so each step below turns what is stuck into a stated outcome.
// It reports whether it changed anything, so Kick schedules again before
// trying the next step.
//
// The steps run in the order that leaves every row most truthful:
//
//  1. what a parked wait still waits for and nothing will now supply (a Task
//     nobody Defined, an empty collection) is sealed;
//  2. a Task in an After cycle settles Blocked, naming the cycle;
//  3. a parked wait is released with ErrWaitDeadlock;
//  4. once draining, any Task still parked settles NotStarted.
func (g *Graph) resolveStall() bool {
	g.lock()
	defer g.unlock()
	draining := g.sched.draining
	if len(g.exec.waits) == 0 && !draining {
		return false
	}
	if g.progressPossibleLocked() {
		return false
	}
	if !draining && g.sealWaitedInputsLocked() {
		return true
	}
	if g.blockCyclesLocked() {
		return true
	}
	if len(g.exec.waits) > 0 {
		g.releaseWaitsLocked()
		return true
	}
	return draining && g.abandonStrandedLocked()
}

// sealWaitedInputsLocked seals what every parked wait waits for, once the run
// proved it cannot move. It reports whether it sealed anything.
func (g *Graph) sealWaitedInputsLocked() bool {
	awaited := make([]*Task, 0, len(g.exec.waits))
	for w := range g.exec.waits {
		awaited = append(awaited, w.task)
	}
	return g.sealWaitedLocked(awaited)
}

// releaseWaitsLocked ends the parked waits nothing left in the run can
// satisfy. Each waiter is told the truth, ErrWaitDeadlock naming the Task it
// awaited, and its callback returns that error, putting the cycle on its own
// row.
//
// A waiter a live callback started is released first, and alone: the stall
// only assumed that callback is blocked on it, and once it returns the
// callback may finish and satisfy every other wait.
func (g *Graph) releaseWaitsLocked() {
	release := g.spawnedWaitsLocked()
	if len(release) == 0 {
		for w := range g.exec.waits {
			release = append(release, w)
		}
	}
	for _, w := range release {
		w.released = g.unreachableWait(w)
		g.misuse.RecordMisuseFor(w.task.Name, ErrWaitDeadlock)
	}
	for _, w := range release {
		delete(g.exec.waits, w)
		close(w.abort)
	}
}

// spawnedWaitsLocked lists the parked waiters a live callback started.
func (g *Graph) spawnedWaitsLocked() []*WaitTicket {
	var spawned []*WaitTicket
	for w := range g.exec.waits {
		if g.startedByCallbackLocked(w) {
			spawned = append(spawned, w)
		}
	}
	return spawned
}

// abandonStrandedLocked settles NotStarted every Task still parked once the
// drain can move no further: what it waits for will never settle (a
// collection whose member nobody Defined). It reports whether any was.
func (g *Graph) abandonStrandedLocked() bool {
	if g.sched.parked == 0 {
		return false
	}
	for _, t := range g.taskList {
		if t.sched.phase == PhaseParked && !terminal(t) {
			g.markNotStartedLocked(t)
		}
	}
	for _, gate := range g.gates {
		if gate.sched.phase == PhaseParked && !terminal(gate) {
			g.markNotStartedLocked(gate)
		}
	}
	return true
}

// progressPossibleLocked reports whether anything could still move the run: a
// callback is doing work, something can start, or a wait is about to wake.
func (g *Graph) progressPossibleLocked() bool {
	return g.exec.executing > g.heldCallbacksLocked() ||
		g.anyStartableLocked() ||
		g.anyAwaitedTaskSettledLocked()
}

// heldCallbacksLocked counts the callbacks a parked waiter may be holding
// still: the ones on its own stack, and those of a callback goroutine that
// started it — the errgroup shape, where that callback blocks on the
// goroutine it started. More callbacks executing than held means one of them
// is still doing work.
func (g *Graph) heldCallbacksLocked() int {
	held := 0
	parked := make(map[GoroutineID]struct{}, len(g.exec.waits))
	for w := range g.exec.waits {
		held += w.depth
		parked[w.self] = struct{}{}
	}
	creators := make(map[GoroutineID]struct{})
	for w := range g.exec.waits {
		if _, counted := parked[w.creator]; !counted && g.startedByCallbackLocked(w) {
			creators[w.creator] = struct{}{}
		}
	}
	for creator := range creators {
		held += g.exec.callbackGoroutines[creator]
	}
	return held
}

// startedByCallbackLocked reports whether w's goroutine was started by a
// goroutine that is running a task callback right now. A parked waiter's own
// callbacks are already counted by its depth, so only a plain waiter can be
// one.
func (g *Graph) startedByCallbackLocked(w *WaitTicket) bool {
	return w.depth == 0 && w.creator != 0 && g.exec.callbackGoroutines[w.creator] > 0
}

// anyStartableLocked reports whether the pool could start queued work now:
// something is eligible and a slot is free. Eligible work behind a full pool
// moves only when an executing callback finishes, which the executing count
// already answers.
func (g *Graph) anyStartableLocked() bool {
	return !g.exec.cancelled &&
		g.exec.inflight < g.ceilingLocked() &&
		g.nextEligibleLocked() != nil
}

// anyAwaitedTaskSettledLocked reports whether some parked waiter's Task is
// already terminal: it is about to wake on its own done channel.
func (g *Graph) anyAwaitedTaskSettledLocked() bool {
	for w := range g.exec.waits {
		if terminal(w.task) {
			return true
		}
	}
	return false
}
