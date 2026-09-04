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
func (t *TaskHandle) mutate(verb, object string, call func() error, opts []EffectOption) error {
	if t == nil || t.out == nil {
		return nil
	}
	_, dryRun, ok := t.out.resolveLedgerTarget(t.id)
	if !ok {
		return nil
	}
	if !dryRun && call != nil {
		if err := call(); err != nil {
			return callerEffectError(err)
		}
	}
	eo := applyEffectOptions(opts)
	t.out.recordMutation(t.id, verb, int64(eo.quantity), eo.hasQty, object)
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
// directly, bypassing the call/dry-run boundary the named verbs enforce —
// the low-level primitive the named verbs (and the conformance goldens)
// share.
func (t *TaskHandle) Record(verb string, quantity int, object string) {
	t.out.recordMutation(t.id, verb, int64(quantity), true, object)
}

// RecordLabel records quantity of object (singular; see Delete) under
// label, verbatim, into the task's Changes ledger. Unlike Record's mutation
// verbs, label is a classification result (e.g. "ready", "blocked") rather
// than an imperative action, so it is never conjugated to past tense, and
// it never moves under [planned] during DryRun — classifying/observing
// already happened whether or not other mutations on this run are a dry
// run.
func (t *TaskHandle) RecordLabel(label string, quantity int, object string) {
	t.out.recordClassification(t.id, label, int64(quantity), object)
}

// RecordName records an arbitrary imperative verb and one named object
// without a quantity. Quantity is for collapsed counts; RecordName is one
// named object.
func (t *TaskHandle) RecordName(verb, object string) {
	t.out.recordMutation(t.id, verb, 0, false, object)
}

// resolveLedgerTarget resolves the task named by taskID and reports the
// ledger subject it mutates into (its own name) plus whether this run is a
// dry run — the shared guard (open, not yet resolved) behind both
// recordMutation and recordClassification. ok is false when the caller
// should record nothing further (misuse already recorded, or the task no
// longer exists).
func (o *Output) resolveLedgerTarget(taskID string) (subject string, dryRun bool, ok bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	st := o.taskByRef[taskID]
	if st == nil {
		return "", false, false
	}
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return "", false, false
	}
	if core.IsTerminalTask(st.state) {
		o.recordMisuseFor(st.name, ErrAlreadyResolved)
		return "", false, false
	}
	return st.name, o.cfg.dryRun, true
}

// recordMutation resolves the task named by taskID, then forwards verb to
// the Plan (DryRun) or Changes (applied) section sharing the task's name,
// conjugating verb to past tense for the applied ledger only.
func (o *Output) recordMutation(taskID, verb string, quantity int64, hasQty bool, object string) {
	subject, dryRun, ok := o.resolveLedgerTarget(taskID)
	if !ok {
		return
	}

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
	subject, _, ok := o.resolveLedgerTarget(taskID)
	if !ok {
		return
	}
	sec := o.changesGetOrCreate(subject)
	sec.declareIntendedVerb(label)
	sec.record(label, int(quantity), object)
}
