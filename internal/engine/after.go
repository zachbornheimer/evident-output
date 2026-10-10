package engine

import "github.com/zachbornheimer/evident-output/internal/graph"

// After declares predecessors: this Task starts only once every one of
// them succeeded, and never starts once one of them cannot.
func (t *TaskHandle) After(preds ...any) *TaskHandle {
	if t == nil || t.out == nil {
		return t
	}
	o := t.out
	o.mu.Lock()
	defer o.mu.Unlock()
	st := o.taskStates[t.id]
	if st == nil {
		return t
	}
	if !o.graph.AddAfter(st.node, o.predecessorsOfLocked(preds)...) {
		o.recordMisuse(ErrInvalidConfig)
	}
	return t
}

// predecessorsOfLocked resolves After's arguments, leaving out the ones
// After ignores.
func (o *Output) predecessorsOfLocked(args []any) []graph.Predecessor {
	preds := make([]graph.Predecessor, 0, len(args))
	for _, arg := range args {
		if pred, ok := o.predecessorOfLocked(arg); ok {
			preds = append(preds, pred)
		}
	}
	return preds
}

// predecessorOfLocked resolves one After argument. ok is false for a nil
// handle, which After ignores. A handle this Output never declared yields
// the empty predecessor, which never succeeds.
func (o *Output) predecessorOfLocked(p any) (pred graph.Predecessor, ok bool) {
	switch x := p.(type) {
	case *TaskHandle:
		if x == nil || x.id == "" {
			return graph.Predecessor{}, false
		}
		if x.out == o {
			return graph.AfterTask(o.nodeOfTask(x.id)), true
		}
	case *GroupHandle:
		if x == nil || x.id == "" {
			return graph.Predecessor{}, false
		}
		if x.out == o {
			return graph.AfterContainer(o.nodeOfContainer(x.id)), true
		}
	case *SequenceHandle:
		if x == nil || x.tasks == nil {
			return graph.Predecessor{}, false
		}
		return o.predecessorOfLocked(x.tasks)
	default:
		return graph.Predecessor{}, false
	}
	return graph.Predecessor{}, true
}

// nodeOfTask is the graph's declaration of the Task named id, nil when this
// Output declared none.
func (o *Output) nodeOfTask(id string) *graph.Task {
	if st := o.taskStates[id]; st != nil {
		return st.node
	}
	return nil
}

// nodeOfContainer is the graph's declaration of the container named id, nil
// when this Output declared none.
func (o *Output) nodeOfContainer(id string) *graph.Container {
	if st := o.containerStates[id]; st != nil {
		return st.node
	}
	return nil
}
