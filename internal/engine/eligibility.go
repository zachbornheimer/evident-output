package engine

// Eligibility is event-driven (§39): a Task becomes eligible the moment
// its last predecessor settles, whoever settled it (the scheduler, or the
// caller through Kept/Skipped/Fail/Cancel). That moment stamps EligibleAt
// and queues the work, so DependencyWait ends at the real cause and the
// scheduler never rescans the run to find it.

// indexDependentLocked registers a just-submitted Task under each of its
// predecessors, so settling one of them re-tests only its own dependents.
func (o *Output) indexDependentLocked(st *taskState) {
	if o.schedDependents == nil {
		o.schedDependents = make(map[predecessor][]*taskState)
	}
	for _, p := range st.preds {
		o.schedDependents[p] = append(o.schedDependents[p], st)
	}
}

// admitIfEligibleLocked queues st when its submitted work may start now.
func (o *Output) admitIfEligibleLocked(st *taskState) {
	if st == nil || st.readied || !o.claimableLocked(st) {
		return
	}
	o.noteEligibleLocked(st)
	o.schedReady.push(st)
}

// settleLocked stamps st's settle time and releases every Task that was
// waiting only on st: its After dependents, the dependents of each
// enclosing Group/Sequence, and its next Sequence sibling.
func (o *Output) settleLocked(st *taskState) {
	st.markSettled(o.cfg.clock.Now())
	for _, dep := range o.schedDependents[predecessor{taskID: st.id}] {
		o.admitIfEligibleLocked(dep)
	}
	for col := st.collection; col != nil; col = col.parent {
		for _, dep := range o.schedDependents[predecessor{groupID: col.id}] {
			o.admitIfEligibleLocked(dep)
		}
	}
	if st.collection != nil && st.collection.sequential {
		o.admitIfEligibleLocked(nextSibling(st))
	}
}

// admitAllEligibleLocked is the drain's one full pass: it queues any
// eligible Task a settle outside settleLocked (Finish's own sweeps) freed.
func (o *Output) admitAllEligibleLocked() {
	for _, st := range o.tasks {
		o.admitIfEligibleLocked(st)
	}
}

// nextSibling is the Task declared right after st in its container.
func nextSibling(st *taskState) *taskState {
	if st.collection == nil {
		return nil
	}
	siblings := st.collection.tasks
	for i, sib := range siblings {
		if sib == st && i+1 < len(siblings) {
			return siblings[i+1]
		}
	}
	return nil
}
