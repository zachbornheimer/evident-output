package engine

import "github.com/zachbornheimer/evident-output/internal/core"

// predecessor is one thing a Task waits for: another Task, or a whole
// Group/Sequence. Both nil is a handle this run never declared, which can
// never succeed.
type predecessor struct {
	task *taskState
	col  *tasksState
}

// predOutcome is what a predecessor currently tells the Tasks after it.
type predOutcome uint8

const (
	// predPending: it may still succeed.
	predPending predOutcome = iota
	predSucceeded
	// predFailed: it can never succeed, so its dependents never start.
	predFailed
)

// stateOutcome classifies a Task state for the Tasks after it.
func stateOutcome(s EntityState) predOutcome {
	switch s {
	case Done, Skipped:
		return predSucceeded
	case Failed, Blocked, Cancelled, NotStarted:
		return predFailed
	default:
		return predPending
	}
}

// collectionTally counts a Group/Sequence's descendant Tasks by outcome.
// declareTaskLocked and settleLocked keep it current, so reading a
// collection's outcome costs the same however large the collection grows.
type collectionTally struct {
	total, succeeded, failed int
	// sealed records that the run proved nothing will declare into this
	// still-empty collection (see sealEmptyPredecessorsLocked).
	sealed bool
	// dependents are the Tasks parked until this collection stops pending.
	dependents []*taskState
}

func (t *collectionTally) count(o predOutcome, delta int) {
	switch o {
	case predSucceeded:
		t.succeeded += delta
	case predFailed:
		t.failed += delta
	case predPending:
	}
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
			st.sched.preds = append(st.sched.preds, pred)
		}
	}
	return t
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

func (o *Output) outcomeLocked(p predecessor) predOutcome {
	switch {
	case p.task != nil:
		return o.taskOutcomeLocked(p.task)
	case p.col != nil:
		return o.collectionOutcomeLocked(p.col)
	default:
		return predFailed
	}
}

// taskOutcomeLocked is t's outcome for its dependents. Once Finish drains,
// a Task nobody Defined never will be, so it can no longer succeed.
func (o *Output) taskOutcomeLocked(t *taskState) predOutcome {
	out := stateOutcome(t.state)
	if out == predPending && o.sched.draining && t.neverDefined() {
		return predFailed
	}
	return out
}

// collectionOutcomeLocked is c's outcome for its dependents. An empty
// collection is still pending while its caller may populate it: a Task
// wired After a Group before the Group's children were declared must wait
// for them. Once Finish drains, or the run proved nothing will populate
// it, an empty collection has nothing left to wait for.
func (o *Output) collectionOutcomeLocked(c *tasksState) predOutcome {
	t := &c.tally
	switch {
	case t.failed > 0:
		return predFailed
	case t.total == 0:
		if o.sched.draining || t.sealed {
			return predSucceeded
		}
		return predPending
	case t.succeeded == t.total:
		return predSucceeded
	default:
		return predPending
	}
}

// predsOutcomeLocked folds st's predecessors into one outcome and, while
// that outcome is pending, the first predecessor still pending.
func (o *Output) predsOutcomeLocked(st *taskState) (predOutcome, predecessor) {
	verdict, blocker := predSucceeded, predecessor{}
	for _, p := range st.sched.preds {
		switch o.outcomeLocked(p) {
		case predFailed:
			return predFailed, p
		case predPending:
			if verdict == predSucceeded {
				verdict, blocker = predPending, p
			}
		case predSucceeded:
		}
	}
	return verdict, blocker
}

func (o *Output) eligibleLocked(st *taskState) bool {
	verdict, _ := o.predsOutcomeLocked(st)
	return verdict == predSucceeded
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
// NotStarted when one can never succeed. st must not be in the queue.
func (o *Output) placeLocked(st *taskState) {
	verdict, blocker := o.predsOutcomeLocked(st)
	switch verdict {
	case predSucceeded:
		o.enterPhaseLocked(st, phaseQueued)
		o.sched.queue.push(st)
	case predPending:
		o.enterPhaseLocked(st, phaseParked)
		if blocker.task != nil {
			blocker.task.sched.dependents = append(blocker.task.sched.dependents, st)
		} else {
			blocker.col.tally.dependents = append(blocker.col.tally.dependents, st)
		}
	case predFailed:
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
		if st.sched.phase == phaseParked && !core.IsTerminalTask(st.state) {
			o.placeLocked(st)
		}
	}
	o.sched.woken = o.sched.woken[:0]
	o.sched.waking = false
}

// propagateSettleLocked tells st's dependents and its collections that st
// settled, having been from before.
func (o *Output) propagateSettleLocked(st *taskState, from predOutcome) {
	deps := st.sched.dependents
	st.sched.dependents = nil
	to := stateOutcome(st.state)
	for c := st.collection; c != nil; c = c.parent {
		c.tally.count(from, -1)
		c.tally.count(to, 1)
		if len(c.tally.dependents) > 0 && o.collectionOutcomeLocked(c) != predPending {
			deps = append(deps, c.tally.dependents...)
			c.tally.dependents = nil
		}
	}
	o.wakeLocked(deps)
}

// tallyDeclaredLocked counts a newly declared Task in every collection it
// sits under.
func tallyDeclaredLocked(st *taskState) {
	for c := st.collection; c != nil; c = c.parent {
		c.tally.total++
		c.tally.count(stateOutcome(st.state), 1)
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
		col.tally.dependents = nil
	}
	for _, st := range parked {
		if st.sched.phase == phaseParked && !core.IsTerminalTask(st.state) {
			o.placeLocked(st)
		}
	}
}

// sealAwaitedInputs seals every still-empty collection the awaited Task
// waits for, directly or through the Tasks it runs After. A Wait asks for
// the answer now: a Group nobody populated before the Wait has nothing
// left to wait for, and treating it as pending would park the caller on
// children only the caller could still declare.
//
// A submitted Task's predecessors are frozen, so each is walked once per
// run however many Waits reach it.
func (o *Output) sealAwaitedInputs(taskID string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	st := o.taskByRef[taskID]
	if st == nil {
		return
	}
	var deps []*taskState
	stack := []*taskState{st}
	for len(stack) > 0 {
		t := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if t.sched.inputsSealed || core.IsTerminalTask(t.state) {
			continue
		}
		t.sched.inputsSealed = t.sched.submitted()
		for _, p := range t.sched.preds {
			switch {
			case p.task != nil:
				stack = append(stack, p.task)
			case p.col != nil && p.col.tally.total == 0 && !p.col.tally.sealed:
				p.col.tally.sealed = true
				deps = append(deps, p.col.tally.dependents...)
				p.col.tally.dependents = nil
			}
		}
	}
	o.wakeLocked(deps)
}

// sealEmptyPredecessorsLocked seals every still-empty collection a Task is
// parked on, once the run proved it cannot move: a waiter is parked, no
// callback runs, and no Task is left for the caller to Define. Nothing
// will populate those collections now, so waiting on them is waiting on
// nothing. It reports whether it woke anyone. It backs sealAwaitedInputs
// for an empty collection reached only through another collection's
// members.
func (o *Output) sealEmptyPredecessorsLocked() bool {
	var deps []*taskState
	for _, col := range o.tasksByRef {
		if col.tally.total == 0 && !col.tally.sealed && len(col.tally.dependents) > 0 {
			col.tally.sealed = true
			deps = append(deps, col.tally.dependents...)
			col.tally.dependents = nil
		}
	}
	o.wakeLocked(deps)
	return len(deps) > 0
}
