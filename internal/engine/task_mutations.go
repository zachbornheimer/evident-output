package engine

import (
	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
	"github.com/zachbornheimer/evident-output/internal/wire"
)

// recordName records an imperative verb and one named object, without a
// quantity, into the task's Plan (DryRun) or Changes (applied) ledger — the
// row evo.File ("write <path>") and evo.Exec ("run <executable>") commit
// for the operation they just performed. Nil-safe: a nil TaskHandle, or
// one whose Output is already gone, records nothing.
func (t *TaskHandle) recordName(verb, object string) {
	if t == nil || t.out == nil {
		return
	}
	t.out.recordMutation(t.id, verb, 0, false, object)
}

// resolveLedgerTarget resolves the task named by taskID and reports the
// ledger subject it mutates into (the Task's own name) plus whether this
// run is a dry run — the shared guard (open, not yet resolved) behind
// Effect and recordMutation. err is
// non-nil (already recorded as misuse where the cause is not simply "the
// task no longer exists") when the caller should record nothing further.
func (o *Output) resolveLedgerTarget(taskID string) (subject string, dryRun bool, err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	st := o.taskByRef[taskID]
	if st == nil {
		return "", false, ErrClosed
	}
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return "", false, err
	}
	if core.IsTerminalTask(st.state) {
		o.recordMisuseFor(st.name, ErrAlreadyResolved)
		return "", false, ErrAlreadyResolved
	}
	return st.name, o.cfg.dryRun, nil
}

// recordMutation resolves the task named by taskID, then forwards verb to
// the Plan (DryRun) or Changes (applied) section sharing the task's name —
// the single-resolve entry point recordName uses (their call carries
// no callback, so there is no window for the double-resolve race
// recordResolvedMutation's callers avoid; see Effect).
func (o *Output) recordMutation(taskID, verb string, quantity int64, hasQty bool, object string) {
	subject, dryRun, err := o.resolveLedgerTarget(taskID)
	if err != nil {
		return
	}
	o.recordResolvedMutation(taskID, subject, dryRun, verb, quantity, hasQty, object)
}

// recordResolvedMutation records verb/quantity/object into subject's Plan
// (dryRun) or Changes (applied) ledger, conjugating verb to past tense for
// the applied ledger only. Takes an already-resolved subject/dryRun pair
// rather than re-resolving taskID itself (E2.5 finding 5): Effect resolves
// once, before running its callback, and passes that result straight
// through here — re-resolving after the call would re-open the terminal-task
// check to a state a concurrent resolution may have legitimately changed in
// the meantime, dropping a real effect as spurious misuse. A zero-quantity
// Effect never reaches here at all (EffectSpec validation rejects it, E2.5
// finding 4). The intended verb is still declared first, so a section that
// ends up with zero rows renders evo-rec.md Problem 18's "nothing to <verb>
// <subject>" empty-section grammar.
func (o *Output) recordResolvedMutation(taskID, subject string, dryRun bool, verb string, quantity int64, hasQty bool, object string) {
	if dryRun {
		sec := o.planGetOrCreate(subject)
		// Declare with the caller's imperative verb before recording, so a
		// section that ends up with zero rows still reads "nothing to delete
		// <subject>" (evo-rec.md "empty effect section grammar").
		sec.declareIntendedVerb(verb)
		if hasQty {
			sec.record(verb, int(quantity), object)
		} else {
			sec.recordNoQty(verb, object)
		}
		o.mu.Lock()
		o.emitWireEventLocked(wire.EventEffectPlanned, taskID, effectPayload(verb, quantity, hasQty, object))
		o.mu.Unlock()
		return
	}
	sec := o.changesGetOrCreate(subject)
	sec.declareIntendedVerb(verb)
	pastTense := txt.ConjugatePast(verb)
	if hasQty {
		sec.record(pastTense, int(quantity), object)
	} else {
		sec.recordNoQty(pastTense, object)
	}
	o.mu.Lock()
	o.emitWireEventLocked(wire.EventEffectCommitted, taskID, effectPayload(pastTense, quantity, hasQty, object))
	o.mu.Unlock()
}

// effectPayload is the effect.planned / effect.committed wire payload. It
// carries the quantity the ledger recorded, so the JSONL stream agrees with
// the human rows and the final document on the count.
func effectPayload(verb string, quantity int64, hasQty bool, object string) map[string]any {
	payload := map[string]any{"verb": verb, "object": object}
	if hasQty {
		payload["quantity"] = quantity
	}
	return payload
}
