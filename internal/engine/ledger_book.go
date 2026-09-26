package engine

import (
	"slices"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/engine/ledger"
)

// entry is s's ledger row for quantity objects: s.Quantity for a
// committed Effect, the committed subset for a PartialEffect.
func (s EffectSpec) entry(quantity int) ledger.Entry {
	return ledger.Counted(string(s.Verb), s.Object, quantity)
}

// ledgerTarget is where a Task's ledger rows go: the Task that owns the
// section, and the tense the run records in.
type ledgerTarget struct {
	owner *taskState
	tense ledger.Tense
}

// resolveLedgerTarget resolves the task named by taskID to its ledger
// target — the shared guard (open, not yet resolved) behind Effect and
// recordLedgerEntry. err is non-nil (already recorded as misuse where the
// cause is not simply "the task no longer exists") when the caller should
// record nothing further.
func (o *Output) resolveLedgerTarget(taskID string) (ledgerTarget, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	st := o.taskByRef[taskID]
	if st == nil {
		return ledgerTarget{}, ErrClosed
	}
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return ledgerTarget{}, err
	}
	if core.IsTerminalTask(st.state.Current()) {
		o.recordMisuseFor(st.name, ErrAlreadyResolved)
		return ledgerTarget{}, ErrAlreadyResolved
	}
	return ledgerTarget{owner: st, tense: ledger.TenseFor(o.cfg.dryRun)}, nil
}

// recordLedgerEntry resolves taskID's ledger target and records e there —
// the entry point for File and Exec, whose rows carry no callback, so
// there is no window between resolving and recording (compare Effect,
// which resolves before its callback runs).
func (o *Output) recordLedgerEntry(taskID string, e ledger.Entry) {
	target, err := o.resolveLedgerTarget(taskID)
	if err != nil {
		return
	}
	o.recordResolvedEntry(taskID, target, e)
}

// recordResolvedEntry records e into target's section, conjugating the verb
// to past tense for the applied ledger only. It takes an already-resolved
// target: Effect resolves once, before running its callback, because
// re-resolving afterward would re-open the terminal-task check to a state a
// concurrent resolution may have legitimately changed, dropping a real
// effect as spurious misuse. The imperative verb is kept as the section's
// intended verb, so a section that ends up with zero rows renders "nothing
// to <verb> <subject>" (evo-rec.md Problem 18).
func (o *Output) recordResolvedEntry(taskID string, target ledgerTarget, e ledger.Entry) {
	verb, event := target.tense.Verb(e.Verb()), target.tense.WireEvent()
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return
	}
	sec := o.ledgerSectionLocked(target.owner, target.tense)
	if sec.Record(verb, e) {
		o.bumpLocked()
		o.appendEventLocked(Event{Type: target.tense.RecordedEvent(), EntityID: sec.ID()})
	}
	o.emitWireEventLocked(event, taskID, e.Payload(verb))
}

// ledgerSectionLocked returns owner's section in tense, opening it on first
// use. Caller must hold o.mu.
func (o *Output) ledgerSectionLocked(owner *taskState, tense ledger.Tense) *ledger.Section {
	if s, ok := o.book.Find(owner.id, tense); ok {
		return s
	}
	s, _ := o.book.Open(ledgerOwner(owner), tense, o.nextID(tense.String()))
	o.bumpLocked()
	o.appendEventLocked(Event{Type: tense.DeclaredEvent(), EntityID: s.ID()})
	return s
}

// ledgerOwner is st's ledger.Owner, as fixed at declaration.
func ledgerOwner(st *taskState) ledger.Owner {
	var containers []string
	for col := st.collection; col != nil; col = col.parent {
		containers = append(containers, col.name)
	}
	slices.Reverse(containers)
	return ledger.Owner{ID: st.id, Name: st.name, Declaration: st.declaration, Containers: containers}
}

// hasPlannedEffect reports whether the Task taskID recorded at least one
// [planned] row: a mutation a dry run or preview skipped.
func (o *Output) hasPlannedEffect(taskID string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.book.HasRecordsIn(taskID, ledger.Planned)
}
