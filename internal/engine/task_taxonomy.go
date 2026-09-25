package engine

import (
	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
	"github.com/zachbornheimer/evident-output/internal/wire"
)

// dispositionVerb names which accumulation act a Reason's usage constraints
// are checked against — TaskHandle.Skipped or TaskHandle.Kept.
type dispositionVerb string

const (
	dispositionSkip dispositionVerb = "skip"
	dispositionKeep dispositionVerb = "keep"
)

// wireName is v's record kind on the machine wire.
func (v dispositionVerb) wireName() string {
	if v == dispositionSkip {
		return wire.DispositionSkipped
	}
	return wire.DispositionKept
}

// Skipped accumulates a (reason, name) skip record on the task, with an
// optional trailing errs for evidence of why. It returns nothing —
// accumulating a record is an act, not a value to chain — is usable before
// the task resolves, and does not itself resolve the task. The taxonomy line
// ("- skipped N (a reasonA, b reasonB)") is derived from every accumulated
// record at render time, so no caller can hand-build (and thereby miscount)
// the summary; the aggregation key is untouched by errs. Any errs render as
// one bounded evidence line under the count row (first cause + "(+N more)"),
// full list under Verbose.
func (t *TaskHandle) Skipped(reason TaxonomyReason) {
	t.recordTaxonomy(reason, "", dispositionSkip, nil)
	t.finish(Skipped, "", nil)
}

// Kept records a keep reason on this Task (the Task name is the kept name)
// and resolves the Task as Done.
func (t *TaskHandle) Kept(reason TaxonomyReason) {
	t.recordTaxonomy(reason, "", dispositionKeep, nil)
	t.finish(Done, "", nil)
}

func (t *TaskHandle) recordTaxonomy(reason TaxonomyReason, name string, verb dispositionVerb, errs []error) {
	t.withTask(func(st *taskState) { t.recordTaxonomyLocked(st, reason, name, verb, errs) })
}

func (t *TaskHandle) recordTaxonomyLocked(st *taskState, reason TaxonomyReason, name string, verb dispositionVerb, errs []error) {
	if err := t.out.ensureOpen(); err != nil {
		t.out.recordMisuse(err)
		return
	}
	if core.IsTerminalTask(st.state) {
		t.out.recordMisuseFor(st.name, ErrAlreadyResolved)
		return
	}
	if name == "" {
		name = st.name
	}
	rec := TaxonomyRecord{Reason: reason.name, Name: txt.Text(name), Causes: causesFromErrors(errs)}
	switch verb {
	case dispositionSkip:
		st.skipped = append(st.skipped, rec)
	case dispositionKeep:
		st.kept = append(st.kept, rec)
	}
	t.out.bumpLocked()
	t.out.appendEventLocked(Event{Type: "task." + string(verb) + "_recorded", EntityID: t.id})
	t.out.emitWireEventLocked(wire.EventDispositionRecorded, t.id, wire.ToDispositionDoc(verb.wireName(), rec).EventPayload())
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
