package engine

import "github.com/zachbornheimer/evident-output/internal/core"

// sealAwaitedInputs seals everything the awaited Task waits for (see
// sealInputsLocked). A Wait asks for the answer now: work nobody supplied
// before the Wait is not coming, and treating it as pending would park
// the caller on declarations only the caller could still make. That holds
// for the awaited Task itself: nobody Defined it, so it settles NotStarted
// just as it would when reached as a predecessor. A nil seen walks fresh.
func (o *Output) sealAwaitedInputs(taskID string, seen *inputSeals) {
	o.mu.Lock()
	defer o.mu.Unlock()
	st := o.taskByRef[taskID]
	switch {
	case st == nil || core.IsTerminalTask(st.rec.State()):
	case st.neverDefined():
		o.markNotStartedLocked(st)
	default:
		if seen == nil {
			seen = &inputSeals{}
		}
		o.sealInputsLocked(st, seen)
	}
}

// sealWaitedInputsLocked seals what every parked Wait waits for, once the
// run proved it cannot move. It reports whether it sealed anything. It
// backs sealAwaitedInputs for inputs declared after that Wait walked.
func (o *Output) sealWaitedInputsLocked() bool {
	var seen inputSeals
	sealed := false
	for ticket := range o.sched.waits {
		st := o.taskByRef[ticket.taskID]
		if st != nil && !core.IsTerminalTask(st.rec.State()) && o.sealInputsLocked(st, &seen) {
			sealed = true
		}
	}
	return sealed
}

// inputSeals is what one Wait has walked so far. A Group or Sequence Wait
// shares one across its members, so inputs they share are walked once
// per Wait (the zero value is ready to use); nothing carries over from an earlier Wait, so how a Wait
// answers never depends on what ran before it.
type inputSeals struct {
	tasks map[*taskState]struct{}
	// cols holds, per collection, the cursor its members were walked
	// through.
	cols map[*tasksState]int
}

// begin readies s for a walk from root, and reports false when an earlier
// member's walk already covered root. The zero inputSeals is empty, and a
// Wait on a settled Task never walks, so it allocates nothing.
func (s *inputSeals) begin(root *taskState) bool {
	if s.tasks == nil {
		s.tasks = map[*taskState]struct{}{}
		s.cols = map[*tasksState]int{}
	}
	if _, walked := s.tasks[root]; walked {
		return false
	}
	s.tasks[root] = struct{}{}
	return true
}

// sealInputsLocked walks everything root waits for, directly or through
// its predecessors and the members of collections it runs After, and
// seals what nothing will now supply: a Task nobody Defined settles
// NotStarted, and a collection's membership is taken as declared. It
// skips what seen already holds, and reports whether it sealed anything.
func (o *Output) sealInputsLocked(root *taskState, seen *inputSeals) bool {
	if !seen.begin(root) {
		return false
	}
	w := inputWalk{seen: seen, stack: []*taskState{root}, cursor: o.declSeq}
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
	seen  *inputSeals
	stack []*taskState
	// undefined are the Tasks nobody Defined that the walk reached.
	undefined []*taskState
	// wake are the Tasks parked on a collection the walk sealed.
	wake []*taskState
	// cursor is the declaration cursor the walk seals memberships through.
	cursor int
}

// expand visits every predecessor of submitted Task t.
func (w *inputWalk) expand(t *taskState) {
	for _, p := range t.sched.preds {
		w.visit(p)
	}
}

func (w *inputWalk) visit(p predecessor) {
	switch {
	case p.task != nil:
		w.visitTask(p.task)
	case p.col != nil:
		w.visitCollection(p)
	}
}

func (w *inputWalk) visitTask(t *taskState) {
	if _, seen := w.seen.tasks[t]; seen || core.IsTerminalTask(t.rec.State()) {
		return
	}
	w.seen.tasks[t] = struct{}{}
	if t.neverDefined() {
		w.undefined = append(w.undefined, t)
		return
	}
	w.stack = append(w.stack, t)
}

// visitCollection walks the members edge p waits for: those declared
// through its cursor. Nothing declared after this Wait walked can gate
// it, so an open edge's membership is sealed here first.
func (w *inputWalk) visitCollection(p predecessor) {
	c, t := p.col, &p.col.tally
	w.wake = append(w.wake, t.seal(w.cursor)...)
	cursor := p.through
	if cursor == 0 {
		cursor = t.sealedThrough
	}
	walked, seen := w.seen.cols[c]
	if seen && walked >= cursor {
		return
	}
	w.seen.cols[c] = cursor
	if !t.hasMemberThrough(cursor) {
		// A sealed empty collection answers for its entry, so what the
		// entry waits for is waited for too.
		if !seen {
			for _, e := range c.entry {
				w.visit(e)
			}
		}
		return
	}
	for _, member := range t.membersThrough(walked, cursor) {
		w.visitTask(member)
	}
}
