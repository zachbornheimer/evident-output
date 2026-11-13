package evo

import (
	"iter"

	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// GroupHandle is the front door for independent child work. Eligible
// children may overlap through the scheduler. The type cannot be named
// Group — that name is the constructor. Nest via Group/Sequence.
type GroupHandle struct {
	out *Output
	id  string
}

// Task declares (or, for a repeated name, returns) a child task. Explicit
// Task children stay individually visible; Each children aggregate.
func (g *GroupHandle) Task(name string) *TaskHandle {
	if g == nil || g.out == nil {
		return &TaskHandle{}
	}
	return g.out.groupTaskGetOrCreate(g.id, name)
}

// Sequence declares (or returns) an ordered child container nested here.
func (g *GroupHandle) Sequence(name string) *SequenceHandle {
	child := g.declareChild(name, true)
	return &SequenceHandle{tasks: child}
}

// Group declares (or returns) an independent child container nested here.
func (g *GroupHandle) Group(name string) *GroupHandle {
	return g.declareChild(name, false)
}

func (g *GroupHandle) declareChild(name string, sequential bool) *GroupHandle {
	if g == nil || g.out == nil {
		return &GroupHandle{}
	}
	g.out.mu.Lock()
	defer g.out.mu.Unlock()
	parent := g.out.tasksByRef[g.id]
	if parent == nil {
		return &GroupHandle{out: g.out, id: g.out.nextID("tasks")}
	}
	child := g.out.childContainerGetOrCreateLocked(parent, name, sequential)
	h := &GroupHandle{out: g.out, id: child.id}
	child.handle = h
	g.out.bumpLocked()
	return h
}

// Summary sets a success-oriented collection summary.
func (g *GroupHandle) Summary(text string) *GroupHandle {
	if g == nil || g.out == nil {
		return g
	}
	g.out.mu.Lock()
	defer g.out.mu.Unlock()
	col := g.out.tasksByRef[g.id]
	if col == nil {
		return g
	}
	if err := g.out.ensureOpen(); err != nil {
		g.out.recordMisuse(err)
		return g
	}
	col.summary = txt.Text(text)
	g.out.bumpLocked()
	g.out.appendEventLocked(Event{Type: "tasks.summary_set", EntityID: g.id})
	return g
}

// Snapshot returns the collection snapshot with derived state.
func (g *GroupHandle) Snapshot() TasksSnapshot {
	if g == nil || g.out == nil {
		return TasksSnapshot{State: Empty}
	}
	g.out.mu.Lock()
	defer g.out.mu.Unlock()
	col := g.out.tasksByRef[g.id]
	if col == nil {
		return TasksSnapshot{ID: g.id, State: Empty}
	}
	return col.snapshot()
}

// Each yields a child Task per item. Range-end waits only for children that
// received Define or a mutation verb during the loop.
func (g *GroupHandle) Each(items []string) iter.Seq2[string, *TaskHandle] {
	if g == nil || g.out == nil {
		return func(func(string, *TaskHandle) bool) {}
	}
	return eachChildren(g.out, g.id, items)
}
