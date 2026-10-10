package graph

// nextStepPreds is what a child declared in c now starts after. In a
// Sequence that is the step declared just before it, whether that step is a
// Task or a nested Group/Sequence; for a Sequence's first step, and in a
// Group, it is what c itself starts after.
func (c *Container) nextStepPreds() []Predecessor {
	if c.Sequential && c.lastStep != nil {
		return c.lastStep
	}
	return c.entry
}

// appendStepPredsLocked appends what the next step declared into c starts
// after, closing the membership of a populated collection among them (see
// closeMembershipLocked).
func (g *Graph) appendStepPredsLocked(preds []Predecessor, c *Container) []Predecessor {
	for _, p := range c.nextStepPreds() {
		g.closeMembershipLocked(p)
		preds = append(preds, p)
	}
	return preds
}

// recordStep makes step c's latest step when c is a Sequence. The next step
// starts after this one alone: an empty nested collection answers for the
// step before it (see collectionOutcomeLocked), so nothing earlier has to be
// carried forward.
func (c *Container) recordStep(step Predecessor) {
	if c.Sequential {
		c.lastStep = []Predecessor{step}
	}
}

// isLastStep reports whether step is c's latest step.
func (c *Container) isLastStep(step *Container) bool {
	return len(c.lastStep) == 1 && c.lastStep[0].col == step
}

// joinPassedSequencesLocked orders self, declared into c, after every
// enclosing Sequence that has already moved past the step holding c and
// taken that step's membership as declared: self is declared after the
// Sequence's latest step, so it runs after that step and becomes the
// Sequence's latest step itself. Declaration order and one Running child
// then hold whatever the step's earlier members took (E-092). A step the
// Sequence passed while empty is still open, so the later step already
// waits for self and nothing changes.
func (g *Graph) joinPassedSequencesLocked(preds []Predecessor, c *Container, self Predecessor) []Predecessor {
	for step, seq := c, c.Parent; seq != nil; step, seq = seq, seq.Parent {
		if !seq.Sequential || !step.tally.sealed || seq.isLastStep(step) {
			continue
		}
		preds = g.appendStepPredsLocked(preds, seq)
		seq.recordStep(self)
	}
	return preds
}

// joinContainerLocked wires t, newly declared under parent, into the
// scheduler: it starts after the step before it, it is parent's latest
// step, and every container above counts it.
func (g *Graph) joinContainerLocked(t *Task, parent *Container) {
	t.sched.preds = g.appendStepPredsLocked(t.sched.preds, parent)
	t.sched.preds = g.joinPassedSequencesLocked(t.sched.preds, parent, AfterTask(t))
	parent.recordStep(AfterTask(t))
	parent.tasks = append(parent.tasks, t)
	for c := parent; c != nil; c = c.Parent {
		c.tally.declare(t)
	}
}

// joinParentLocked wires container c, newly declared under parent, into the
// scheduler: it starts after the step before it and is parent's latest step.
func (g *Graph) joinParentLocked(c, parent *Container) {
	c.entry = g.appendStepPredsLocked(nil, parent)
	c.entry = g.joinPassedSequencesLocked(c.entry, parent, AfterContainer(c))
	parent.recordStep(AfterContainer(c))
	parent.children = append(parent.children, c)
}
