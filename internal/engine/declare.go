package engine

import (
	"fmt"
	"sync/atomic"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/wire"
)

func (o *Output) nextID(prefix string) string {
	n := atomic.AddUint64(&o.idSeq, 1)
	return fmt.Sprintf("%s_%d", prefix, n)
}

func (o *Output) nextDecl() int {
	o.declSeq++
	return o.declSeq
}

func (o *Output) ensureEntityRoomLocked() error {
	n := len(o.tasks)
	if n >= o.cfg.maxEntities {
		return ErrLimitExceeded
	}
	return nil
}

// Task declares a single operation. Optional evo.ID sets a stable machine
// key. name is a printf format when args are present (fmt.Sprintf
// semantics) — evo.ID (or any other EntityOption) may be mixed into args in
// any position and still applies.
func (o *Output) Task(name string) *TaskHandle {
	return o.taskScoped(name, "")
}

// taskScoped is the declaration path behind Output.Task and Scope.Task. A
// repeated call with the same (scope, name) pair — or a repeated explicit
// evo.ID under any name — is a duplicate sibling declaration (§3.1), never a
// get-or-create: 1.0 removed that idiom because letting two distinct
// declarations silently merge into one identity would make a false
// "already satisfied" possible once identity drives manifest reconciliation.
// A same-name repeat records a Failed task with ProblemCodeDuplicateSiblingName
// (see failDuplicateSiblingLocked); a reused explicit key still reports
// ErrDuplicateKey, its own pre-existing identity-conflict error.
func (o *Output) taskScoped(name, scope string, opts ...EntityOption) *TaskHandle {
	eo := applyEntityOptions(opts)
	clean := declaredName(name)
	key := qualifyKey(scope, eo.key)

	o.mu.Lock()
	defer o.mu.Unlock()

	if key != "" {
		if _, ok := o.taskNameByKey[key]; ok {
			o.recordMisuse(ErrDuplicateKey)
			return o.rejectedTask(ErrDuplicateKey)
		}
	} else if _, ok := o.namedTasks["\x00"+scope+"\x00"+clean]; ok {
		return o.rejectedTask(o.failDuplicateSiblingLocked(nil, kindTask, clean))
	}

	h := o.addTaskLocked(clean, nil, key, scope)
	if h.rejected != nil {
		return h
	}
	if o.namedTasks == nil {
		o.namedTasks = make(map[string]*TaskHandle)
	}
	if key != "" {
		if o.taskNameByKey == nil {
			o.taskNameByKey = make(map[string]string)
		}
		o.taskNameByKey[key] = clean
		o.namedTasks["key:"+key] = h
	} else {
		o.namedTasks["\x00"+scope+"\x00"+clean] = h
	}
	if eo.phase != "" {
		if st := o.taskByRef[h.id]; st != nil && o.ensureOpen() == nil && !core.IsTerminalTask(st.state) {
			o.setPhaseLocked(st, eo.phase)
		}
	}
	return h
}

func (o *Output) addTaskLocked(name string, col *tasksState, key, parentKey string) *TaskHandle {
	h := o.declareTaskLocked(name, col, key, parentKey)
	if _, ok := o.taskByRef[h.id]; ok {
		o.signalLiveLocked(true)
	}
	return h
}

// declareTaskLocked records a child without painting; addTaskLocked
// paints it.
//
// When key is empty, the task's §3.1 stable identity defaults to
// kind+parentKey+normalized-name; an explicit key replaces that derivation
// entirely and is registered instead. parentKey is the declaring parent's
// own stable key (a Group/Sequence's key, or the declaration scope for a
// root-level Task — see Scope).
func (o *Output) declareTaskLocked(name string, col *tasksState, key, parentKey string) *TaskHandle {
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return o.rejectedTask(err)
	}
	effectiveKey := key
	if effectiveKey != "" {
		if _, ok := o.keys[effectiveKey]; ok {
			o.recordMisuse(ErrDuplicateKey)
			return o.rejectedTask(ErrDuplicateKey)
		}
		o.keys[effectiveKey] = struct{}{}
	} else {
		effectiveKey = stableKey(kindTask, parentKey, name)
	}
	if err := o.ensureEntityRoomLocked(); err != nil {
		o.recordMisuse(err)
		return o.rejectedTask(err)
	}
	st := &taskState{
		id:          o.nextID("task"),
		key:         effectiveKey,
		name:        name,
		state:       Pending,
		progress:    Progress{Kind: Indeterminate},
		collection:  col,
		declaration: o.nextDecl(),
		doneCh:      make(chan struct{}),
		resolution:  ResolutionNoWork,
	}
	h := &TaskHandle{out: o, id: st.id}
	st.handle = h
	o.appendTaskLocked(st)
	if col != nil {
		st.sched.preds = append(st.sched.preds, col.nextStepPreds()...)
		col.recordStep(predecessor{task: st})
		col.tasks = append(col.tasks, st)
		tallyDeclaredLocked(st)
	}
	o.taskByRef[st.id] = st
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "task.declared", EntityID: st.id})
	o.emitWireEventLocked(wire.EventTaskDeclared, st.id, map[string]any{"name": name})
	return h
}

// Group declares a collection of independent child tasks. Eligible children
// may overlap through the scheduler. A repeated name is a duplicate sibling
// declaration (§3.1), not a get-or-create — see failDuplicateSiblingLocked.
// name is a printf format when args are present.
func (o *Output) Group(name string) *GroupHandle {
	clean := declaredName(name)
	o.mu.Lock()
	defer o.mu.Unlock()
	if _, ok := o.namedGroupHandles[clean]; ok {
		return o.rejectedGroup(o.failDuplicateSiblingLocked(nil, kindGroup, clean))
	}
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return o.rejectedGroup(err)
	}
	st := o.declareContainerLocked(clean, false)
	o.collections = append(o.collections, st)
	h := &GroupHandle{out: o, id: st.id}
	st.handle = h
	if o.namedGroupHandles == nil {
		o.namedGroupHandles = make(map[string]*GroupHandle)
	}
	o.namedGroupHandles[clean] = h
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "tasks.declared", EntityID: st.id})
	o.emitCollectionDeclaredLocked(st, "")
	return h
}

// Sequence declares a self-managing, ordered container — the front door for
// a sequence of steps that must stop implying "still might run" once a
// member has already failed or been cancelled. A repeated
// evo.Sequence("python") call is a duplicate sibling declaration (§3.1), not
// a get-or-create — see failDuplicateSiblingLocked. name is a printf format
// when args are present (fmt.Sprintf semantics); no args leaves name
// untouched.
func (o *Output) Sequence(name string) *SequenceHandle {
	clean := declaredName(name)
	o.mu.Lock()
	defer o.mu.Unlock()
	if _, ok := o.namedGroups[clean]; ok {
		return &SequenceHandle{tasks: o.rejectedGroup(o.failDuplicateSiblingLocked(nil, kindSequence, clean))}
	}
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return &SequenceHandle{tasks: o.rejectedGroup(err)}
	}
	st := o.declareContainerLocked(clean, true)
	o.collections = append(o.collections, st)
	h := &GroupHandle{out: o, id: st.id}
	st.handle = h
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "tasks.declared", EntityID: st.id})
	o.emitCollectionDeclaredLocked(st, "")
	g := &SequenceHandle{tasks: h}
	if o.namedGroups == nil {
		o.namedGroups = make(map[string]*SequenceHandle)
	}
	o.namedGroups[clean] = g
	return g
}

// declareContainerLocked allocates a new top-level tasksState — the shared
// body behind Group and Sequence, which differ only in the sequential flag.
// name must already be declaredName-normalized — Group and Sequence
// normalize once at declaration entry, before their sibling-dedup check.
func (o *Output) declareContainerLocked(name string, sequential bool) *tasksState {
	st := &tasksState{
		id:          o.nextID("tasks"),
		key:         stableKey(childKindFor(sequential), "", name),
		name:        name,
		declaration: o.nextDecl(),
		sequential:  sequential,
	}
	o.tasksByRef[st.id] = st
	return st
}

// childKindFor names the entity kind a nested container declares, for the
// duplicate-sibling problem it may need to report.
func childKindFor(sequential bool) entityKind {
	if sequential {
		return kindSequence
	}
	return kindGroup
}

// declareChildContainerLocked declares a nested container under parent,
// scoped to this one parent (path + name is the identity). A repeated name
// is a duplicate sibling declaration (§3.1); the caller (GroupHandle/
// SequenceHandle.Group/Sequence) receives the refusal and hands back a
// rejected handle.
func (o *Output) declareChildContainerLocked(parent *tasksState, name string, sequential bool) (*tasksState, error) {
	clean := declaredName(name)
	kind := childKindFor(sequential)
	if _, ok := parent.namedChildren[clean]; ok {
		return nil, o.failDuplicateSiblingLocked(parent, kind, clean)
	}
	st := &tasksState{
		id:          o.nextID("tasks"),
		key:         stableKey(kind, parent.key, clean),
		name:        clean,
		declaration: o.nextDecl(),
		sequential:  sequential,
		parent:      parent,
		entry:       parent.nextStepPreds(),
	}
	parent.recordStep(predecessor{col: st})
	o.tasksByRef[st.id] = st
	parent.children = append(parent.children, st)
	if parent.namedChildren == nil {
		parent.namedChildren = make(map[string]*tasksState)
	}
	parent.namedChildren[clean] = st
	o.emitCollectionDeclaredLocked(st, parent.id)
	return st, nil
}

// declareGroupTask declares a child task by name in the container g — the
// identity behind Group.Task/Sequence.Task. A repeated name is a duplicate
// sibling declaration (§3.1), not a get-or-create. A task declared under a
// refused container is refused for the same reason.
func (o *Output) declareGroupTask(g *GroupHandle, name string, opts ...EntityOption) *TaskHandle {
	clean := declaredName(name)
	o.mu.Lock()
	defer o.mu.Unlock()
	col := o.tasksByRef[g.id]
	if col == nil {
		return o.rejectedTask(g.rejected)
	}
	if _, ok := col.namedTasks[clean]; ok {
		return o.rejectedTask(o.failDuplicateSiblingLocked(col, kindTask, clean))
	}
	eo := applyEntityOptions(opts)
	h := o.addTaskLocked(clean, col, eo.key, col.key)
	if h.rejected != nil {
		return h
	}
	if col.namedTasks == nil {
		col.namedTasks = make(map[string]*TaskHandle)
	}
	col.namedTasks[clean] = h
	return h
}
