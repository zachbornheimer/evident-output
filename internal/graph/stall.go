package graph

// resolveStallLocked ends a stall: nothing in the run can move on its own while the
// drain is running. Left alone, the drain would hang the run forever, so each
// step below turns what is stuck into a stated outcome. It reports whether it
// changed anything, so Kick schedules again before trying the next step, and
// the misuse it found, for the caller to tell the MisuseSink once the graph
// lock is free.
//
// The steps run in the order that leaves every row most truthful:
//
//  1. a Task in an After cycle settles Blocked, naming the cycle;
//  2. any Task still parked settles NotStarted.
func (g *Graph) resolveStallLocked() (moved bool, notes []misuseNote) {
	defer func() { notes = g.takeMisuseNotesLocked() }()
	if !g.sched.draining || g.progressPossibleLocked() {
		return false, nil
	}
	if g.blockCyclesLocked() {
		return true, nil
	}
	return g.abandonStrandedLocked(), nil
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
// callback is doing work, or something can start.
func (g *Graph) progressPossibleLocked() bool {
	return g.exec.executing.Load() > 0 || g.anyStartableLocked()
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
