package engine

import (
	"strings"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/render"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// ledgerTense is which ledger a section belongs to: [changed] records work
// that happened, [planned] work a dry run or preview would do.
type ledgerTense int

const (
	tenseChanged ledgerTense = iota
	tensePlanned
)

// tenseFor is the tense a run's ledger rows take.
func tenseFor(dryRun bool) ledgerTense {
	if dryRun {
		return tensePlanned
	}
	return tenseChanged
}

// String is the section header word ("changed", "planned").
func (t ledgerTense) String() string {
	if t == tensePlanned {
		return "planned"
	}
	return "changed"
}

// declaredEvent and recordedEvent are the journal events this tense's
// sections emit.
func (t ledgerTense) declaredEvent() string {
	if t == tensePlanned {
		return "plan.declared"
	}
	return "changes.declared"
}

func (t ledgerTense) recordedEvent() string {
	if t == tensePlanned {
		return "plan.recorded"
	}
	return "change.recorded"
}

// ledgerSection is one Task's [changed] or [planned] rows: the Effects,
// Files, and Execs its Define recorded. The Task owns the section; its name
// is only how the section is shown. Guarded by o.mu.
type ledgerSection struct {
	id    string
	owner *taskState
	tense ledgerTense
	// subject is the rendered name: the owner's name, or its container path
	// when another section shares that name (see qualifyLocked).
	subject string
	records []EffectRecord
	// intendedVerb is the first imperative verb recorded (evo-rec.md "empty
	// effect section grammar"), so a section that ends up with zero rows
	// still renders "nothing to <verb> <subject>".
	intendedVerb string
	// namedRowsEmitted is true once commitNamedEffectsLocked streamed this
	// section at its owner's resolution; Finish's residual ledger skips it
	// so its rows never render twice.
	namedRowsEmitted bool
}

// order is the section's place in the ledger: its owner's declaration, so
// the ledger reads the same on every run whichever Task finished first.
func (s *ledgerSection) order() int { return s.owner.declaration }

// record appends one row. A counted entry with zero quantity adds no row
// (there is nothing to show) and reports false. verb is already in the
// section's tense.
func (s *ledgerSection) record(verb string, e ledgerEntry) bool {
	if e.counted && e.quantity == 0 {
		return false
	}
	row := EffectRecord{Verb: txt.Text(verb), Object: txt.Text(e.object)}
	if e.counted {
		row.Quantity = int64(e.quantity)
		row.HasQty = true
	}
	s.records = append(s.records, row)
	return true
}

func (s *ledgerSection) changesSnapshot() ChangesSnapshot {
	c := core.NewChangesSnapshot(ChangesSnapshot{ID: s.id, Subject: s.subject, Records: append([]EffectRecord(nil), s.records...), IntendedVerb: s.intendedVerb}, s.owner.id)
	return core.WithChangesContainers(c, s.owner.containerPath())
}

func (s *ledgerSection) planSnapshot() PlanSnapshot {
	p := core.NewPlanSnapshot(PlanSnapshot{ID: s.id, Subject: s.subject, Records: append([]EffectRecord(nil), s.records...), IntendedVerb: s.intendedVerb}, s.owner.id)
	return core.WithPlanContainers(p, s.owner.containerPath())
}

// foldSource is s as the renderer's ledger fold sees it.
func (s *ledgerSection) foldSource() render.SectionSource {
	return render.SectionSource{
		Subject: s.subject, Records: s.records, IntendedVerb: s.intendedVerb,
		Containers: s.owner.containerPath(), Streamed: s.namedRowsEmitted,
	}
}

// ledgerSectionKey identifies a section: one per owning Task per tense.
type ledgerSectionKey struct {
	owner string
	tense ledgerTense
}

// sectionsLocked is the run's section list for tense, in ledger order.
func (o *Output) sectionsLocked(tense ledgerTense) *[]*ledgerSection {
	if tense == tensePlanned {
		return &o.plans
	}
	return &o.changes
}

// ledgerSectionLocked returns owner's section in tense, opening it on first
// use. Caller must hold o.mu.
func (o *Output) ledgerSectionLocked(owner *taskState, tense ledgerTense) *ledgerSection {
	key := ledgerSectionKey{owner: owner.id, tense: tense}
	if s, ok := o.ledger.byOwner[key]; ok {
		return s
	}
	s := &ledgerSection{id: o.nextID(tense.String()), owner: owner, tense: tense, subject: owner.name}
	sections := o.sectionsLocked(tense)
	*sections = insertByLedgerOrder(*sections, s)
	o.ledger.opened(key, s)
	o.qualifyLocked(s)
	o.bumpLocked()
	o.appendEventLocked(Event{Type: tense.declaredEvent(), EntityID: s.id})
	return s
}

// qualifyLocked names the newly opened section by its container path once
// another section shares its name, and the one earlier section too when
// this open is what made the name ambiguous. Every other same-named
// section was qualified when it opened, and a Task's container path is
// fixed at declaration, so qualification stays O(1) per open. A root
// Task's section keeps its bare name: root names are unique siblings, and
// a root section may already have streamed (commitNamedEffectsLocked)
// before a nested one opened.
func (o *Output) qualifyLocked(opened *ledgerSection) {
	shared := o.ledger.byName[opened.owner.name]
	switch len(shared) {
	case 0, 1:
		return
	case 2:
		qualify(shared[0])
	}
	qualify(opened)
}

// qualify names a nested section by its container path.
func qualify(s *ledgerSection) {
	if s.owner.collection != nil {
		s.subject = qualifiedSubject(s.owner)
	}
}

// qualifiedSubject is st's container path and name ("alpha › prune").
func qualifiedSubject(st *taskState) string {
	parts := []string{st.name}
	for col := st.collection; col != nil; col = col.parent {
		parts = append(parts, col.name)
	}
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return strings.Join(parts, core.QualifiedSubjectSeparator)
}

// hasLedgerSectionLocked reports whether the Task taskID owns a section in
// either tense. Caller must hold o.mu.
func (o *Output) hasLedgerSectionLocked(taskID string) bool {
	_, changed := o.ledger.byOwner[ledgerSectionKey{owner: taskID, tense: tenseChanged}]
	_, planned := o.ledger.byOwner[ledgerSectionKey{owner: taskID, tense: tensePlanned}]
	return changed || planned
}

// hasRecordedEffectLocked reports whether the Task taskID's sections carry
// at least one row — see the unresolved-task amnesty in Finish (beginner-1,
// I1). Caller must hold o.mu.
func (o *Output) hasRecordedEffectLocked(taskID string) bool {
	for _, tense := range []ledgerTense{tenseChanged, tensePlanned} {
		if s, ok := o.ledger.byOwner[ledgerSectionKey{owner: taskID, tense: tense}]; ok && len(s.records) > 0 {
			return true
		}
	}
	return false
}

// hasPlannedEffect reports whether the Task taskID recorded at least one
// [planned] row: a mutation a dry run or preview skipped.
func (o *Output) hasPlannedEffect(taskID string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	s, ok := o.ledger.byOwner[ledgerSectionKey{owner: taskID, tense: tensePlanned}]
	return ok && len(s.records) > 0
}

// maxSubjectWidth is the widest subject among sections, so their rows
// align; a lone section needs no alignment.
func maxSubjectWidth(sections []*ledgerSection) int {
	if len(sections) < 2 {
		return 0
	}
	width := 0
	for _, s := range sections {
		width = max(width, len([]rune(s.subject)))
	}
	return width
}
