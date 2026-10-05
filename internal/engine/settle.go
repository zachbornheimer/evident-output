package engine

// settleLocked is the one way a Task reaches a terminal state. Every path
// that ends a Task (a caller's verb, a scheduled callback's return, a
// failed predecessor's cascade, a cancelled Confirm, a duplicate
// declaration, Finish's sweep) goes through it, so the bookkeeping a
// terminal transition owes cannot drift between sites:
//
//   - the active phase clears;
//   - every Wait parked on the Task wakes;
//   - submitted work that never started releases its hold on the drain;
//   - the snapshot version advances and the task.<state> event is journaled;
//   - its collections' tallies move, and every Task parked on it (or on a
//     collection it just resolved) is placed again (see wakeLocked).
//
// It also owns the evidence rule (honestOutcome): a success-class target
// over a Task holding a Problem settles Failed, whichever path asked.
//
// Callers own only what differs between paths: the summary, the Problems,
// and where the settled row is committed. Callers must already hold o.mu.
func (o *Output) settleLocked(st *taskState, state EntityState) {
	state = st.honestOutcome(state)
	from := st.state
	st.state = state
	if st.gateFor != nil {
		o.concludeGateLocked(st)
		return
	}
	st.censusMoved(from)
	st.phase = ""
	o.stopPlainHeartbeatLocked(st)
	st.closeDoneLocked()
	if st.sched.awaitingStart() {
		o.abandonLocked(st)
	}
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "task." + string(state), EntityID: st.id})
	o.propagateSettleLocked(st)
}
