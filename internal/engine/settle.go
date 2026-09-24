package engine

// settleLocked is the one way a Task reaches a terminal state. Every path
// that ends a Task (a caller's verb, a scheduled callback's return, a
// failed predecessor's cascade, a cancelled Confirm, a duplicate
// declaration, Finish's sweep) goes through it, so the bookkeeping a
// terminal transition owes cannot drift between sites:
//
//   - the active phase clears;
//   - every Wait parked on the Task wakes;
//   - a Sequence step parked behind the Task is queued;
//   - a non-success outcome marks the dependents' cascade due;
//   - the snapshot version advances and the task.<state> event is journaled.
//
// Callers own only what differs between paths: the summary, the Problems,
// and where the settled row is committed. Callers must already hold o.mu.
func (o *Output) settleLocked(st *taskState, state EntityState) {
	st.state = state
	st.phase = ""
	st.closeDoneLocked()
	o.releaseNextStepLocked(st)
	if predecessorFailed(state) {
		o.schedCascadeDue = true
	}
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "task." + string(state), EntityID: st.id})
}
