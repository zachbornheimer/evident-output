package engine

import (
	"fmt"

	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// DisplayGroup is a handle for a presentation-only collection of independent
// child tasks: state is always derived from children (glyph + header only,
// no Done/Fail/Progress methods), and a group reads Failed iff any child
// failed — no ordering assumed, so concurrent Running children are the
// expected shape (see Sequence for the ordered alternative).
type DisplayGroupHandle struct {
	out *Output
	id  string
}

// Task declares a child task in declaration order. Optional evo.ID sets a
// stable machine key. name is a printf format when args are present
// (fmt.Sprintf semantics) — evo.ID (or any other EntityOption) may be mixed
// into args in any position and still applies.
func (g *DisplayGroupHandle) Task(name string) *TaskHandle {
	g.out.mu.Lock()
	defer g.out.mu.Unlock()
	col := g.out.tasksByRef[g.id]
	if col == nil {
		return &TaskHandle{out: g.out, id: g.out.nextID("task")}
	}
	return g.out.addTaskLocked(txt.Text(name), col, "")
}

// Sequence declares (or, for a repeated name, returns) an ordered child
// container nested under this DisplayGroup (P3's recursive nesting) — see
// Output.Sequence for the get-or-create and cascade contract.
func (g *DisplayGroupHandle) Sequence(name string) *SequenceHandle {
	child := g.declareChild(name, nil, true)
	return &SequenceHandle{tasks: child}
}

// DisplayGroup declares a fresh child container nested under this
// DisplayGroup (P3's recursive nesting) — see Output.DisplayGroup for the
// fan-out contract.
func (g *DisplayGroupHandle) DisplayGroup(name string) *DisplayGroupHandle {
	return g.declareChild(name, nil, false)
}

// declareChild is the shared body behind Sequence/DisplayGroup nesting: it
// resolves this collection's own tasksState, then delegates to
// childContainerGetOrCreateLocked for the get-or-create-vs-always-fresh
// distinction the two container kinds make.
func (g *DisplayGroupHandle) declareChild(name string, args []any, sequential bool) *DisplayGroupHandle {
	if len(args) > 0 {
		name = fmt.Sprintf(name, args...)
	}
	g.out.mu.Lock()
	defer g.out.mu.Unlock()
	parent := g.out.tasksByRef[g.id]
	if parent == nil {
		return &DisplayGroupHandle{out: g.out, id: g.out.nextID("tasks")}
	}
	child := g.out.childContainerGetOrCreateLocked(parent, name, sequential)
	h := &DisplayGroupHandle{out: g.out, id: child.id}
	child.handle = h
	g.out.bumpLocked()
	return h
}

// Summary sets a success-oriented collection summary. text is a printf
// format when args are present (fmt.Sprintf semantics) — one text spelling
// shared with Task/Sequence/Reason (C6).
func (g *DisplayGroupHandle) Summary(text string) *DisplayGroupHandle {
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
func (g *DisplayGroupHandle) Snapshot() TasksSnapshot {
	g.out.mu.Lock()
	defer g.out.mu.Unlock()
	col := g.out.tasksByRef[g.id]
	if col == nil {
		return TasksSnapshot{ID: g.id, State: Empty}
	}
	return col.snapshot()
}
