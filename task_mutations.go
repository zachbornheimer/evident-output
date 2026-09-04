package evo

import (
	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// Mutation verbs on TaskHandle record what the task did (or, under DryRun,
// would do) into the one Changes/Plan section named after the task — the
// verb chooses the ledger tense (imperative vs. conjugatePast), the run's
// DryRun mode chooses Plan vs. Changes. Callers never write their own tense.
// These methods return nothing: recording a mutation is an act, not a value
// to chain.

// Add records an addition of quantity of object; see Delete for the
// singular-object convention and the int (not int64) quantity. Part of the
// verb set unified across TaskHandle/Changes/Plan (C10) — Add was
// previously Plan-only.
func (t *TaskHandle) Add(quantity int, object string) {
	t.Record("add", quantity, object)
}

// Delete records a deletion of quantity of object. object is always
// singular ("branch", not "branches") — the ledger pluralizes it from
// quantity at render time (I4), so a call site never hand-composes its own
// singular/plural noun or calls evo.Pluralize itself. quantity is int (not
// int64) so the common caller shape — Delete(len(x), "...") — compiles
// without a manual conversion.
func (t *TaskHandle) Delete(quantity int, object string) {
	t.Record("delete", quantity, object)
}

// Create records the creation of one named object.
func (t *TaskHandle) Create(object string) {
	t.RecordName("create", object)
}

// Update records an update of quantity of object; see Delete for the
// singular-object convention and the int (not int64) quantity.
func (t *TaskHandle) Update(quantity int, object string) {
	t.Record("update", quantity, object)
}

// Remove records a removal of quantity of object; see Delete for the
// singular-object convention and the int (not int64) quantity.
func (t *TaskHandle) Remove(quantity int, object string) {
	t.Record("remove", quantity, object)
}

// Write records the writing of one named object.
func (t *TaskHandle) Write(object string) {
	t.RecordName("write", object)
}

// Push records a push of quantity of object; see Delete for the
// singular-object convention and the int (not int64) quantity.
func (t *TaskHandle) Push(quantity int, object string) {
	t.Record("push", quantity, object)
}

// Record records an arbitrary imperative verb/quantity/object mutation.
// Add/Delete/Create/Update/Remove/Write/Push are named shorthands for this.
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
			sec.Record(verb, int(quantity), object)
		} else {
			sec.RecordName(verb, object)
		}
		return
	}
	sec := o.changesGetOrCreate(subject)
	sec.declareIntendedVerb(verb)
	pastTense := txt.ConjugatePast(verb)
	if hasQty {
		sec.Record(pastTense, int(quantity), object)
	} else {
		sec.RecordName(pastTense, object)
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
	sec.Record(label, int(quantity), object)
}
