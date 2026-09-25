package engine

// SequenceHandle is the front door for ordered child work: the same
// scheduler as Group with implicit predecessor edges in declaration order.
// A failed/blocked/cancelled child makes later siblings NotStarted.
type SequenceHandle struct {
	tasks *GroupHandle
	// facade holds this handle's public wrapper (see FacadeSlot).
	facade FacadeSlot
}

// Task declares (or, for a repeated name, returns) a child task in
// declaration order.
func (g *SequenceHandle) Task(name string) *TaskHandle {
	if g == nil || g.tasks == nil {
		return &TaskHandle{}
	}
	return g.tasks.out.declareGroupTask(g.tasks, name)
}

// Summary sets a success-oriented sequence summary.
func (g *SequenceHandle) Summary(text string) *SequenceHandle {
	if g == nil || g.tasks == nil {
		return g
	}
	g.tasks.Summary(text)
	return g
}

// Snapshot returns the sequence snapshot with derived state.
func (g *SequenceHandle) Snapshot() TasksSnapshot {
	if g == nil || g.tasks == nil {
		return TasksSnapshot{State: Empty}
	}
	return g.tasks.Snapshot()
}

// Sequence declares (or returns) an ordered child container nested here.
func (g *SequenceHandle) Sequence(name string) *SequenceHandle {
	if g == nil || g.tasks == nil {
		return &SequenceHandle{}
	}
	return g.tasks.Sequence(name)
}

// Group declares (or returns) an independent child container nested here.
func (g *SequenceHandle) Group(name string) *GroupHandle {
	if g == nil || g.tasks == nil {
		return &GroupHandle{}
	}
	return g.tasks.Group(name)
}

// nextStepPreds is what a child declared in c now starts after. In a
// Sequence that is the step declared just before it, whether that step is
// a Task or a nested Group/Sequence; for a Sequence's first step, and in a
// Group, it is what c itself starts after.
func (c *tasksState) nextStepPreds() []predecessor {
	if c.sequential && c.lastStep != nil {
		return c.lastStep
	}
	return c.entry
}

// appendStepPredsLocked appends what the next step declared into c starts
// after, closing the membership of a populated collection among them (see
// closeMembershipLocked).
func (o *Output) appendStepPredsLocked(preds []predecessor, c *tasksState) []predecessor {
	for _, p := range c.nextStepPreds() {
		o.closeMembershipLocked(p)
		preds = append(preds, p)
	}
	return preds
}

// recordStep makes step c's latest step when c is a Sequence. The next
// step starts after this one alone: an empty nested collection answers for
// the step before it (see collectionOutcomeLocked), so nothing earlier has
// to be carried forward.
func (c *tasksState) recordStep(step predecessor) {
	if c.sequential {
		c.lastStep = []predecessor{step}
	}
}
