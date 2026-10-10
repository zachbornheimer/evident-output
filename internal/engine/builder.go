package engine

import (
	"github.com/zachbornheimer/evident-output/internal/graph"
)

// After declares predecessors: this Group declares its children only once
// every one of them succeeded, and never does once one of them cannot. A
// predecessor may be a Task, a Computed producer, or a Group or Sequence.
// Call it before Define and before declaring any child; later is misuse.
func (g *GroupHandle) After(preds ...any) *GroupHandle {
	if g == nil || g.out == nil {
		return g
	}
	o := g.out
	o.mu.Lock()
	defer o.mu.Unlock()
	col := o.containerStates[g.id]
	if col == nil {
		return g
	}
	if !o.graph.AddContainerAfter(col.node, o.predecessorsOfLocked(preds)...) {
		o.recordMisuse(ErrInvalidConfig)
	}
	return g
}

// After is GroupHandle.After for a Sequence.
func (s *SequenceHandle) After(preds ...any) *SequenceHandle {
	if s == nil || s.tasks == nil {
		return s
	}
	s.tasks.After(preds...)
	return s
}

// Define defers this Group's children: build runs once, when every After
// predecessor succeeded, and declares the children through the handle it
// receives. build is topology only. It returns nothing, takes no context,
// and must not Wait. Children it declares run under the Group's normal
// semantics. When a predecessor fails, build never runs and the Group
// settles NotStarted.
func (g *GroupHandle) Define(build func(*GroupHandle)) {
	if build == nil {
		g.recordInvalid()
		return
	}
	g.defineBuilder(func() { build(g) })
}

// Define is GroupHandle.Define for a Sequence: build declares ordered steps.
func (s *SequenceHandle) Define(build func(*SequenceHandle)) {
	if s == nil || s.tasks == nil {
		return
	}
	if build == nil {
		s.tasks.recordInvalid()
		return
	}
	s.tasks.defineBuilder(func() { build(s) })
}

func (g *GroupHandle) recordInvalid() {
	if g != nil && g.out != nil {
		g.out.recordMisuse(ErrInvalidConfig)
	}
}

func (g *GroupHandle) defineBuilder(run func()) {
	if g == nil || g.out == nil {
		return
	}
	o := g.out
	o.mu.Lock()
	col := o.containerStates[g.id]
	switch {
	case col == nil:
		o.recordMisuse(g.rejected)
		o.mu.Unlock()
		return
	case o.ensureOpen() != nil:
		o.recordMisuse(ErrClosed)
		o.mu.Unlock()
		return
	}
	node := o.graph.AddGate(col.node, func() error { run(); return nil })
	if node == nil {
		o.recordMisuse(ErrInvalidConfig)
		o.mu.Unlock()
		return
	}
	gate := &taskState{id: node.ID, node: node, name: node.Name, followed: declaredState}
	gate.handle = &TaskHandle{out: o, id: gate.id}
	o.taskStates[gate.id] = gate
	o.graph.Submit(node, graph.Work{})
	o.followRecordLocked()
	o.mu.Unlock()
	o.graph.Kick()
}

// declaredInCallback reports whether the calling goroutine is inside a Task
// callback that is not running a topology builder. Declaring from a
// callback is misuse: declare it from a builder, or before the run.
func (o *Output) declaredInCallback() bool {
	if o.graph.Executing() == 0 {
		return false
	}
	m := graph.ReadStackMarks()
	return m.Callbacks > m.Builders
}
