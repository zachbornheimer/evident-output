package evo

import "github.com/zachbornheimer/evident-output/v2/internal/graph"

// Predecessor is a handle After accepts: *Task, *Group, *Sequence or *Computed[T]. Sealed.
type Predecessor interface {
	predecessor()
}

func (*Task) predecessor() {}

func (*Group) predecessor() {}

func (*Sequence) predecessor() {}

// After declares predecessors: this Task starts only once all succeeded, never if one cannot.
func (t *Task) After(preds ...Predecessor) *Task {
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
	o.graph.After(st.node, o.predecessorsOfLocked(preds)...)
	return t
}

func (r *Run) predecessorsOfLocked(args []Predecessor) []graph.Predecessor {
	preds := make([]graph.Predecessor, 0, len(args))
	for _, arg := range args {
		if pred, ok := r.predecessorOfLocked(arg); ok {
			preds = append(preds, pred)
		}
	}
	return preds
}

// predecessorOfLocked resolves one After argument; a handle from another Output yields the empty predecessor, which never succeeds.
func (r *Run) predecessorOfLocked(p Predecessor) (pred graph.Predecessor, ok bool) {
	switch x := p.(type) {
	case *Task:
		if x == nil || x.id == "" {
			return graph.Predecessor{}, false
		}
		if x.out == r {
			return graph.AfterTask(r.nodeOfTask(x.id)), true
		}
	case *Group:
		if x == nil || x.id == "" {
			return graph.Predecessor{}, false
		}
		if x.out == r {
			return graph.AfterContainer(r.nodeOfContainer(x.id)), true
		}
	case *Sequence:
		if x == nil || x.tasks == nil {
			return graph.Predecessor{}, false
		}
		return r.predecessorOfLocked(x.tasks)
	case interface{ producer() *Task }:
		if x == nil {
			return graph.Predecessor{}, false
		}
		return r.predecessorOfLocked(x.producer())
	default:
		return graph.Predecessor{}, false
	}
	return graph.Predecessor{}, true
}

func (r *Run) nodeOfTask(id string) *graph.Task {
	if st := r.taskStates[id]; st != nil {
		return st.node
	}
	return nil
}

func (r *Run) nodeOfContainer(id string) *graph.Container {
	if st := r.containerStates[id]; st != nil {
		return st.node
	}
	return nil
}
