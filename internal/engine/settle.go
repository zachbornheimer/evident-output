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
//   - its collections' tallies move, and every Task parked on it (or on a
//     collection it just resolved) is placed again (see wakeLocked).
//
// What the render state owes a terminal transition (the snapshot version,
// the task.<state> event, the live census and filing, the plain heartbeat)
// is the engine listener's job: it hears the transition once the lock is
// free (see outputListener).
//
// It also owns the evidence rule (Task.HonestOutcome): a success-class target
// over a Task holding a Problem settles Failed, whichever path asked.
//
// Callers own only what differs between paths: the summary, the Problems,
// and where the settled row is committed. Callers must already hold o.mu.
func (o *Output) settleLocked(st *taskState, state EntityState) {
	state = st.rec.HonestOutcome(state)
	st.rec.Transition(state)
	if st.gateFor != nil {
		o.concludeGateLocked(st)
		return
	}
	st.rec.ClearPhase()
	st.closeDoneLocked()
	if st.sched.awaitingStart() {
		o.abandonLocked(st)
	}
	o.propagateSettleLocked(st)
}
