package engine

import (
	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/record"
	txt "github.com/zachbornheimer/evident-output/internal/text"
	"github.com/zachbornheimer/evident-output/internal/wire"
)

// entry is s's ledger row for quantity objects: s.Quantity for a
// committed Effect, the committed subset for a PartialEffect.
func (s EffectSpec) entry(quantity int) record.LedgerEntry {
	return record.CountedEntry(string(s.Verb), s.Object, quantity)
}

// ledgerTarget is where a Task's ledger rows go: the Task that owns the
// section, and the tense the run records in.
type ledgerTarget struct {
	owner *taskState
	tense ledgerTense
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
	if core.IsTerminalTask(st.state) {
		o.recordMisuseFor(st.name, ErrAlreadyResolved)
		return ledgerTarget{}, ErrAlreadyResolved
	}
	return ledgerTarget{owner: st, tense: record.TenseFor(o.cfg.dryRun)}, nil
}

// recordLedgerEntry resolves taskID's ledger target and records e there —
// the entry point for File and Exec, whose rows carry no callback, so
// there is no window between resolving and recording (compare Effect,
// which resolves before its callback runs).
func (o *Output) recordLedgerEntry(taskID string, e record.LedgerEntry) {
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
func (o *Output) recordResolvedEntry(taskID string, target ledgerTarget, e record.LedgerEntry) {
	verb, event := e.Verb(), wire.EventEffectPlanned
	if target.tense == tenseChanged {
		verb, event = txt.ConjugatePast(e.Verb()), wire.EventEffectCommitted
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return
	}
	sectionID := o.ledgerSectionIDLocked(target.owner, target.tense)
	if o.rec.RecordEntry(target.owner.id, target.tense, verb, e) {
		o.bumpLocked()
		o.appendEventLocked(Event{Type: target.tense.RecordedEvent(), EntityID: sectionID})
	}
	o.emitWireEventLocked(event, taskID, e.Payload(verb))
}
