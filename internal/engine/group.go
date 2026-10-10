package engine

// GroupHandle is the front door for independent child work. Eligible
// children may overlap through the scheduler. The type cannot be named
// Group — that name is the constructor. Nest via Group/Sequence.
type GroupHandle struct {
	out *Output
	id  string
	// rejected is why the declaration was refused (see rejectedGroup); nil
	// for a declared Group or Sequence.
	rejected error
	// facade holds this handle's public wrapper (see FacadeSlot).
	facade FacadeSlot
}

// Task declares a child task. A repeated name is a duplicate sibling
// declaration, not a get-or-create (§3.1, Output.Task's doc comment): it
// fails the new call with ProblemCodeDuplicateSiblingName rather than
// returning the earlier handle. Callers that reference a Task again later
// (for example in After) must keep the first handle, typically in a typed
// variable, instead of re-declaring by name.
func (g *GroupHandle) Task(name string) *TaskHandle {
	if g == nil || g.out == nil {
		return &TaskHandle{}
	}
	return g.out.declareGroupTask(g, name)
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
	parent := g.out.containerStates[g.id]
	if parent == nil {
		return g.out.rejectedGroup(g.rejected)
	}
	return g.out.declareContainerLocked(parent, name, sequential)
}

// Summary sets a success-oriented collection summary.
func (g *GroupHandle) Summary(text string) *GroupHandle {
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
	col.rec.SetSummary(text)
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
	col := g.out.containerStates[g.id]
	if col == nil {
		return TasksSnapshot{ID: g.id, State: Empty}
	}
	return col.snapshot()
}
