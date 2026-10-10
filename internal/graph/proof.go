package graph

// OrderedAfter reports whether consumer can only run once producer
// settled: producer is reachable through the After edges declared on the
// consumer or a container it sits in, or through Sequence order (an earlier
// step of a Sequence settles before a later one starts).
func (g *Graph) OrderedAfter(consumer, producer *Task) bool {
	g.lock()
	defer g.unlock()
	if consumer == producer {
		return false
	}
	w := proofWalk{producer: producer, seenTasks: map[*Task]struct{}{}, seenCols: map[*Container]struct{}{}}
	if consumer.gateFor != nil {
		return w.col(consumer.gateFor)
	}
	return w.task(consumer)
}

// proofWalk searches for the ordering edges that prove a producer settled.
type proofWalk struct {
	producer  *Task
	seenTasks map[*Task]struct{}
	seenCols  map[*Container]struct{}
}

func (w *proofWalk) task(t *Task) bool {
	if t == w.producer {
		return true
	}
	if _, ok := w.seenTasks[t]; ok {
		return false
	}
	w.seenTasks[t] = struct{}{}
	if w.sequenceStepBefore(t.Parent, t.Declaration) || w.preds(t.after) {
		return true
	}
	return w.col(t.Parent)
}

// col walks c and its ancestors: a container starts only once its own After
// edges and its Sequence predecessors settled.
func (w *proofWalk) col(c *Container) bool {
	for ; c != nil; c = c.Parent {
		if _, ok := w.seenCols[c]; ok {
			return false
		}
		w.seenCols[c] = struct{}{}
		if w.preds(c.entry) || (c.Parent != nil && w.sequenceStepBefore(c.Parent, c.Declaration)) {
			return true
		}
	}
	return false
}

func (w *proofWalk) preds(preds []Predecessor) bool {
	for _, p := range preds {
		switch {
		case p.task != nil && w.task(p.task):
			return true
		case p.col != nil && (containsTask(p.col, w.producer) || w.col(p.col)):
			return true
		}
	}
	return false
}

// sequenceStepBefore reports whether seq is a Sequence that holds the
// producer, in a step declared before the step at declaration.
func (w *proofWalk) sequenceStepBefore(seq *Container, declaration int) bool {
	if seq == nil || !seq.Sequential {
		return false
	}
	step := w.stepDeclaration(seq)
	return step != 0 && step < declaration
}

// stepDeclaration is the declaration of the step of seq that holds the
// producer, or zero when the producer is not inside seq.
func (w *proofWalk) stepDeclaration(seq *Container) int {
	if w.producer.Parent == seq {
		return w.producer.Declaration
	}
	for c := w.producer.Parent; c != nil; c = c.Parent {
		if c.Parent == seq {
			return c.Declaration
		}
	}
	return 0
}

// containsTask reports whether t is a member of c, at any depth.
func containsTask(c *Container, t *Task) bool {
	for m := t.Parent; m != nil; m = m.Parent {
		if m == c {
			return true
		}
	}
	return false
}
