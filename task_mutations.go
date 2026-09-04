package evo

import (
	"fmt"

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

// Add records an addition of object.
func (t *TaskHandle) Add(object string, call func() error, opts ...EffectOption) error {
	return t.mutate("add", object, call, opts)
}

// Delete records a deletion of object; see Add for the call/dry-run/
// singular-object contract shared by every mutation verb.
func (t *TaskHandle) Delete(object string, call func() error, opts ...EffectOption) error {
	return t.mutate("delete", object, call, opts)
}

// Create records the creation of object.
func (t *TaskHandle) Create(object string, call func() error, opts ...EffectOption) error {
	return t.mutate("create", object, call, opts)
}

// Update records an update of object.
func (t *TaskHandle) Update(object string, call func() error, opts ...EffectOption) error {
	return t.mutate("update", object, call, opts)
}

// Remove records a removal of object.
func (t *TaskHandle) Remove(object string, call func() error, opts ...EffectOption) error {
	return t.mutate("remove", object, call, opts)
}

// Write records the writing of object.
func (t *TaskHandle) Write(object string, call func() error, opts ...EffectOption) error {
	return t.mutate("write", object, call, opts)
}

// Push records a push of object.
func (t *TaskHandle) Push(object string, call func() error, opts ...EffectOption) error {
	return t.mutate("push", object, call, opts)
}

// mutate is the shared mutation boundary behind Add/Delete/Create/Update/
// Remove/Write/Push: resolve the task's dry-run status, run call (never on
// a dry run, never when call is nil), then record the resulting effect as
// committed (Changes) or planned (Plan) depending on the run mode — never
// both, and never anything at all on a call error.
//
// The ledger target resolves exactly once, before call runs (E2.5 finding
// 5): re-resolving afterward — separately re-locking and re-checking whether
// the task is still open — created a window where a concurrent Done racing
// call's execution would see the task already terminal and silently drop
// the effect call just committed as spurious misuse. subject/dryRun are
// captured once and carried straight into recordResolvedMutation.
func (t *TaskHandle) mutate(verb, object string, call func() error, opts []EffectOption) error {
	if t == nil || t.out == nil {
		// A nil handle or a handle whose Output is already gone is caller
		// misuse, not a silent no-op (E2.5 finding 2): the caller must never
		// read a mutation verb's nil error as "it ran."
		return ErrClosed
	}
	eo := applyEffectOptions(opts)
	if eo.hasQty && eo.quantity < 0 {
		// A negative Affected count can never be a real effect quantity
		// (E2.5 finding 4) — caller misuse, recorded and nothing else
		// touched (no call, no ledger section).
		t.out.recordMisuse(ErrInvalidConfig)
		return ErrInvalidConfig
	}
	subject, dryRun, err := t.out.resolveLedgerTarget(t.id)
	if err != nil {
		return err
	}
	if !dryRun && call != nil {
		// P5 concurrency truth: a mutation-verb callback starting counts as
		// activity, promoting a Pending task to Running the same way
		// Phase/Progress/Each/Run/PhaseWriter's first write does — a
		// long-running Delete/Update call must render as working, not sit
		// parked on a queued-looking spinner-less row.
		t.out.promoteRunningForActivity(t.id)
		if err := call(); err != nil {
			return callerEffectError(err)
		}
	}
	if eo.hasQty && eo.quantity == 0 {
		// Affected(0): nothing happened, so there is nothing to plan or
		// declare (E2.5 finding 4) — declaring an empty section here is
		// exactly the "[planned] repo-retire" phantom-row bug the fixture
		// reports. Scoped to the Affected-quantity mutation-verb boundary
		// only: Record's own zero-quantity contract (evo-rec.md Problem 18's
		// "nothing to <verb> <subject>" empty-section grammar) is untouched.
		return nil
	}
	t.out.recordResolvedMutation(subject, dryRun, verb, int64(eo.quantity), eo.hasQty, object)
	return nil
}

// callerEffectError passes a mutation verb's call error back with its
// message untouched: it is the caller's own callback failing, not an
// evo-internal operation, so adding evo's own "doing X: " context here
// would misattribute ownership. The %w wrap keeps errors.Is/As reaching the
// caller's original error (mirrors TaskHandle.Run's own `return cmd.Run()`
// passthrough).
func callerEffectError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w", err)
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
