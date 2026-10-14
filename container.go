package evo

import "github.com/zachbornheimer/evident-output/v2/internal/graph"

// After declares predecessors: this Group declares children only once all succeeded.
// Call it before Define and before declaring any child; later is misuse.
func (g *Group) After(preds ...Predecessor) *Group {
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
	o.graph.ContainerAfter(col.node, o.predecessorsOfLocked(preds)...)
	return g
}

// After is Group.After for a Sequence.
func (s *Sequence) After(preds ...Predecessor) *Sequence {
	if s == nil || s.tasks == nil {
		return s
	}
	s.tasks.After(preds...)
	return s
}

// define defers this Group's children: build runs once all After predecessors succeeded,
// and is topology only (no context, no Wait). If one fails, build never runs; the Group settles NotStarted.
func (g *Group) define(build func(*Group)) {
	if build == nil {
		g.recordInvalid()
		return
	}
	g.defineBuilder(func() { build(g) })
}

func (s *Sequence) define(build func(*Sequence)) {
	if s == nil || s.tasks == nil {
		return
	}
	if build == nil {
		s.tasks.recordInvalid()
		return
	}
	s.tasks.defineBuilder(func() { build(s) })
}

func (g *Group) recordInvalid() {
	if g != nil && g.out != nil {
		g.out.recordMisuse(ErrInvalidConfig)
	}
}

func (g *Group) defineBuilder(run func()) {
	if g == nil || g.out == nil {
		return
	}
	o := g.out
	o.mu.Lock()
	col := o.containerStates[g.id]
	o.mu.Unlock()
	if col == nil {
		o.recordMisuse(g.rejected)
		return
	}
	o.graph.DefineBuilder(col.node, run)
}

// Task declares a single root-level operation named name. A repeated name is a
// duplicate sibling declaration (§3.1), never a get-or-create.
func (o *Run) Task(name string) *Task {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.taskHandle(o.graph.DeclareTask(nil, name))
}

// Group declares independent child tasks that may overlap; a repeated name is a duplicate (§3.1).
func (o *Run) Group(name string) *Group {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.groupHandle(o.graph.DeclareContainer(nil, name, false))
}

// Sequence declares an ordered container: once a member fails or is cancelled,
// later members never start. A repeated name is a duplicate sibling declaration (§3.1).
func (o *Run) Sequence(name string) *Sequence {
	o.mu.Lock()
	defer o.mu.Unlock()
	return &Sequence{tasks: o.groupHandle(o.graph.DeclareContainer(nil, name, true))}
}

// taskHandle is the declared Task, or a refused handle keeping err. Callers hold o.mu.
func (o *Run) taskHandle(node *graph.Task, err error) *Task {
	if err != nil {
		return o.rejectedTask(err)
	}
	return o.taskStates[node.ID].handle
}

func (o *Run) groupHandle(node *graph.Container, err error) *Group {
	if err != nil {
		return o.rejectedGroup(err)
	}
	return o.containerStates[node.ID].handle
}

// Container is a node that can declare work beneath it: the run, a Group or a Sequence.
type Container interface {
	Task(name string) *Task
	Group(name string) *Group
	Sequence(name string) *Sequence
}

var (
	_ Container = (*Run)(nil)
	_ Container = (*Group)(nil)
	_ Container = (*Sequence)(nil)
)

// Group is the front door for independent child work; eligible children may overlap.
type Group struct {
	out *Run
	id  string
	// rejected is why the declaration was refused; nil for a declared Group or Sequence.
	rejected error
}

// Task declares a child task. A repeated name fails with ProblemCodeDuplicateSiblingName (§3.1),
// so keep the first handle if you reference the Task again (for example in After).
func (g *Group) Task(name string) *Task {
	if g == nil || g.out == nil {
		return &Task{}
	}
	return g.out.declareChildTask(g, name)
}

func (o *Run) declareChildTask(g *Group, name string) *Task {
	o.mu.Lock()
	defer o.mu.Unlock()
	col := o.containerStates[g.id]
	if col == nil {
		return o.rejectedTask(g.rejected)
	}
	return o.taskHandle(o.graph.DeclareTask(col.node, name))
}

// Sequence declares (or returns) an ordered child container nested here.
func (g *Group) Sequence(name string) *Sequence {
	child := g.declareChild(name, true)
	return &Sequence{tasks: child}
}

// Group declares (or returns) an independent child container nested here.
func (g *Group) Group(name string) *Group {
	return g.declareChild(name, false)
}

func (g *Group) declareChild(name string, sequential bool) *Group {
	if g == nil || g.out == nil {
		return &Group{}
	}
	g.out.mu.Lock()
	defer g.out.mu.Unlock()
	parent := g.out.containerStates[g.id]
	if parent == nil {
		return g.out.rejectedGroup(g.rejected)
	}
	return g.out.groupHandle(g.out.graph.DeclareContainer(parent.node, name, sequential))
}

// Summary sets a success-oriented collection summary.
func (g *Group) Summary(text string) *Group {
	if g == nil || g.out == nil {
		return g
	}
	g.out.mu.Lock()
	defer g.out.mu.Unlock()
	col := g.out.containerStates[g.id]
	if col == nil {
		return g
	}
	if err := g.out.ensureOpen(); err != nil {
		g.out.recordMisuse(err)
		return g
	}
	col.node.Summarize(text)
	g.out.bumpLocked()
	g.out.appendEventLocked(recEvent{Type: "tasks.summary_set", EntityID: g.id})
	return g
}

// Sequence is the front door for ordered child work: Group's scheduler with implicit
// predecessor edges. A failed, blocked or cancelled child makes later siblings NotStarted.
type Sequence struct {
	tasks *Group
}

// Task declares a child Task that runs after the Sequence's earlier children settle. Sibling names are unique
// per parent: declaring a name the Sequence already holds is misuse and returns a Task that never runs.
func (g *Sequence) Task(name string) *Task {
	if g == nil || g.tasks == nil {
		return &Task{}
	}
	return g.tasks.out.declareChildTask(g.tasks, name)
}

// Summary sets a success-oriented sequence summary.
func (g *Sequence) Summary(text string) *Sequence {
	if g == nil || g.tasks == nil {
		return g
	}
	g.tasks.Summary(text)
	return g
}

// Sequence declares (or returns) an ordered child container nested here.
func (g *Sequence) Sequence(name string) *Sequence {
	if g == nil || g.tasks == nil {
		return &Sequence{}
	}
	return g.tasks.Sequence(name)
}

// Group declares (or returns) an independent child container nested here.
func (g *Sequence) Group(name string) *Group {
	if g == nil || g.tasks == nil {
		return &Group{}
	}
	return g.tasks.Group(name)
}

func (g *Group) Build(fn func(*Group)) { g.define(fn) }

func (s *Sequence) Build(fn func(*Sequence)) { s.define(fn) }
