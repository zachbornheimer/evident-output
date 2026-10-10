package record

import "slices"

// Task is the truth of one Task: where it stands, what it reported, and what
// it found. It lives inside a Run and shares the Run's mutex, so a write is
// an append method and a read is a copy. What decides when the Task may run
// (the scheduler's state) is not truth and stays with the scheduler.
type Task struct {
	run *Run

	state          EntityState
	phase          string
	progress       Progress
	summary        string
	problems       []Problem
	warnings       []Problem
	facts          []Fact
	verification   []VerificationDetail
	resolution     Resolution
	verifyEvidence TaskEvidence
	skipped        []TaxonomyRecord
	kept           []TaxonomyRecord
}

// TaskInit is what a Task's record starts with. Zero fields stay zero.
type TaskInit struct {
	State      EntityState
	Progress   Progress
	Resolution Resolution
	Summary    string
	Problems   []Problem
}

// NewTask starts the record of a Task in this run.
func (r *Run) NewTask(init TaskInit) *Task {
	return &Task{
		run: r, state: init.State, progress: init.Progress, resolution: init.Resolution,
		summary: init.Summary, problems: init.Problems,
	}
}

// DeclaresSuccess reports whether state claims the work went well — the
// class of claim only the scheduler's observation can ratify.
func DeclaresSuccess(state EntityState) bool {
	return state == Done || state == Skipped
}

// TaskTruth is a view of a Task's record that shares its slices: read it
// under the lock that guards the run and never let it escape, since the next
// write to the Task can change what it shows. It costs no allocation, so a
// live frame can classify every Task by it and copy only the ones it shows.
type TaskTruth struct {
	State          EntityState
	Phase          string
	Progress       Progress
	Summary        string
	Problems       []Problem
	Warnings       []Problem
	Facts          []Fact
	Verification   []VerificationDetail
	Resolution     Resolution
	VerifyEvidence TaskEvidence
	Skipped        []TaxonomyRecord
	Kept           []TaxonomyRecord
}

// Truth is the shared view of the Task's record.
func (t *Task) Truth() TaskTruth {
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	return TaskTruth{
		State: t.state, Phase: t.phase, Progress: t.progress, Summary: t.summary,
		Problems: t.problems, Warnings: t.warnings, Facts: t.facts, Verification: t.verification,
		Resolution: t.resolution, VerifyEvidence: t.verifyEvidence, Skipped: t.skipped, Kept: t.kept,
	}
}

// State is where the Task stands.
func (t *Task) State() EntityState {
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	return t.state
}

// Transition moves the Task to state and returns the state it left.
func (t *Task) Transition(to EntityState) (from EntityState) {
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	from, t.state = t.state, to
	return from
}

// HonestOutcome is the one rule between a Task's blocking evidence and its
// terminal state: a Task holding any Problem cannot settle success-class, so
// a Done or Skipped claim over one settles Failed.
func (t *Task) HonestOutcome(state EntityState) EntityState {
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	if DeclaresSuccess(state) && len(t.problems) > 0 {
		return Failed
	}
	return state
}

// Phase is the Task's current-step text.
func (t *Task) Phase() string {
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	return t.phase
}

// SetPhase replaces the current-step text with a sanitized text.
func (t *Task) SetPhase(text string) {
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	t.phase = SanitizeText(text)
}

// SetPhaseIfChanged is SetPhase that reports false, changing nothing, when
// the sanitized text is already the phase.
func (t *Task) SetPhaseIfChanged(text string) bool {
	text = SanitizeText(text)
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	if text == t.phase {
		return false
	}
	t.phase = text
	return true
}

// ClearPhase ends the current step.
func (t *Task) ClearPhase() {
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	t.phase = ""
}

// Progress is the Task's absolute measurement.
func (t *Task) Progress() Progress {
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	return t.progress
}

// EnsureIndeterminateProgress gives a Task that has reported no measurement
// the Indeterminate kind, so a live row knows it is working.
func (t *Task) EnsureIndeterminateProgress() {
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	if t.progress.Kind == "" {
		t.progress.Kind = Indeterminate
	}
}

// ProgressVerdict is what a progress report did to a Task's measurement.
type ProgressVerdict int

const (
	// ProgressApplied means the measurement was recorded.
	ProgressApplied ProgressVerdict = iota
	// ProgressInvalid means the values or the sealed total were rejected.
	ProgressInvalid
	// ProgressRegressed means a running measurement of the same kind went backward.
	ProgressRegressed
)

// ApplyProgress records an absolute measurement unless a guard rejects it:
// values must be non-negative and consistent, a running measurement of the
// same kind may not regress, and once a nonzero total is reported for a kind
// it cannot change (retry-safety depends on the denominator staying put).
// Switching kind is a deliberate re-declaration and resets both freely.
func (t *Task) ApplyProgress(completed, total int64, kind ProgressKind) ProgressVerdict {
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	switch {
	case completed < 0 || total < 0:
		return ProgressInvalid
	case total == 0 && completed != 0:
		return ProgressInvalid
	case completed > total && total > 0:
		return ProgressInvalid
	}
	sameKindRunning := t.state == Running && t.progress.Kind != Indeterminate && t.progress.Total > 0 && kind == t.progress.Kind
	if sameKindRunning {
		if completed < t.progress.Completed {
			return ProgressRegressed
		}
		if total != t.progress.Total {
			return ProgressInvalid
		}
	}
	t.progress = Progress{Kind: kind, Completed: completed, Total: total}
	return ProgressApplied
}

// Summary is the one line of result text for the Task's terminal row.
func (t *Task) Summary() string {
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	return t.summary
}

// SetSummary replaces the result text with a sanitized text; empty clears it.
func (t *Task) SetSummary(text string) {
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	t.summary = SanitizeText(text)
}

// Resolution is why the Task settled successfully.
func (t *Task) Resolution() Resolution {
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	return t.resolution
}

// SetResolution records why the Task settled successfully.
func (t *Task) SetResolution(r Resolution) {
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	t.resolution = r
}

// VerifyEvidence is both observation phases a Verify recorded.
func (t *Task) VerifyEvidence() TaskEvidence {
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	return t.verifyEvidence
}

// RecordBeforeEvidence records the observation made before Define ran.
func (t *Task) RecordBeforeEvidence(p EvidencePhase) {
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	t.verifyEvidence.Before = p
}

// RecordAfterEvidence records the observation made after Define ran.
func (t *Task) RecordAfterEvidence(p EvidencePhase) {
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	t.verifyEvidence.After = p
}

// Problems is a copy of the Task's blocking Problems, nil when it has none.
func (t *Task) Problems() []Problem {
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	return CloneProblems(t.problems)
}

// ProblemCount is how many blocking Problems the Task holds.
func (t *Task) ProblemCount() int {
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	return len(t.problems)
}

// AppendProblems records blocking Problems, sanitized and copied.
func (t *Task) AppendProblems(problems ...Problem) {
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	t.problems = append(t.problems, StoreProblems(problems)...)
}

// ResolveProblems folds extra into the Task's Problems as it settles into
// state. A Failed or Blocked row's Problems that carry no detail of their
// own gain evidenceTail, the capture tail the Task already gathered, so the
// evidence a caller collected needs no opt-in.
func (t *Task) ResolveProblems(state EntityState, extra []Problem, evidenceTail string) {
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	if len(extra) == 0 && len(t.problems) == 0 {
		return
	}
	all := slices.Concat(t.problems, extra)
	if takesEvidenceTail(state) {
		for i := range all {
			if lacksDetail(all[i]) {
				all[i].Detail = evidenceTail
			}
		}
	}
	t.problems = StoreProblems(all)
}

// NeedsEvidenceTail reports whether settling into state with extra would give
// some Problem the capture tail, so the caller reads the capture only then.
func (t *Task) NeedsEvidenceTail(state EntityState, extra []Problem) bool {
	if !takesEvidenceTail(state) {
		return false
	}
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	return slices.ContainsFunc(t.problems, lacksDetail) || slices.ContainsFunc(extra, lacksDetail)
}

func takesEvidenceTail(state EntityState) bool { return state == Failed || state == Blocked }

func lacksDetail(p Problem) bool { return p.Detail == "" && p.EvidenceTail == "" }

// Warnings is a copy of the Task's warning-severity Problems.
func (t *Task) Warnings() []Problem {
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	return CloneProblems(t.warnings)
}

// WarningCount is how many warnings the Task holds.
func (t *Task) WarningCount() int {
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	return len(t.warnings)
}

// AppendWarning records a warning and returns how many the Task now holds.
// Warnings annotate the Task's lifecycle; they never replace it.
func (t *Task) AppendWarning(p Problem) int {
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	t.warnings = append(t.warnings, p)
	return len(t.warnings)
}

// Facts is a copy of the Task's discovered name/value annotations.
func (t *Task) Facts() []Fact {
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	return CloneFacts(t.facts)
}

// AppendFact records a discovered name/value annotation.
func (t *Task) AppendFact(f Fact) {
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	t.facts = append(t.facts, f)
}

// Verification is a copy of the per-attribute evidence File and Exec recorded.
func (t *Task) Verification() []VerificationDetail {
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	return CloneVerificationDetails(t.verification)
}

// AttachVerification records details, sanitized, and returns the stored copy.
func (t *Task) AttachVerification(details []VerificationDetail) []VerificationDetail {
	stored := StoreVerificationDetails(details)
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	t.verification = append(t.verification, stored...)
	return stored
}

// Skipped is a copy of the Task's skip records.
func (t *Task) Skipped() []TaxonomyRecord {
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	return CloneTaxonomy(t.skipped)
}

// Kept is a copy of the Task's keep records.
func (t *Task) Kept() []TaxonomyRecord {
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	return CloneTaxonomy(t.kept)
}

// HasTaxonomy reports whether the Task accumulated any skip or keep record.
func (t *Task) HasTaxonomy() bool {
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	return len(t.skipped) > 0 || len(t.kept) > 0
}

// AppendSkipped records one skipped item.
func (t *Task) AppendSkipped(rec TaxonomyRecord) {
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	t.skipped = append(t.skipped, rec)
}

// AppendKept records one kept item.
func (t *Task) AppendKept(rec TaxonomyRecord) {
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	t.kept = append(t.kept, rec)
}

// CloneTaxonomy deep-copies records, including each record's causes.
func CloneTaxonomy(in []TaxonomyRecord) []TaxonomyRecord {
	if len(in) == 0 {
		return nil
	}
	out := make([]TaxonomyRecord, len(in))
	copy(out, in)
	for i := range out {
		if len(out[i].Causes) > 0 {
			out[i].Causes = append([]string(nil), out[i].Causes...)
		}
	}
	return out
}
