package graph

// resolveStall ends a stall: nothing in the run can move on its own, yet a
// wait is parked or the drain is running. Left alone, either would hang the
// run forever, so each step below turns what is stuck into a stated outcome.
// It reports whether it changed anything, so Kick schedules again before
// trying the next step, and the misuse it found, for the caller to tell the
// MisuseSink once the graph lock is free.
//
// The steps run in the order that leaves every row most truthful:
//
//  1. what a parked wait still waits for and nothing will now supply (a Task
//     nobody Defined, an empty collection) is sealed;
//  2. a Task in an After cycle settles Blocked, naming the cycle;
//  3. a parked wait is released with ErrWaitDeadlock;
//  4. once draining, any Task still parked settles NotStarted.
func (g *Graph) resolveStall() (moved bool, notes []misuseNote) {
	g.lock()
	defer g.unlock()
	defer func() { notes = g.takeMisuseNotesLocked() }()
	draining := g.sched.draining
	if len(g.exec.waits) == 0 && !draining {
		return false, nil
	}
	if g.progressPossibleLocked() {
		return false, nil
	}
	if !draining && g.sealWaitedInputsLocked() {
		return true, nil
	}
	if g.blockCyclesLocked() {
		return true, nil
	}
	if len(g.exec.waits) > 0 {
		g.releaseWaitsLocked()
		return true, nil
	}
	return draining && g.abandonStrandedLocked(), nil
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
// satisfy, in the order that blames the fewest:
//
//  1. A wait on a Task whose row already settled is released alone, with no
//     error and no misuse, when its own Task is awaited by another parked
//     wait: the row states the answer, and only the callback's own error is
//     still on its way out. That callback may be what the other wait is
//     blocked on, so the stall may not be a deadlock once this one returns.
//     Any other wait on a settled Task stays parked: the callback's error is
//     owed to it, and releasing it early would answer with the row instead.
//  2. Failing that, a waiter a live callback started is released alone: the
//     stall only assumed that callback is blocked on it, and once it returns
//     the callback may finish and satisfy every other wait.
//  3. Failing that, every wait on a Task still running is told the truth,
//     ErrWaitDeadlock naming the Task it awaited, and its callback returns
//     that error, putting the cycle on its own row.
//
// Waits on settled Tasks that remain wake when those callbacks return. When
// every parked wait is on a settled Task and none can relieve another, they
// are all released with the row's answer rather than left to hang.
func (g *Graph) releaseWaitsLocked() {
	if relieving := g.waitsWhereLocked(g.relievesAnOwingCallbackLocked()); len(relieving) > 0 {
		g.wakeWaitsLocked(relieving)
		return
	}
	running := g.waitsWhereLocked(func(w *WaitTicket) bool { return !terminal(w.task) })
	if len(running) == 0 {
		g.wakeWaitsLocked(g.waitsWhereLocked(func(*WaitTicket) bool { return true }))
		return
	}
	release := keepWaits(running, g.startedByCallbackLocked)
	if len(release) == 0 {
		release = running
	}
	for _, w := range release {
		w.released = g.unreachableWait(w)
		g.noteMisuseLocked(w.task.Name, ErrWaitDeadlock)
	}
	g.wakeWaitsLocked(release)
}

// wakeWaitsLocked unparks every waiter in release.
func (g *Graph) wakeWaitsLocked(release []*WaitTicket) {
	for _, w := range release {
		delete(g.exec.waits, w)
		close(w.abort)
	}
}

// waitsWhereLocked lists the parked waiters that satisfy keep.
func (g *Graph) waitsWhereLocked(keep func(*WaitTicket) bool) []*WaitTicket {
	var kept []*WaitTicket
	for w := range g.exec.waits {
		if keep(w) {
			kept = append(kept, w)
		}
	}
	return kept
}

// keepWaits lists the waiters in waits that satisfy keep.
func keepWaits(waits []*WaitTicket, keep func(*WaitTicket) bool) []*WaitTicket {
	var kept []*WaitTicket
	for _, w := range waits {
		if keep(w) {
			kept = append(kept, w)
		}
	}
	return kept
}

// relievesAnOwingCallbackLocked is the test for a wait on a settled Task
// whose own Task is awaited by another parked wait: returning it may let the
// callback that wait is blocked on finish.
func (g *Graph) relievesAnOwingCallbackLocked() func(*WaitTicket) bool {
	awaited := make(map[*Task]int, len(g.exec.waits))
	for w := range g.exec.waits {
		awaited[w.task]++
	}
	return func(w *WaitTicket) bool {
		if !terminal(w.task) || w.owner == nil {
			return false
		}
		others := awaited[w.owner]
		if w.task == w.owner {
			others--
		}
		return others > 0
	}
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
		held += g.exec.runningGoroutines[creator].total()
	}
	return held
}

// startedByCallbackLocked reports whether w's goroutine was started by a
// goroutine that is running a task callback or container builder right now. A
// parked waiter's own callbacks are already counted by its depth, so only a
// plain waiter can be one.
func (g *Graph) startedByCallbackLocked(w *WaitTicket) bool {
	return w.depth == 0 && w.creator != 0 && g.exec.runningGoroutines[w.creator].total() > 0
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
// already terminal and owes no callback error still on its way out: it is
// about to wake on its own channels.
func (g *Graph) anyAwaitedTaskSettledLocked() bool {
	for w := range g.exec.waits {
		if terminal(w.task) && !w.task.errorStillOwedLocked() {
			return true
		}
	}
	return false
}
