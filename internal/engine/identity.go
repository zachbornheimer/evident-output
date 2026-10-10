package engine

import (
	"fmt"

	"github.com/zachbornheimer/evident-output/internal/graph"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// Problem codes are stable, machine-readable Problem.Code values —
// consumers match on these instead of parsing Summary text, which is
// presentation and may be reworded.
const (
	// ProblemCodeDuplicateSiblingName marks a Task/Group/Sequence declared
	// with a name already used by another child of the same parent (§3.1).
	// get-or-create was removed for exactly this reason in 1.0: letting two
	// distinct declarations silently merge into one identity would make a
	// false "already satisfied" possible once identity drives manifest
	// reconciliation, so declaration fails instead of returning an
	// ambiguous handle.
	ProblemCodeDuplicateSiblingName = "duplicate-sibling-name"
	// ProblemCodeVerificationUnsatisfied marks a Task whose post-Define
	// Verify observed that the desired state was not reached (§9.1): the
	// callback ran but its intended effect could not be confirmed.
	ProblemCodeVerificationUnsatisfied = "verification-unsatisfied"
)

// containerNode is col's declaration in the graph, nil for the root (col == nil).
func containerNode(col *tasksState) *graph.Container {
	if col == nil {
		return nil
	}
	return col.node
}

// Key sets an advanced, refactor/rename-stable override for this Task's
// §3.1 identity, replacing the default kind+parent-key+name derivation
// within the application/workspace manifest namespace. Must be called
// before Define — dependency/verification/execution configuration freezes
// at Define, and identity is part of that configuration — a call after
// Define, or after a terminal verb settled the Task, records
// ErrKeyAfterDefine and leaves the task's key untouched. Repeating the key
// the Task already has is a no-op. A key already claimed by another Task is
// ErrDuplicateKey, the same identity-conflict error every other
// explicit-key path already reports.
func (t *TaskHandle) Key(key string) *TaskHandle {
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
	if !st.neverDefined() {
		o.recordMisuseFor(st.name, ErrKeyAfterDefine)
		return t
	}
	if o.graph.Rekey(st.node, txt.Text(key)) == graph.KeyTaken {
		o.recordMisuse(ErrDuplicateKey)
	}
	return t
}

// failDuplicateSiblingLocked records a real, visible Failed task carrying
// ProblemCodeDuplicateSiblingName — the truthful conclusion/exit-code path
// a duplicate declaration now takes instead of a panic or a silently
// returned existing handle. The row is named for the duplicated name
// itself, and its one Problem says what is wrong with it, so the reader
// sees `✗ t  duplicate task name` once. col is the parent container the
// duplicate was declared under, or nil for a root-level declaration.
// Callers must already hold o.mu. It returns the refusal the duplicate's
// rejected handle keeps.
func (o *Output) failDuplicateSiblingLocked(col *tasksState, kind graph.EntityKind, name string) error {
	h := o.addTaskLocked(name, col)
	rejected := fmt.Errorf("%w: %s", ErrDuplicateSiblingName, name)
	st := o.taskStates[h.id]
	if st == nil {
		return rejected
	}
	st.rec.AppendProblems(Problem{
		Code:    ProblemCodeDuplicateSiblingName,
		Summary: fmt.Sprintf("duplicate %s name", kind),
	})
	o.settleLocked(st, Failed)
	o.recordMisuseFor(name, ErrDuplicateSiblingName)
	return rejected
}
