package engine

// consumerScope names the Task (or container builder gate) whose callback
// the calling goroutine is running, so Computed.Get can ask whether that
// consumer is ordered after the producer. Go has no goroutine-local
// storage and Get takes no context, so the scheduler records it per
// goroutine.
func (o *Output) enterConsumer(st *taskState) (leave func()) {
	g := currentGoroutine()
	o.mu.Lock()
	if o.sched.consumers == nil {
		o.sched.consumers = make(map[goroutineID][]*taskState)
	}
	o.sched.consumers[g] = append(o.sched.consumers[g], st)
	o.mu.Unlock()
	return func() {
		o.mu.Lock()
		stack := o.sched.consumers[g]
		if len(stack) <= 1 {
			delete(o.sched.consumers, g)
		} else {
			o.sched.consumers[g] = stack[:len(stack)-1]
		}
		o.mu.Unlock()
	}
}

// currentConsumerLocked is the Task or builder gate running on the calling
// goroutine, or nil for a caller outside any callback.
func (o *Output) currentConsumerLocked() *taskState {
	stack := o.sched.consumers[currentGoroutine()]
	if len(stack) == 0 {
		return nil
	}
	return stack[len(stack)-1]
}

// orderedAfterLocked reports whether consumer can only run once producer
// settled: producer is reachable through the After edges declared on the
// consumer or a container it sits in, or through Sequence order (an
// earlier step of a Sequence settles before a later one starts).
func (o *Output) orderedAfterLocked(consumer, producer *taskState) bool {
	if consumer == producer {
		return false
	}
	w := proofWalk{producer: producer, seenTasks: map[*taskState]struct{}{}, seenCols: map[*tasksState]struct{}{}}
	if consumer.gateFor != nil {
		return w.col(consumer.gateFor)
	}
	return w.task(consumer)
}

// proofWalk searches for the ordering edges that prove a producer settled.
type proofWalk struct {
	producer  *taskState
	seenTasks map[*taskState]struct{}
	seenCols  map[*tasksState]struct{}
}

func (w *proofWalk) task(t *taskState) bool {
	if t == w.producer {
		return true
	}
	if _, ok := w.seenTasks[t]; ok {
		return false
	}
	w.seenTasks[t] = struct{}{}
	if w.sequenceStepBefore(t.collection, t.declaration) || w.preds(t.after) {
		return true
	}
	return w.col(t.collection)
}

// col walks c and its ancestors: a container starts only once its own After
// edges and its Sequence predecessors settled.
func (w *proofWalk) col(c *tasksState) bool {
	for ; c != nil; c = c.parent {
		if _, ok := w.seenCols[c]; ok {
			return false
		}
		w.seenCols[c] = struct{}{}
		if w.preds(c.entry) || (c.parent != nil && w.sequenceStepBefore(c.parent, c.declaration)) {
			return true
		}
	}
	return false
}

func (w *proofWalk) preds(preds []predecessor) bool {
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
func (w *proofWalk) sequenceStepBefore(seq *tasksState, declaration int) bool {
	if seq == nil || !seq.sequential {
		return false
	}
	step := w.stepDeclaration(seq)
	return step != 0 && step < declaration
}

// stepDeclaration is the declaration of the step of seq that holds the
// producer, or zero when the producer is not inside seq.
func (w *proofWalk) stepDeclaration(seq *tasksState) int {
	if w.producer.collection == seq {
		return w.producer.declaration
	}
	for c := w.producer.collection; c != nil; c = c.parent {
		if c.parent == seq {
			return c.declaration
		}
	}
	return 0
}

// containsTask reports whether t is a member of c, at any depth.
func containsTask(c *tasksState, t *taskState) bool {
	for m := t.collection; m != nil; m = m.parent {
		if m == c {
			return true
		}
	}
	return false
}
