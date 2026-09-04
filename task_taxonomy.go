package evo

import txt "github.com/zachbornheimer/evident-output/internal/text"

// dispositionVerb names which accumulation act a Reason's usage constraints
// are checked against — TaskHandle.Skipped or TaskHandle.Kept.
type dispositionVerb string

const (
	dispositionSkip dispositionVerb = "skip"
	dispositionKeep dispositionVerb = "keep"
)

// Skipped accumulates a (reason, name) skip record on the task, with an
// optional trailing errs for evidence of why. It returns nothing —
// accumulating a record is an act, not a value to chain — is usable before
// the task resolves, and does not itself resolve the task. The taxonomy line
// ("!  skipped N  (a reasonA, b reasonB)") is derived from every accumulated
// record at render time, so no caller can hand-build (and thereby miscount)
// the summary; the aggregation key is untouched by errs. Any errs render as
// one bounded evidence line under the count row (first cause + "(+N more)"),
// full list under Verbose.
func (t *TaskHandle) Skipped(reason TaxonomyReason, name string, errs ...error) {
	t.recordTaxonomy(reason, name, dispositionSkip, errs)
}

// Kept accumulates a (reason, name) keep record on the task — same machinery
// as Skipped, second verb ("!  kept N  (...)").
func (t *TaskHandle) Kept(reason TaxonomyReason, name string, errs ...error) {
	t.recordTaxonomy(reason, name, dispositionKeep, errs)
}

func (t *TaskHandle) recordTaxonomy(reason TaxonomyReason, name string, verb dispositionVerb, errs []error) {
	t.out.mu.Lock()
	defer t.out.mu.Unlock()
	st := t.out.taskByRef[t.id]
	if st == nil {
		return
	}
	if err := t.out.ensureOpen(); err != nil {
		t.out.recordMisuse(err)
		return
	}
	if isTerminalTask(st.state) {
		t.out.recordMisuseFor(st.name, ErrAlreadyResolved)
		return
	}
	t.out.enforceReasonConstraintLocked(reason, st.name, verb)
	rec := TaxonomyRecord{Reason: reason.name, Name: txt.Text(name), Causes: causesFromErrors(errs)}
	switch verb {
	case dispositionSkip:
		st.skipped = append(st.skipped, rec)
	case dispositionKeep:
		st.kept = append(st.kept, rec)
	}
	t.out.bumpLocked()
	t.out.appendEventLocked(Event{Type: "task." + string(verb) + "_recorded", EntityID: t.id})
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
// constraints (ForSkip, OnTask) don't match how it is being used here.
// Strict panics via recordMisuse; production still counts the record —
// a constraint violation degrades to "counted anyway", never a dropped truth.
func (o *Output) enforceReasonConstraintLocked(reason TaxonomyReason, taskName string, verb dispositionVerb) {
	if reason.forSkip && verb != dispositionSkip {
		o.recordMisuse(ErrReasonSkipOnly)
	}
	if reason.onTask != "" && reason.onTask != taskName {
		o.recordMisuse(ErrReasonWrongTask)
	}
}
