package evo

// Mutation verbs on TaskHandle record what the task did (or, under DryRun,
// would do) into the one Changes/Plan section named after the task — the
// verb chooses the ledger tense (imperative vs. conjugatePast), the run's
// DryRun mode chooses Plan vs. Changes. Callers never write their own tense.
// These methods return nothing: recording a mutation is an act, not a value
// to chain.

// Delete records a deletion of quantity of object.
func (t *TaskHandle) Delete(quantity int64, object string) {
	t.Record("delete", quantity, object)
}

// Create records the creation of one named object.
func (t *TaskHandle) Create(object string) {
	t.RecordName("create", object)
}

// Update records an update of quantity of object.
func (t *TaskHandle) Update(quantity int64, object string) {
	t.Record("update", quantity, object)
}

// Remove records a removal of quantity of object.
func (t *TaskHandle) Remove(quantity int64, object string) {
	t.Record("remove", quantity, object)
}

// Write records the writing of one named object.
func (t *TaskHandle) Write(object string) {
	t.RecordName("write", object)
}

// Push records a push of quantity of object.
func (t *TaskHandle) Push(quantity int64, object string) {
	t.Record("push", quantity, object)
}

// Record records an arbitrary imperative verb/quantity/object mutation.
// Delete/Create/Update/Remove/Write/Push are named shorthands for this.
func (t *TaskHandle) Record(verb string, quantity int64, object string) {
	t.out.recordMutation(t.id, verb, quantity, true, object)
}

// RecordName records an arbitrary imperative verb and one named object
// without a quantity. Quantity is for collapsed counts; RecordName is one
// named object.
func (t *TaskHandle) RecordName(verb, object string) {
	t.out.recordMutation(t.id, verb, 0, false, object)
}

// recordMutation resolves the task named by taskID, then forwards verb to
// the Plan (DryRun) or Changes (applied) section sharing the task's name,
// conjugating verb to past tense for the applied ledger only.
func (o *Output) recordMutation(taskID, verb string, quantity int64, hasQty bool, object string) {
	o.mu.Lock()
	st := o.taskByRef[taskID]
	if st == nil {
		o.mu.Unlock()
		return
	}
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		o.mu.Unlock()
		return
	}
	if isTerminalTask(st.state) {
		o.recordMisuse(ErrAlreadyResolved)
		o.mu.Unlock()
		return
	}
	subject := st.name
	dryRun := o.cfg.dryRun
	o.mu.Unlock()

	if dryRun {
		sec := o.planGetOrCreate(subject)
		// Declare with the caller's imperative verb before Record runs, so a
		// section that ends up with zero rows still reads "nothing to delete
		// <subject>" (evo-rec.md "empty effect section grammar").
		sec.declareIntendedVerb(verb)
		if hasQty {
			sec.Record(verb, quantity, object)
		} else {
			sec.RecordName(verb, object)
		}
		return
	}
	sec := o.changesGetOrCreate(subject)
	sec.declareIntendedVerb(verb)
	pastTense := conjugatePast(verb)
	if hasQty {
		sec.Record(pastTense, quantity, object)
	} else {
		sec.RecordName(pastTense, object)
	}
}
