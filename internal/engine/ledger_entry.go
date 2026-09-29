package engine

import (
	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
	"github.com/zachbornheimer/evident-output/internal/wire"
)

// ledgerEntry is one row an Effect, File, or Exec records into a Task's
// Plan (dry run) or Changes (applied) ledger.
type ledgerEntry struct {
	// verb is the imperative verb ("delete"); the applied ledger conjugates
	// it to past tense.
	verb   string
	object string
	// quantity counts object when counted; File and Exec name one object
	// ("write <path>") instead of counting.
	quantity int
	counted  bool
}

// entry is s's ledger row for quantity objects: s.Quantity for a
// committed Effect, the committed subset for a PartialEffect.
func (s EffectSpec) entry(quantity int) ledgerEntry {
	return ledgerEntry{verb: string(s.Verb), object: s.Object, quantity: quantity, counted: true}
}

// namedEntry is an uncounted row naming one object: File's "write <path>",
// Exec's "run <executable>".
func namedEntry(verb, object string) ledgerEntry {
	return ledgerEntry{verb: verb, object: object}
}

// payload is the effect.planned / effect.committed wire payload for e,
// recorded under verb. It carries the quantity the ledger recorded, so the
// JSONL stream agrees with the human rows and the final document.
func (e ledgerEntry) payload(verb string) map[string]any {
	payload := map[string]any{"verb": verb, "object": e.object}
	if e.counted {
		payload["quantity"] = e.quantity
	}
	return payload
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
	return ledgerTarget{owner: st, tense: tenseFor(o.cfg.dryRun)}, nil
}

// recordLedgerEntry resolves taskID's ledger target and records e there —
// the entry point for File and Exec, whose rows carry no callback, so
// there is no window between resolving and recording (compare Effect,
// which resolves before its callback runs).
func (o *Output) recordLedgerEntry(taskID string, e ledgerEntry) {
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
func (o *Output) recordResolvedEntry(taskID string, target ledgerTarget, e ledgerEntry) {
	verb, event := e.verb, wire.EventEffectPlanned
	if target.tense == tenseChanged {
		verb, event = txt.ConjugatePast(e.verb), wire.EventEffectCommitted
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return
	}
	sec := o.ledgerSectionLocked(target.owner, target.tense)
	if sec.intendedVerb == "" {
		sec.intendedVerb = txt.Text(e.verb)
	}
	if sec.record(verb, e) {
		o.bumpLocked()
		o.appendEventLocked(Event{Type: target.tense.recordedEvent(), EntityID: sec.id})
	}
	o.emitWireEventLocked(event, taskID, e.payload(verb))
}
