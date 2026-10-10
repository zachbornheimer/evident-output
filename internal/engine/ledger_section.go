package engine

import (
	"github.com/zachbornheimer/evident-output/internal/record"
)

// ledgerTense is which ledger a section belongs to: [changed] records work
// that happened, [planned] work a dry run or preview would do.
type ledgerTense = record.LedgerTense

const (
	tenseChanged = record.TenseChanged
	tensePlanned = record.TensePlanned
)

// ledgerOwner is st as the ledger knows its Task: placed by declaration,
// found by id, shown by name, qualified by container path.
func (st *taskState) ledgerOwner() record.LedgerOwner {
	return record.LedgerOwner{ID: st.id, Name: st.name, Declaration: st.declaration, Containers: st.containerPath()}
}

// ledgerSectionIDLocked returns the id of owner's section in tense, opening
// it on first use. Caller must hold o.mu.
func (o *Output) ledgerSectionIDLocked(owner *taskState, tense ledgerTense) string {
	if id, ok := o.rec.SectionID(owner.id, tense); ok {
		return id
	}
	id := o.rec.OpenSection(owner.ledgerOwner(), tense, o.nextID(tense.String()))
	o.bumpLocked()
	o.appendEventLocked(Event{Type: tense.DeclaredEvent(), EntityID: id})
	return id
}

// hasRecordedEffectLocked reports whether the Task taskID's sections carry
// at least one row — see the unresolved-task amnesty in Finish (beginner-1,
// I1). Caller must hold o.mu.
func (o *Output) hasRecordedEffectLocked(taskID string) bool {
	return o.rec.HasRecordedEffect(taskID)
}

// hasPlannedEffect reports whether the Task taskID recorded at least one
// [planned] row: a mutation a dry run or preview skipped.
func (o *Output) hasPlannedEffect(taskID string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.rec.HasPlannedEffect(taskID)
}
