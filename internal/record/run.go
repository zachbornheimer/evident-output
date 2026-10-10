package record

import (
	"sync"
	"sync/atomic"
)

// Run is the truth of one run: the one place its events, outcomes and
// ledger are held. Every write is an append method and every read returns a
// copy, so nothing outside this package can change what happened. One mutex
// guards all of it; no method calls out of the package while holding it.
type Run struct {
	mu      sync.Mutex
	journal journal
	ledger  ledger
	// reasons backs get-or-create identity for evo.Reason: repeated calls with
	// the same name merge into one bucket.
	reasons map[string]TaxonomyReason
	// lines is the history-format text of everything printed, for the final
	// plain projection and residual emission.
	lines    []string
	messages []MessageSnapshot
	debug    []DebugRecord
	// facts and warnings are run-scoped annotations: the same "annotate, never
	// resolve" contract a task's facts and warnings have, scoped to the run.
	facts      []Fact
	warnings   []Problem
	conclusion *Conclusion
	// listener hears changes after the lock is released (see SetListener).
	listener atomic.Pointer[listenerBox]
}

// NewRun is an empty record of a run that has not started.
func NewRun() *Run { return &Run{} }

// AppendEvent stamps e with the next sequence number, records it, and
// returns the stamped event. limit <= 0 keeps every event.
func (r *Run) AppendEvent(e Event, limit int) Event {
	stamped := r.stampEvent(e, limit)
	r.notifyEventAppended(stamped)
	return stamped
}

func (r *Run) stampEvent(e Event, limit int) Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.journal.append(e, limit)
}

// Events is a copy of the events still retained under limit.
func (r *Run) Events(limit int) []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.journal.snapshot(limit)
}

// SectionID is the id of owner's ledger section in tense, when it is open.
func (r *Run) SectionID(ownerID string, tense LedgerTense) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.ledger.find(ownerID, tense)
	if !ok {
		return "", false
	}
	return s.id, true
}

// OpenSection opens owner's ledger section in tense under id and returns id.
// The caller checks SectionID first: opening twice replaces the section.
func (r *Run) OpenSection(owner LedgerOwner, tense LedgerTense, id string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ledger.open(owner, tense, id).id
}

// RecordEntry records e into the ownerID's open section in tense, with verb
// already in the section's tense, and reports whether a row was added. The
// first entry also fixes the section's intended verb, so a section that
// ends with zero rows still reads "nothing to <verb> <subject>".
func (r *Run) RecordEntry(ownerID string, tense LedgerTense, verb string, e LedgerEntry) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.ledger.find(ownerID, tense)
	if !ok {
		return false
	}
	if s.intendedVerb == "" {
		s.intendedVerb = SanitizeText(e.verb)
	}
	return s.record(verb, e)
}

// Section is a copy of owner's ledger section in tense, when it is open.
func (r *Run) Section(ownerID string, tense LedgerTense) (SectionView, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.ledger.find(ownerID, tense)
	if !ok {
		return SectionView{}, false
	}
	return s.view(), true
}

// Sections is a copy of tense's ledger sections, in ledger order.
func (r *Run) Sections(tense LedgerTense) []SectionView {
	r.mu.Lock()
	defer r.mu.Unlock()
	sections := r.ledger.sections(tense)
	views := make([]SectionView, len(sections))
	for i, s := range sections {
		views[i] = s.view()
	}
	return views
}

// MarkSectionStreamed notes that ownerID's section in tense had its named
// rows written, so Finish's residual ledger skips it.
func (r *Run) MarkSectionStreamed(ownerID string, tense LedgerTense) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.ledger.find(ownerID, tense); ok {
		s.streamed = true
	}
}

// HasLedgerSection reports whether ownerID owns a section in either tense.
func (r *Run) HasLedgerSection(ownerID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ledger.hasSection(ownerID)
}

// HasRecordedEffect reports whether ownerID's sections carry at least one row.
func (r *Run) HasRecordedEffect(ownerID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ledger.hasRecordedEffect(ownerID)
}

// HasPlannedEffect reports whether ownerID recorded at least one [planned]
// row: a mutation a dry run or preview skipped.
func (r *Run) HasPlannedEffect(ownerID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ledger.hasPlannedEffect(ownerID)
}

// HasUnstreamedSection reports whether any section still waits for Finish
// to write it.
func (r *Run) HasUnstreamedSection() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ledger.hasUnstreamedSection()
}

// MaxSubjectWidth is the widest subject among tense's sections, 0 for fewer
// than two (a lone section needs no alignment).
func (r *Run) MaxSubjectWidth(tense LedgerTense) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ledger.maxSubjectWidth(tense)
}

// ChangeSnapshots is the [changed] sections as snapshots, in ledger order.
func (r *Run) ChangeSnapshots() []ChangesSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []ChangesSnapshot
	for _, s := range r.ledger.changes {
		out = append(out, s.changesSnapshot())
	}
	return out
}

// PlanSnapshots is the [planned] sections as snapshots, in ledger order.
func (r *Run) PlanSnapshots() []PlanSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []PlanSnapshot
	for _, s := range r.ledger.plans {
		out = append(out, s.planSnapshot())
	}
	return out
}
