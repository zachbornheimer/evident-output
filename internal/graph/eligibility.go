package graph

import "github.com/zachbornheimer/evident-output/internal/record"

// AddAfter declares predecessors: t starts only once every one of them
// succeeded, and never starts once one of them cannot. It reports false,
// changing nothing, when t's work was already submitted, since its
// configuration is frozen then.
func (g *Graph) AddAfter(t *Task, preds ...Predecessor) bool {
	g.lock()
	defer g.unlock()
	if t.sched.submitted() {
		return false
	}
	for _, p := range preds {
		closed := g.closeMembershipLocked(p)
		t.sched.preds = append(t.sched.preds, closed)
		t.after = append(t.after, closed)
	}
	return true
}

// AddContainerAfter declares what c starts after: its builder runs, or its
// children start, only once every predecessor succeeded. It reports false,
// changing nothing, once c has a builder or a member.
func (g *Graph) AddContainerAfter(c *Container, preds ...Predecessor) bool {
	g.lock()
	defer g.unlock()
	if c.builder != nil || len(c.tasks)+len(c.children) > 0 {
		return false
	}
	for _, p := range preds {
		c.entry = append(c.entry, g.closeMembershipLocked(p))
	}
	return true
}

// SealCollection takes c's membership as declared: a Wait asks for its
// outcome now, so a Task wired After it may start once the members declared
// so far succeed.
func (g *Graph) SealCollection(c *Container) {
	g.lock()
	defer g.unlock()
	g.sealCollectionLocked(c)
}

// closeMembershipLocked is p as an edge takes it: naming a populated
// Group or Sequence as a predecessor, by After or as the step before in a
// Sequence, means the Tasks declared into it so far, so the edge records
// the declaration cursor and a member declared later never gates it
// (E-093). An empty one stays open, so a Task wired After a Group before
// the loop that populates it waits for every child (E-028); a Wait, the
// drain, or a stall closes it instead.
func (g *Graph) closeMembershipLocked(p Predecessor) Predecessor {
	if p.col == nil || p.through != 0 || p.col.tally.total() == 0 || p.col.tally.holds > 0 {
		return p
	}
	p.through = g.declSeq
	g.sealCollectionLocked(p.col)
	return p
}

// sealCollectionLocked seals c's open edges through the current
// declaration cursor and re-places the Tasks parked on them; any still
// pending park again.
func (g *Graph) sealCollectionLocked(c *Container) {
	g.wakeLocked(c.tally.seal(g.declSeq))
}

// edgeCursorLocked is the declaration cursor edge p reads c's members
// through, or open when p reads a membership nothing has closed yet.
func (g *Graph) edgeCursorLocked(p Predecessor) (cursor int, open bool) {
	t := &p.col.tally
	switch {
	case t.holds > 0:
		return 0, true
	case p.through != 0:
		return p.through, false
	case t.sealed:
		return t.sealedThrough, false
	case g.sched.draining:
		return throughAll, false
	default:
		return 0, true
	}
}

// outcomeLocked is p's outcome and, while that is pending, what to park
// on until it changes: p itself, or for an empty collection that answers
// for its entry, the pending predecessor inside that entry.
func (g *Graph) outcomeLocked(p Predecessor) (predOutcome, Predecessor) {
	switch {
	case p.task != nil:
		return g.taskOutcomeLocked(p.task), p
	case p.col != nil:
		return g.collectionOutcomeLocked(p)
	default:
		return predFailed, p
	}
}

// taskOutcomeLocked is t's outcome for its dependents. Once the drain
// starts, a Task nobody Defined never will be, so it can no longer succeed.
func (g *Graph) taskOutcomeLocked(t *Task) predOutcome {
	out := stateOutcome(t.Rec.State())
	if out == predPending && g.sched.draining && t.neverDefinedLocked() {
		return predFailed
	}
	return out
}

// collectionOutcomeLocked is collection edge p's outcome for its
// dependents, and what to park on while it is pending. An open edge is
// still pending while its caller may populate the collection: a Task
// wired After a Group before the Group's children were declared must wait
// for every one of them, even when the children declared so far already
// finished. Once closed, the edge reads the members declared through its
// cursor; when there are none, the collection ran nothing, so it answers
// for what it starts after (see entryOutcomeLocked).
//
// A populated edge needs no such forwarding: every member starts after
// c's entry, so the members succeed only once the entry has.
func (g *Graph) collectionOutcomeLocked(p Predecessor) (predOutcome, Predecessor) {
	t := &p.col.tally
	if t.builderFailed {
		return predFailed, p
	}
	cursor, open := g.edgeCursorLocked(p)
	switch {
	case open && t.firstFailed != 0, !open && t.failedThrough(cursor):
		return predFailed, p
	case open:
		return predPending, p
	case !t.hasMemberThrough(cursor):
		return g.entryOutcomeLocked(p.col)
	case t.succeededThrough(cursor):
		return predSucceeded, p
	default:
		return predPending, p
	}
}

// entryOutcomeLocked folds c's entry, the one step c starts after in a
// Sequence (none elsewhere), with the first predecessor still pending.
// Holding only that step keeps a Sequence of n nested steps at n
// predecessors; a chain of empty steps forwards along the chain.
func (g *Graph) entryOutcomeLocked(c *Container) (predOutcome, Predecessor) {
	verdict, blocker := predSucceeded, Predecessor{}
	for _, p := range c.entry {
		out, b := g.outcomeLocked(p)
		if out == predFailed {
			return predFailed, b
		}
		if out == predPending && verdict == predSucceeded {
			verdict, blocker = predPending, b
		}
	}
	return verdict, blocker
}

// predScan says how far predsOutcomeLocked reads a Task's predecessors.
type predScan uint8

const (
	// scanToBlocker stops at the first predecessor still pending. The
	// Task parks on that one and reads again only once it settles, so a
	// predecessor further on that fails meanwhile is found then.
	scanToBlocker predScan = iota
	// scanAll reads every predecessor, so one that already failed settles
	// the Task NotStarted at once (§48).
	scanAll
)

// predsOutcomeLocked folds t's predecessors into one outcome and, while
// that outcome is pending, the first predecessor still pending.
//
// It forgets each Task predecessor it finds succeeded: a terminal state
// never reverts, and rereading it on every wake would make fan-in over n
// Tasks cost n² reads. A collection predecessor is kept, because a newly
// declared member can make it pending again.
func (g *Graph) predsOutcomeLocked(t *Task, scan predScan) (predOutcome, Predecessor) {
	verdict, blocker := predSucceeded, Predecessor{}
	preds := t.sched.preds
	kept := preds[:0]
	i := 0
	for ; i < len(preds); i++ {
		p := preds[i]
		g.sched.predChecks++
		out, parkOn := g.outcomeLocked(p)
		if out == predFailed {
			verdict, blocker = predFailed, p
			break
		}
		if out == predPending && verdict == predSucceeded {
			verdict, blocker = predPending, parkOn
		}
		if out != predSucceeded || p.task == nil {
			kept = append(kept, p)
		}
		if verdict == predPending && scan == scanToBlocker {
			i++
			break
		}
	}
	kept = append(kept, preds[i:]...)
	clear(preds[len(kept):])
	t.sched.preds = kept
	return verdict, blocker
}

func (g *Graph) eligibleLocked(t *Task) bool {
	verdict, _ := g.predsOutcomeLocked(t, scanToBlocker)
	return verdict == predSucceeded
}

// Eligible reports whether every predecessor of t succeeded.
func (g *Graph) Eligible(t *Task) bool {
	g.lock()
	defer g.unlock()
	return g.eligibleLocked(t)
}

// NextEligible is the earliest queued Task that may start now, or nil. An
// entry that lost its eligibility since it was queued (a collection it
// waits for gained a member) is placed again on the way.
func (g *Graph) NextEligible() *Task {
	g.lock()
	defer g.unlock()
	return g.nextEligibleLocked()
}

func (g *Graph) nextEligibleLocked() *Task {
	for {
		t := g.sched.queue.head()
		if t == nil || g.eligibleLocked(t) {
			return t
		}
		g.sched.queue.dropHead()
		g.placeLocked(t, scanToBlocker)
	}
}

// Claim marks t's callback as started: it is running now.
func (g *Graph) Claim(t *Task) {
	g.lock()
	defer g.unlock()
	g.enterPhaseLocked(t, PhaseRunning)
}

// Enqueue submits t's work. A gate arrives with its work already set. The
// Task is counted as outstanding work until it settles or finishes running
// (see WorkDone), and waits in the queue until Place decides where it
// belongs.
func (g *Graph) Enqueue(t *Task, work func() error) {
	g.lock()
	defer g.unlock()
	if work != nil {
		t.sched.work = work
	}
	g.sched.work.Add(1)
	g.enterPhaseLocked(t, PhaseQueued)
}

// Place routes submitted t by what its predecessors say: queued when all
// succeeded, parked on the first one still pending, and settled NotStarted
// when one can never succeed. It reads every predecessor, so one that
// already failed settles t at once.
func (g *Graph) Place(t *Task) {
	g.lock()
	defer g.unlock()
	g.placeLocked(t, scanAll)
}

// enterPhaseLocked moves t to phase, keeping the parked count true.
func (g *Graph) enterPhaseLocked(t *Task, phase Phase) {
	if t.sched.phase == PhaseParked {
		g.sched.parked--
	}
	if phase == PhaseParked {
		g.sched.parked++
	}
	t.sched.phase = phase
}

// placeLocked routes submitted t by what its predecessors say. scan says
// how far it reads them. t must not be in the queue.
func (g *Graph) placeLocked(t *Task, scan predScan) {
	verdict, blocker := g.predsOutcomeLocked(t, scan)
	switch verdict {
	case predSucceeded:
		g.enterPhaseLocked(t, PhaseQueued)
		g.sched.queue.push(t)
	case predPending:
		g.enterPhaseLocked(t, PhaseParked)
		if blocker.task != nil {
			blocker.task.sched.dependents = append(blocker.task.sched.dependents, t)
		} else {
			cursor, open := g.edgeCursorLocked(blocker)
			blocker.col.tally.park(t, cursor, open)
		}
	case predFailed:
		g.markNotStartedLocked(t)
	}
}

// wakeLocked re-places every parked Task in deps, now that what it was
// parked on stopped pending. A woken Task that settles NotStarted wakes
// its own dependents; the worklist keeps that cascade iterative however
// long the chain.
func (g *Graph) wakeLocked(deps []*Task) {
	g.sched.woken = append(g.sched.woken, deps...)
	if !g.sched.waking {
		g.drainWokenLocked()
	}
}

// holdWakesLocked defers every wake until the returned release runs, so a
// batch of settles is decided before any of them cascades.
func (g *Graph) holdWakesLocked() (release func()) {
	g.sched.waking = true
	return g.drainWokenLocked
}

func (g *Graph) drainWokenLocked() {
	g.sched.waking = true
	for i := 0; i < len(g.sched.woken); i++ {
		t := g.sched.woken[i]
		g.sched.woken[i] = nil
		if t.sched.phase == PhaseParked && !record.IsTerminalTask(t.Rec.State()) {
			g.placeLocked(t, scanToBlocker)
		}
	}
	g.sched.woken = g.sched.woken[:0]
	g.sched.waking = false
}

// propagateSettleLocked tells t's dependents and its collections that t
// settled.
func (g *Graph) propagateSettleLocked(t *Task) {
	deps := t.sched.dependents
	t.sched.dependents = nil
	for c := t.Parent; c != nil; c = c.Parent {
		deps = append(deps, c.tally.settle(t)...)
	}
	g.wakeLocked(deps)
}

// BeginDrain starts the drain: a Task nobody Defined and an empty
// collection stop pending, so every parked Task is placed again, once, now.
func (g *Graph) BeginDrain() {
	g.lock()
	defer g.unlock()
	g.sched.draining = true
	g.replaceParkedLocked()
}

// replaceParkedLocked re-places every parked Task. Draining calls it once:
// a Task nobody Defined and an empty collection stop pending then, without
// any settle to wake the Tasks parked on them.
func (g *Graph) replaceParkedLocked() {
	var parked []*Task
	for _, t := range g.taskList {
		t.sched.dependents = nil
		if t.sched.phase == PhaseParked {
			parked = append(parked, t)
		}
	}
	for _, gate := range g.gates {
		if gate.sched.phase == PhaseParked {
			parked = append(parked, gate)
		}
	}
	for _, col := range g.containers {
		col.tally.unpark()
	}
	for _, t := range parked {
		if t.sched.phase == PhaseParked && !record.IsTerminalTask(t.Rec.State()) {
			g.placeLocked(t, scanAll)
		}
	}
}

// Draining reports whether the drain started (see BeginDrain).
func (g *Graph) Draining() bool {
	g.lock()
	defer g.unlock()
	return g.sched.draining
}

// Parked is how many Tasks wait off the queue on a predecessor.
func (g *Graph) Parked() int {
	g.lock()
	defer g.unlock()
	return g.sched.parked
}

// Gates are the container builders' scheduler entities, which no Task list
// holds.
func (g *Graph) Gates() []*Task {
	g.lock()
	defer g.unlock()
	return append([]*Task(nil), g.gates...)
}

// PredecessorChecks counts the predecessor outcomes read, so a test can
// prove fan-in scheduling stays linear.
func (g *Graph) PredecessorChecks() int {
	g.lock()
	defer g.unlock()
	return g.sched.predChecks
}

// QueueVisits counts the queue entries examined, so a test can prove
// scheduling work stays linear without timing it.
func (g *Graph) QueueVisits() int {
	g.lock()
	defer g.unlock()
	return g.sched.queue.visits
}

// WorkDone counts one piece of work as finished.
func (g *Graph) WorkDone() { g.sched.work.Done() }

// WaitForWork blocks until all outstanding work settled or finished.
func (g *Graph) WaitForWork() { g.sched.work.Wait() }
