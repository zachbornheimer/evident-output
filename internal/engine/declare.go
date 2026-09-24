package engine

import (
	"fmt"
	"sync/atomic"

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

// Task declares a single operation named name. Identity overrides go
// through TaskHandle.Key.
func (o *Output) Task(name string) *TaskHandle {
	return o.taskScoped(name, "", "")
}

// taskScoped is the declaration path behind Output.Task and Scope.Task;
// key is an explicit stable key (tests only), or "" for the default. A
// repeated call with the same (scope, name) pair — or a repeated explicit
// key under any name — is a duplicate sibling declaration (§3.1), never a
// get-or-create: 1.0 removed that idiom because letting two distinct
// declarations silently merge into one identity would make a false
// "already satisfied" possible once identity drives manifest reconciliation.
// A same-name repeat records a Failed task with ProblemCodeDuplicateSiblingName
// (see failDuplicateSiblingLocked); a reused explicit key still reports
// ErrDuplicateKey, its own pre-existing identity-conflict error.
func (o *Output) taskScoped(name, scope, explicitKey string) *TaskHandle {
	clean := declaredName(name)
	key := qualifyKey(scope, explicitKey)

	o.mu.Lock()
	defer o.mu.Unlock()

	if key != "" {
		// An explicit key is the identity: declareTaskLocked refuses a
		// reused one (ErrDuplicateKey), and the name claims nothing.
		return o.addTaskLocked(clean, nil, key, scope)
	}
	names := o.siblingsLocked(nil, scope)
	if names.taskTaken(clean) {
		return o.rejectedTask(o.failDuplicateSiblingLocked(nil, kindTask, clean))
	}
	h := o.addTaskLocked(clean, nil, "", scope)
	if h.rejected == nil {
		names.claimTask(clean)
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
	clean := declaredName(name)
	kind := childKindFor(sequential)
	names := o.siblingsLocked(parent, "")
	if names.containerTaken(clean) {
		return o.rejectedGroup(o.failDuplicateSiblingLocked(parent, kind, clean))
	}
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return o.rejectedGroup(err)
	}
	st := &tasksState{
		id:          o.nextID("tasks"),
		key:         stableKey(kind, parentKeyOf(parent), clean),
		name:        clean,
		declaration: o.nextDecl(),
		sequential:  sequential,
		parent:      parent,
	}
	parentID := ""
	if parent == nil {
		o.collections = append(o.collections, st)
	} else {
		parentID = parent.id
		st.entry = parent.nextStepPreds()
		parent.recordStep(predecessor{col: st})
		parent.children = append(parent.children, st)
	}
	o.tasksByRef[st.id] = st
	names.claimContainer(clean)
	h := &GroupHandle{out: o, id: st.id}
	st.handle = h
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "tasks.declared", EntityID: st.id})
	o.emitCollectionDeclaredLocked(st, parentID)
	return h
}

// childKindFor names the entity kind a nested container declares, for the
// duplicate-sibling problem it may need to report.
func childKindFor(sequential bool) entityKind {
	if sequential {
		return kindSequence
	}
	return kindGroup
}

// declareGroupTask declares a child task by name in the container g — the
// identity behind Group.Task/Sequence.Task. A repeated name is a duplicate
// sibling declaration (§3.1), not a get-or-create. A task declared under a
// refused container is refused for the same reason.
func (o *Output) declareGroupTask(g *GroupHandle, name string) *TaskHandle {
	clean := declaredName(name)
	o.mu.Lock()
	defer o.mu.Unlock()
	col := o.tasksByRef[g.id]
	if col == nil {
		return o.rejectedTask(g.rejected)
	}
	if col.names.taskTaken(clean) {
		return o.rejectedTask(o.failDuplicateSiblingLocked(col, kindTask, clean))
	}
	h := o.addTaskLocked(clean, col, "", col.key)
	if h.rejected == nil {
		col.names.claimTask(clean)
	}
	return h
}
