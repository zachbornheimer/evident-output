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
// target is the raw, undecided outcome the caller wants (Done, Failed,
// Blocked, ...); hasProblems is whether the Task already carries a
// Problem. settleLocked passes both straight to State.Settle, which runs
// Decide and writes the result in one call — the evidence rule fires a
// single time per resolution, inside lifecycle, rather than once in the
// caller and again here. ok reports whether the Task actually moved:
// State.Settle refuses to re-settle a Task that is already terminal, so a
// caller racing a second resolution onto the same Task gets false back
// and does none of the bookkeeping below a second time. resolved is the
// Decide'd value that was actually stored, valid only when ok is true.
//
// Callers own only what differs between paths: the summary, the Problems,
// and where the settled row is committed. Callers must already hold o.mu.
func (o *Output) settleLocked(st *taskState, target EntityState, hasProblems bool) (resolved EntityState, ok bool) {
	resolved, from, ok := st.state.Settle(target, hasProblems)
	if !ok {
		return resolved, false
	}
	st.censusMoved(from)
	st.phase = ""
	o.stopPlainHeartbeatLocked(st)
	st.closeDoneLocked()
	if st.sched.standing.AwaitingStart() {
		o.abandonLocked(st)
	}
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "task." + string(resolved), EntityID: st.id})
	o.propagateSettleLocked(st)
	return resolved, true
}
