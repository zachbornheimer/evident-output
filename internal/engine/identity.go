package engine

import (
	"fmt"

	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// entityKind names the three declarable entity kinds §3.1's default stable
// key is built from (kind + parent stable key + normalized name). It is
// spelled out here, not reused from any presentation type, because identity
// and presentation are different concerns even though today they use the
// same words.
type entityKind string

const (
	kindTask     entityKind = "task"
	kindGroup    entityKind = "group"
	kindSequence entityKind = "sequence"
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

// declaredName is the single normalization every Task/Group/Sequence
// declaration path must apply to a caller-supplied name before using it for
// anything identity-related — sibling-dedup lookups and stableKey's own
// name segment alike. Two call sites normalizing separately (or one
// normalizing and one keying off the raw string) can disagree: a raw name
// carrying a control character evident-output strips on presentation
// (txt.Text) would then dedup-check under a different string than the one
// its stable key derives from, letting "deploy\x01" and "deploy\x02" both
// register as distinct siblings of the same rendered name "deploy".
func declaredName(name string) string {
	return txt.Text(name)
}

// stableKey computes §3.1's default identity: entity kind + parent stable
// key + normalized entity name. The application/workspace manifest
// namespace already supplies application identity, so it is not duplicated
// into every key here. name must already be declaredName-normalized —
// every caller in this package normalizes once, at declaration entry.
func stableKey(kind entityKind, parentKey, name string) string {
	return fmt.Sprintf("%s:%s/%s", kind, parentKey, name)
}

// parentKeyOf returns col's own stable key, or "" for a root-level
// declaration (col == nil).
func parentKeyOf(col *tasksState) string {
	if col == nil {
		return ""
	}
	return col.key
}

// Key sets an advanced, refactor/rename-stable override for this Task's
// §3.1 identity, replacing the default kind+parent-key+name derivation
// within the application/workspace manifest namespace. Must be called
// before Define — dependency/verification/execution configuration freezes
// at Define, and identity is part of that configuration — a call after
// Define records ErrKeyAfterDefine and leaves the task's key untouched. A
// key already claimed by another Task is ErrDuplicateKey, the same
// identity-conflict error every other explicit-key path already reports.
func (t *TaskHandle) Key(key string) *TaskHandle {
	if t == nil || t.out == nil {
		return t
	}
	o := t.out
	o.mu.Lock()
	defer o.mu.Unlock()
	st := o.taskByRef[t.id]
	if st == nil {
		return t
	}
	if st.submitted {
		o.recordMisuseFor(st.name, ErrKeyAfterDefine)
		return t
	}
	clean := txt.Text(key)
	if _, ok := o.keys[clean]; ok {
		o.recordMisuse(ErrDuplicateKey)
		return t
	}
	delete(o.keys, st.key)
	o.keys[clean] = struct{}{}
	st.key = clean
	return t
}

// failDuplicateSiblingLocked records a real, visible Failed task carrying
// ProblemCodeDuplicateSiblingName — the truthful conclusion/exit-code path
// a duplicate declaration now takes instead of a panic or a silently
// returned existing handle. col is the parent container the duplicate was
// declared under, or nil for a root-level declaration. Callers must already
// hold o.mu.
func (o *Output) failDuplicateSiblingLocked(col *tasksState, kind entityKind, name string) {
	summary := fmt.Sprintf("duplicate %s name: %s", kind, name)
	h := o.addTaskLocked(summary, col, "", parentKeyOf(col), false)
	st := o.taskByRef[h.id]
	if st == nil {
		return
	}
	st.state = Failed
	st.summary = txt.Text(summary)
	st.problems = core.StoreProblems([]Problem{{
		Code:    ProblemCodeDuplicateSiblingName,
		Subject: name,
		Summary: summary,
	}})
	st.closeDoneLocked()
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "task." + string(Failed), EntityID: st.id})
	o.recordMisuseFor(name, ErrDuplicateSiblingName)
}
