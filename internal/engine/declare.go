package engine

import (
	"github.com/zachbornheimer/evident-output/internal/graph"
	"github.com/zachbornheimer/evident-output/internal/record"
	"github.com/zachbornheimer/evident-output/internal/wire"
)

func (o *Output) ensureEntityRoomLocked() error {
	n := len(o.tasks)
	if n >= o.cfg.maxEntities {
		return ErrLimitExceeded
	}
	return nil
}

// Task declares a single root-level operation named name. Identity
// overrides go through TaskHandle.Key. A repeated name is a duplicate
// sibling declaration (§3.1), never a get-or-create: 1.0 removed that idiom
// because letting two distinct declarations silently merge into one
// identity would make a false "already satisfied" possible once identity
// drives manifest reconciliation. The repeat records a Failed task with
// ProblemCodeDuplicateSiblingName (see failDuplicateSiblingLocked).
func (o *Output) Task(name string) *TaskHandle {
	clean := graph.DeclaredName(name)
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.graph.NameTaken(nil, graph.KindTask, clean) {
		return o.rejectedTask(o.failDuplicateSiblingLocked(nil, graph.KindTask, clean))
	}
	h := o.addTaskLocked(clean, nil)
	if h.rejected == nil {
		o.graph.ClaimName(nil, graph.KindTask, clean)
	}
	return h
}

func (o *Output) addTaskLocked(name string, col *tasksState) *TaskHandle {
	h := o.declareTaskLocked(name, col)
	if _, ok := o.taskStates[h.id]; ok {
		o.signalLiveLocked(true)
	}
	return h
}

// declareTaskLocked records a child without painting; addTaskLocked
// paints it.
//
// The task's §3.1 stable identity is kind + the declaring parent's key
// (parentKeyOf) + name, for every declaration, a refused duplicate's row
// included; TaskHandle.Key replaces it later.
func (o *Output) declareTaskLocked(name string, col *tasksState) *TaskHandle {
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return o.rejectedTask(err)
	}
	if o.declaredInCallback() {
		o.recordMisuse(ErrDeclaredInCallback)
		return o.rejectedTask(ErrDeclaredInCallback)
	}
	if err := o.ensureEntityRoomLocked(); err != nil {
		o.recordMisuse(err)
		return o.rejectedTask(err)
	}
	node := o.graph.AddTask(containerNode(col), name, record.TaskInit{
		State: Pending, Progress: Progress{Kind: Indeterminate}, Resolution: ResolutionNoWork,
	})
	st := &taskState{
		id:          node.ID,
		node:        node,
		name:        node.Name,
		rec:         node.Rec,
		collection:  col,
		declaration: node.Declaration,
		doneCh:      make(chan struct{}),
	}
	h := &TaskHandle{out: o, id: st.id}
	st.handle = h
	o.appendTaskLocked(st)
	if col != nil {
		st.sched.preds = o.appendStepPredsLocked(st.sched.preds, col)
		st.sched.preds = o.joinPassedSequencesLocked(st.sched.preds, col, predecessor{task: st})
		col.recordStep(predecessor{task: st})
		st.filing.pos = len(col.tasks)
		col.tasks = append(col.tasks, st)
		st.markFiling()
		col.hasNamesake = col.hasNamesake || name == col.name
		tallyDeclaredLocked(st)
		st.censusDeclared()
	}
	o.taskStates[st.id] = st
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "task.declared", EntityID: st.id})
	o.emitWireEventLocked(wire.EventTaskDeclared, st.id, map[string]any{"name": name})
	if col != nil {
		o.stopIfFollowerLocked(st)
	}
	return h
}

// Group declares a collection of independent child tasks. Eligible children
// may overlap through the scheduler. A repeated name is a duplicate sibling
// declaration (§3.1), not a get-or-create — see failDuplicateSiblingLocked.
func (o *Output) Group(name string) *GroupHandle {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.declareContainerLocked(nil, name, false)
}

// Sequence declares a self-managing, ordered container — the front door for
// a sequence of steps that must stop implying "still might run" once a
// member has already failed or been cancelled. A repeated name is a
// duplicate sibling declaration (§3.1), not a get-or-create — see
// failDuplicateSiblingLocked.
func (o *Output) Sequence(name string) *SequenceHandle {
	o.mu.Lock()
	defer o.mu.Unlock()
	return &SequenceHandle{tasks: o.declareContainerLocked(nil, name, true)}
}

// declareContainerLocked declares a Group (sequential false) or Sequence
// under parent, or at the root when parent is nil — the one path every
// level shares, so one rule decides duplicates everywhere (§3.1). A nested
// container's identity is its parent's key plus its name, and it starts
// after the step before it when parent is a Sequence. A refusal comes back
// as a rejected handle. Callers must already hold o.mu.
func (o *Output) declareContainerLocked(parent *tasksState, name string, sequential bool) *GroupHandle {
	clean := graph.DeclaredName(name)
	kind := graph.ContainerKind(sequential)
	if o.graph.NameTaken(containerNode(parent), kind, clean) {
		return o.rejectedGroup(o.failDuplicateSiblingLocked(parent, kind, clean))
	}
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return o.rejectedGroup(err)
	}
	if o.declaredInCallback() {
		o.recordMisuse(ErrDeclaredInCallback)
		return o.rejectedGroup(ErrDeclaredInCallback)
	}
	node := o.graph.AddContainer(containerNode(parent), clean, sequential)
	st := &tasksState{
		id:          node.ID,
		node:        node,
		name:        node.Name,
		rec:         node.Rec,
		declaration: node.Declaration,
		sequential:  sequential,
		parent:      parent,
	}
	parentID := ""
	if parent == nil {
		o.collections = append(o.collections, st)
	} else {
		parentID = parent.id
		st.entry = o.appendStepPredsLocked(nil, parent)
		st.entry = o.joinPassedSequencesLocked(st.entry, parent, predecessor{col: st})
		parent.recordStep(predecessor{col: st})
		parent.children = append(parent.children, st)
	}
	o.containerStates[st.id] = st
	o.graph.ClaimName(containerNode(parent), kind, clean)
	h := &GroupHandle{out: o, id: st.id}
	st.handle = h
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "tasks.declared", EntityID: st.id})
	o.emitCollectionDeclaredLocked(st, parentID)
	return h
}

// declareGroupTask declares a child task by name in the container g — the
// identity behind Group.Task/Sequence.Task. A repeated name is a duplicate
// sibling declaration (§3.1), not a get-or-create. A task declared under a
// refused container is refused for the same reason.
func (o *Output) declareGroupTask(g *GroupHandle, name string) *TaskHandle {
	clean := graph.DeclaredName(name)
	o.mu.Lock()
	defer o.mu.Unlock()
	col := o.containerStates[g.id]
	if col == nil {
		return o.rejectedTask(g.rejected)
	}
	if o.graph.NameTaken(col.node, graph.KindTask, clean) {
		return o.rejectedTask(o.failDuplicateSiblingLocked(col, graph.KindTask, clean))
	}
	h := o.addTaskLocked(clean, col)
	if h.rejected == nil {
		o.graph.ClaimName(col.node, graph.KindTask, clean)
	}
	return h
}
