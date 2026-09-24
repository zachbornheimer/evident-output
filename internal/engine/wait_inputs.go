package engine

import "github.com/zachbornheimer/evident-output/internal/core"

// sealAwaitedInputs seals everything the awaited Task waits for (see
// sealInputsLocked). A Wait asks for the answer now: work nobody supplied
// before the Wait is not coming, and treating it as pending would park
// the caller on declarations only the caller could still make.
func (o *Output) sealAwaitedInputs(taskID string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	st := o.taskByRef[taskID]
	if st == nil || st.neverDefined() || core.IsTerminalTask(st.state) {
		return
	}
	o.sealInputsLocked(st, true)
}

// sealWaitedInputsLocked seals what every parked Wait waits for, once the
// run proved it cannot move. It reports whether it sealed anything. It
// backs sealAwaitedInputs for inputs declared after that Wait walked.
func (o *Output) sealWaitedInputsLocked() bool {
	sealed := false
	for ticket := range o.sched.waits {
		st := o.taskByRef[ticket.taskID]
		if st != nil && !core.IsTerminalTask(st.state) && o.sealInputsLocked(st, false) {
			sealed = true
		}
	}
	return sealed
}

// sealInputsLocked walks everything root waits for, directly or through
// its predecessors and the members of collections it runs After, and
// seals what nothing will now supply: a Task nobody Defined settles
// NotStarted, and a still-empty collection stops being waited on. It
// reports whether it sealed anything.
//
// trustEarlier skips a Task or collection an earlier Wait already sealed
// and that gained nothing since, so a Group Wait over many members walks
// the shared inputs once.
func (o *Output) sealInputsLocked(root *taskState, trustEarlier bool) bool {
	w := inputWalk{
		trustEarlier: trustEarlier,
		tasks:        map[*taskState]struct{}{root: {}},
		cols:         map[*tasksState]struct{}{},
		stack:        []*taskState{root},
	}
	for len(w.stack) > 0 {
		t := w.stack[len(w.stack)-1]
		w.stack = w.stack[:len(w.stack)-1]
		w.expand(t)
	}
	o.wakeLocked(w.wake)
	for _, t := range w.undefined {
		if t.neverDefined() {
			o.markNotStartedLocked(t)
		}
	}
	return len(w.wake)+len(w.undefined) > 0
}

// inputWalk is one sealInputsLocked pass. It visits each Task and
// collection once, so an After cycle or a diamond of predecessors costs
// one step per node, never one per path.
type inputWalk struct {
	trustEarlier bool
	tasks        map[*taskState]struct{}
	cols         map[*tasksState]struct{}
	stack        []*taskState
	// undefined are the Tasks nobody Defined that the walk reached.
	undefined []*taskState
	// wake are the Tasks parked on a collection the walk sealed.
	wake []*taskState
}

// expand visits every predecessor of submitted Task t.
func (w *inputWalk) expand(t *taskState) {
	if w.trustEarlier && t.sched.inputsSealed {
		return
	}
	t.sched.inputsSealed = t.sched.submitted()
	for _, p := range t.sched.preds {
		w.visit(p)
	}
}

func (w *inputWalk) visit(p predecessor) {
	switch {
	case p.task != nil:
		w.visitTask(p.task)
	case p.col != nil:
		w.visitCollection(p.col)
	}
}

func (w *inputWalk) visitTask(t *taskState) {
	if _, seen := w.tasks[t]; seen || core.IsTerminalTask(t.state) {
		return
	}
	w.tasks[t] = struct{}{}
	if t.neverDefined() {
		w.undefined = append(w.undefined, t)
		return
	}
	w.stack = append(w.stack, t)
}

func (w *inputWalk) visitCollection(c *tasksState) {
	if _, seen := w.cols[c]; seen {
		return
	}
	w.cols[c] = struct{}{}
	t := &c.tally
	if t.total == 0 {
		if !t.sealed {
			t.sealed = true
			w.wake = append(w.wake, t.dependents...)
			t.dependents = nil
		}
		// A sealed empty collection answers for its entry, so what the
		// entry waits for is waited for too.
		for _, p := range c.entry {
			w.visit(p)
		}
		return
	}
	if w.trustEarlier && t.walked == t.total {
		return
	}
	t.walked = t.total
	for _, member := range appendDescendantTasksLocked(c, nil) {
		w.visitTask(member)
	}
}
