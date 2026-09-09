package engine

import (
	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// Mutation verbs on TaskHandle are Evo-controlled mutation boundaries (13-
// problem doc P1): the caller reports success and domain effects; evo
// derives the result (Changed/Ready/Planned), the tense (imperative vs.
// past), and the number (evo.Affected). object is always a singular noun
// phrase ("branch", not "branches") — the ledger pluralizes it from the
// affected quantity at render time (I4), so a call site never hand-composes
// its own singular/plural noun.
//
// call == nil records the effect without executing anything. Otherwise:
// normal run executes call and commits the effect only on success; dry run
// never executes call and records a planned effect instead; a non-nil error
// from call commits nothing and is returned (see callerEffectError).

// Add records an addition of n object.
func (t *TaskHandle) Add(n int, object string) { t.recordCount("add", n, object) }

// Delete records a deletion of n object.
func (t *TaskHandle) Delete(n int, object string) { t.recordCount("delete", n, object) }

// Create records the creation of n object.
func (t *TaskHandle) Create(n int, object string) { t.recordCount("create", n, object) }

// Update records an update of n object.
func (t *TaskHandle) Update(n int, object string) { t.recordCount("update", n, object) }

// Remove records a removal of n object.
func (t *TaskHandle) Remove(n int, object string) { t.recordCount("remove", n, object) }

// Write records the writing of n object.
func (t *TaskHandle) Write(n int, object string) { t.recordCount("write", n, object) }

// Push records a push of n object.
func (t *TaskHandle) Push(n int, object string) { t.recordCount("push", n, object) }

func (t *TaskHandle) recordCount(verb string, n int, object string) {
	if t == nil || t.out == nil {
		return
	}
	if n < 0 {
		t.out.recordMisuse(ErrInvalidConfig)
		return
	}
	if n == 0 {
		return
	}
	t.Record(verb, n, object)
}

// Record records an arbitrary imperative verb/quantity/object mutation
// directly, resolving the target task's dry-run status the same way the
// named verbs do (it does not bypass Plan/Changes routing — only the
// call/error boundary the named verbs wrap around an executed callback).
// The low-level primitive the named verbs (and the conformance goldens)
// share. Nil-safe: a nil TaskHandle, or one whose Output is already gone,
// records nothing instead of panicking.
func (t *TaskHandle) Record(verb string, quantity int, object string) {
	if t == nil || t.out == nil {
		return
	}
	t.out.recordMutation(t.id, verb, int64(quantity), true, object)
}

// RecordLabel records quantity of object (singular; see Delete) under
// label, verbatim, into the task's Changes ledger. Unlike Record's mutation
// verbs, label is a classification result (e.g. "ready", "blocked") rather
// than an imperative action, so it is never conjugated to past tense, and
// it never moves under [planned] during DryRun — classifying/observing
// already happened whether or not other mutations on this run are a dry
// run. Nil-safe: see Record.
func (t *TaskHandle) RecordLabel(label string, quantity int, object string) {
	if t == nil || t.out == nil {
		return
	}
	t.out.recordClassification(t.id, label, int64(quantity), object)
}

// RecordName records an arbitrary imperative verb and one named object
// without a quantity. Quantity is for collapsed counts; RecordName is one
// named object. Nil-safe: see Record.
func (t *TaskHandle) RecordName(verb, object string) {
	if t == nil || t.out == nil {
		return
	}
	t.out.recordMutation(t.id, verb, 0, false, object)
}

// resolveLedgerTarget resolves the task named by taskID and reports the
// ledger subject it mutates into (its own name) plus whether this run is a
// dry run — the shared guard (open, not yet resolved) behind
// TaskHandle.mutate, recordMutation, and recordClassification. err is
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
// the single-resolve entry point Record/RecordName use (their call carries
// no callback, so there is no window for the double-resolve race
// recordResolvedMutation's callers avoid; see TaskHandle.mutate).
func (o *Output) recordMutation(taskID, verb string, quantity int64, hasQty bool, object string) {
	subject, dryRun, err := o.resolveLedgerTarget(taskID)
	if err != nil {
		return
	}
	o.recordResolvedMutation(subject, dryRun, verb, quantity, hasQty, object)
}

// recordResolvedMutation records verb/quantity/object into subject's Plan
// (dryRun) or Changes (applied) ledger, conjugating verb to past tense for
// the applied ledger only. Takes an already-resolved subject/dryRun pair
// rather than re-resolving taskID itself (E2.5 finding 5): TaskHandle.mutate
// resolves once, before running its call, and passes that result straight
// through here — re-resolving after the call would re-open the terminal-task
// check to a state a concurrent Done may have legitimately changed in the
// meantime, dropping a real effect as spurious misuse. A zero-quantity
// Affected() call never reaches here at all (TaskHandle.mutate returns
// early, E2.5 finding 4) — Record's own zero-quantity call still does, and
// keeps declaring its intended verb so an empty Record section still renders
// evo-rec.md Problem 18's "nothing to <verb> <subject>" empty-section
// grammar.
func (o *Output) recordResolvedMutation(subject string, dryRun bool, verb string, quantity int64, hasQty bool, object string) {
	if dryRun {
		sec := o.planGetOrCreate(subject)
		// Declare with the caller's imperative verb before Record runs, so a
		// section that ends up with zero rows still reads "nothing to delete
		// <subject>" (evo-rec.md "empty effect section grammar").
		sec.declareIntendedVerb(verb)
		if hasQty {
			sec.record(verb, int(quantity), object)
		} else {
			sec.recordNoQty(verb, object)
		}
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
}

// recordClassification resolves the task named by taskID, then records
// quantity of object under label verbatim into the task's Changes ledger —
// always Changes, never Plan, and never conjugated (see
// TaskHandle.RecordLabel).
func (o *Output) recordClassification(taskID, label string, quantity int64, object string) {
	subject, _, err := o.resolveLedgerTarget(taskID)
	if err != nil {
		return
	}
	sec := o.changesGetOrCreate(subject)
	sec.declareIntendedVerb(label)
	sec.record(label, int(quantity), object)
}
