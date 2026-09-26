package engine

import (
	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/engine/schedule"
)

// predecessor is one thing a Task waits for: another Task, or a whole
// Group/Sequence. Both nil is a handle this run never declared, which can
// never succeed.
type predecessor struct {
	task *taskState
	col  *tasksState
	// through is, for a collection named while populated, the declaration
	// cursor at that moment: the edge reads only the members declared
	// through it. 0 for one named while empty (see edgeCursorLocked).
	through int
}

// After declares predecessors: this Task starts only once every one of
// them succeeded, and never starts once one of them cannot.
func (t *TaskHandle) After(preds ...any) *TaskHandle {
	if t == nil || t.out == nil {
		return t
	}
	o := t.out
	o.mu.Lock()
	defer o.mu.Unlock()
	st := o.taskByRef[t.id]
	if st == nil {
		return t
	}
	if st.sched.submitted() {
		o.recordMisuse(ErrInvalidConfig)
		return t
	}
	for _, p := range preds {
		if pred, ok := o.predecessorOfLocked(p); ok {
			st.sched.preds = append(st.sched.preds, o.closeMembershipLocked(pred))
		}
	}
	return t
}

// closeMembershipLocked is p as an edge takes it: naming a populated
// Group or Sequence as a predecessor, by After or as the step before in a
// Sequence, means the Tasks declared into it so far, so the edge records
// the declaration cursor and a member declared later never gates it
// (E-093). An empty one stays open, so a Task wired After a Group before
// the loop that populates it waits for every child (E-028); a Wait, the
// drain, or a stall closes it instead.
func (o *Output) closeMembershipLocked(p predecessor) predecessor {
	if p.col == nil || p.through != 0 || p.col.tally.Len() == 0 {
		return p
	}
	p.through = o.declSeq
	o.sealCollectionLocked(p.col)
	return p
}

// sealCollectionLocked seals c's open edges through the current
// declaration cursor and re-places the Tasks parked on them; any still
// pending park again.
func (o *Output) sealCollectionLocked(c *tasksState) {
	o.wakeLocked(c.tally.Seal(o.declSeq))
}

// edgeCursorLocked is the declaration cursor edge p reads c's members
// through, or open when p reads a membership nothing has closed yet.
func (o *Output) edgeCursorLocked(p predecessor) (cursor int, open bool) {
	t := &p.col.tally
	sealedThrough, sealed := t.SealedThrough()
	switch {
	case p.through != 0:
		return p.through, false
	case sealed:
		return sealedThrough, false
	case o.sched.draining:
		return schedule.ThroughAll, false
	default:
		return 0, true
	}
}

// predecessorOfLocked resolves one After argument. ok is false for a nil
// handle, which After ignores. A handle this Output never declared yields
// the empty predecessor, which never succeeds.
func (o *Output) predecessorOfLocked(p any) (pred predecessor, ok bool) {
	switch x := p.(type) {
	case *TaskHandle:
		if x == nil || x.id == "" {
			return predecessor{}, false
		}
		if x.out == o {
			pred.task = o.taskByRef[x.id]
		}
	case *GroupHandle:
		if x == nil || x.id == "" {
			return predecessor{}, false
		}
		if x.out == o {
			pred.col = o.tasksByRef[x.id]
		}
	case *SequenceHandle:
		if x == nil || x.tasks == nil {
			return predecessor{}, false
		}
		return o.predecessorOfLocked(x.tasks)
	default:
		return predecessor{}, false
	}
	return pred, true
}

// outcomeLocked is p's outcome and, while that is pending, what to park
// on until it changes: p itself, or for an empty collection that answers
// for its entry, the pending predecessor inside that entry.
func (o *Output) outcomeLocked(p predecessor) (schedule.Outcome, predecessor) {
	switch {
	case p.task != nil:
		return o.taskOutcomeLocked(p.task), p
	case p.col != nil:
		return o.collectionOutcomeLocked(p)
	default:
		return schedule.Failed, p
	}
}

// taskOutcomeLocked is t's outcome for its dependents. Once Finish drains,
// a Task nobody Defined never will be, so it can no longer succeed.
func (o *Output) taskOutcomeLocked(t *taskState) schedule.Outcome {
	out := schedule.OutcomeOf(t.state.Current())
	if out == schedule.Pending && o.sched.draining && t.neverDefined() {
		return schedule.Failed
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
func (o *Output) collectionOutcomeLocked(p predecessor) (schedule.Outcome, predecessor) {
	t := &p.col.tally
	cursor, open := o.edgeCursorLocked(p)
	switch {
	case open && t.AnyFailed(), !open && t.FailedThrough(cursor):
		return schedule.Failed, p
	case open:
		return schedule.Pending, p
	case !t.HasMemberThrough(cursor):
		return o.entryOutcomeLocked(p.col)
	case t.SucceededThrough(cursor):
		return schedule.Succeeded, p
	default:
		return schedule.Pending, p
	}
}

// entryOutcomeLocked folds c's entry, the one step c starts after in a
// Sequence (none elsewhere), with the first predecessor still pending.
// Holding only that step keeps a Sequence of n nested steps at n
// predecessors; a chain of empty steps forwards along the chain.
func (o *Output) entryOutcomeLocked(c *tasksState) (schedule.Outcome, predecessor) {
	verdict, blocker := schedule.Succeeded, predecessor{}
	for _, p := range c.entry {
		out, b := o.outcomeLocked(p)
		if out == schedule.Failed {
			return schedule.Failed, b
		}
		if out == schedule.Pending && verdict == schedule.Succeeded {
			verdict, blocker = schedule.Pending, b
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

// predsOutcomeLocked folds st's predecessors into one outcome and, while
// that outcome is pending, the first predecessor still pending.
//
// It forgets each Task predecessor it finds succeeded: a terminal state
// never reverts, and rereading it on every wake would make fan-in over n
// Tasks cost n² reads. A collection predecessor is kept, because a newly
// declared member can make it pending again.
func (o *Output) predsOutcomeLocked(st *taskState, scan predScan) (schedule.Outcome, predecessor) {
	verdict, blocker := schedule.Succeeded, predecessor{}
	preds := st.sched.preds
	kept := preds[:0]
	i := 0
	for ; i < len(preds); i++ {
		p := preds[i]
		o.sched.predChecks++
		out, parkOn := o.outcomeLocked(p)
		if out == schedule.Failed {
			verdict, blocker = schedule.Failed, p
			break
		}
		if out == schedule.Pending && verdict == schedule.Succeeded {
			verdict, blocker = schedule.Pending, parkOn
		}
		if out != schedule.Succeeded || p.task == nil {
			kept = append(kept, p)
		}
		if verdict == schedule.Pending && scan == scanToBlocker {
			i++
			break
		}
	}
	kept = append(kept, preds[i:]...)
	clear(preds[len(kept):])
	st.sched.preds = kept
	return verdict, blocker
}

func (o *Output) eligibleLocked(st *taskState) bool {
	verdict, _ := o.predsOutcomeLocked(st, scanToBlocker)
	return verdict == schedule.Succeeded
}

// enterPhaseLocked moves st to phase, keeping the parked count true.
func (o *Output) enterPhaseLocked(st *taskState, phase schedPhase) {
	if st.sched.phase == phaseParked {
		o.sched.parked--
	}
	if phase == phaseParked {
		o.sched.parked++
	}
	st.sched.phase = phase
}

// placeLocked routes submitted st by what its predecessors say: queued
// when all succeeded, parked on the first one still pending, and settled
// NotStarted when one can never succeed. scan says how far it reads them.
// st must not be in the queue.
func (o *Output) placeLocked(st *taskState, scan predScan) {
	verdict, blocker := o.predsOutcomeLocked(st, scan)
	switch verdict {
	case schedule.Succeeded:
		o.enterPhaseLocked(st, phaseQueued)
		o.sched.queue.Push(st)
	case schedule.Pending:
		o.enterPhaseLocked(st, phaseParked)
		if blocker.task != nil {
			blocker.task.sched.dependents = append(blocker.task.sched.dependents, st)
		} else {
			cursor, open := o.edgeCursorLocked(blocker)
			blocker.col.tally.Park(st, cursor, open)
		}
	case schedule.Failed:
		o.markNotStartedLocked(st)
	}
}

// wakeLocked re-places every parked Task in deps, now that what it was
// parked on stopped pending. A woken Task that settles NotStarted wakes
// its own dependents; the worklist keeps that cascade iterative however
// long the chain.
func (o *Output) wakeLocked(deps []*taskState) {
	o.sched.woken = append(o.sched.woken, deps...)
	if !o.sched.waking {
		o.drainWokenLocked()
	}
}

// holdWakesLocked defers every wake until the returned release runs, so a
// batch of settles is decided before any of them cascades.
func (o *Output) holdWakesLocked() (release func()) {
	o.sched.waking = true
	return o.drainWokenLocked
}

func (o *Output) drainWokenLocked() {
	o.sched.waking = true
	for i := 0; i < len(o.sched.woken); i++ {
		st := o.sched.woken[i]
		o.sched.woken[i] = nil
		if st.sched.phase == phaseParked && !core.IsTerminalTask(st.state.Current()) {
			o.placeLocked(st, scanToBlocker)
		}
	}
	o.sched.woken = o.sched.woken[:0]
	o.sched.waking = false
}

// propagateSettleLocked tells st's dependents and its collections that st
// settled.
func (o *Output) propagateSettleLocked(st *taskState) {
	deps := st.sched.dependents
	st.sched.dependents = nil
	for c := st.collection; c != nil; c = c.parent {
		deps = append(deps, c.tally.Settle(st)...)
	}
	o.wakeLocked(deps)
}

// tallyDeclaredLocked counts a newly declared Task in every collection it
// sits under.
func tallyDeclaredLocked(st *taskState) {
	for c := st.collection; c != nil; c = c.parent {
		c.tally.Declare(st)
	}
}

// replaceParkedLocked re-places every parked Task. Draining calls it once:
// a Task nobody Defined and an empty collection stop pending then, without
// any settle to wake the Tasks parked on them.
func (o *Output) replaceParkedLocked() {
	var parked []*taskState
	for _, st := range o.tasks {
		st.sched.dependents = nil
		if st.sched.phase == phaseParked {
			parked = append(parked, st)
		}
	}
	for _, col := range o.tasksByRef {
		col.tally.Unpark()
	}
	for _, st := range parked {
		if st.sched.phase == phaseParked && !core.IsTerminalTask(st.state.Current()) {
			o.placeLocked(st, scanAll)
		}
	}
}
