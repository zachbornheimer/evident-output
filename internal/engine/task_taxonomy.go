package engine

import (
	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
	"github.com/zachbornheimer/evident-output/internal/wire"
)

// Skipped accumulates a (reason, name) skip record on the task, with an
// optional trailing errs for evidence of why. It returns nothing —
// accumulating a record is an act, not a value to chain — is usable before
// the task resolves, and does not itself resolve the task. The taxonomy line
// ("- skipped N (a reasonA, b reasonB)") is derived from every accumulated
// record at render time, so no caller can hand-build (and thereby miscount)
// the summary; the aggregation key is untouched by errs. Any errs render as
// one bounded evidence line under the count row (first cause + "(+N more)"),
// full list under Verbose.
//
// Skipped is the only disposition: a per-candidate Task that policy excludes
// is intentionally not executed, so it is Skipped. A count such as "kept
// 383" is domain information (a Fact or part of the Summary), not a second
// outcome (contract Vocabulary, §13).
func (t *TaskHandle) Skipped(reason TaxonomyReason) {
	t.recordSkip(reason, "", nil)
	t.finish(Skipped, "", nil)
}

func (t *TaskHandle) recordSkip(reason TaxonomyReason, name string, errs []error) {
	t.withTask(func(st *taskState) { t.recordSkipLocked(st, reason, name, errs) })
}

func (t *TaskHandle) recordSkipLocked(st *taskState, reason TaxonomyReason, name string, errs []error) {
	if err := t.out.ensureOpen(); err != nil {
		t.out.recordMisuse(err)
		return
	}
	if core.IsTerminalTask(st.state) {
		t.out.recordMisuseFor(st.name, ErrAlreadyResolved)
		return
	}
	t.out.enforceReasonConstraintLocked(reason, st.name)
	if name == "" {
		name = st.name
	}
	rec := TaxonomyRecord{Reason: reason.name, Name: txt.Text(name), Causes: causesFromErrors(errs)}
	st.skipped = append(st.skipped, rec)
	t.out.bumpLocked()
	t.out.appendEventLocked(Event{Type: "task.skip_recorded", EntityID: t.id})
	t.out.emitWireEventLocked(wire.EventDispositionRecorded, t.id, wire.ToDispositionDoc(rec).EventPayload())
}

// causesFromErrors renders each non-nil err's text, sanitized like every
// other human-visible taxonomy field, for TaxonomyRecord.Causes.
func causesFromErrors(errs []error) []string {
	if len(errs) == 0 {
		return nil
	}
	var out []string
	for _, err := range errs {
		if err == nil {
			continue
		}
		out = append(out, txt.Text(err.Error()))
	}
	return out
}

// enforceReasonConstraintLocked records misuse when reason's declared
// constraint (OnTask) doesn't match the Task recording it.
// Strict panics via recordMisuse; production still counts the record —
// a constraint violation degrades to "counted anyway", never a dropped truth.
func (o *Output) enforceReasonConstraintLocked(reason TaxonomyReason, taskName string) {
	if reason.onTask != "" && reason.onTask != taskName {
		o.recordMisuse(ErrReasonWrongTask)
	}
}
