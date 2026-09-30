package engine

// attachActions appends remedy actions to the Task itself. Engine-internal
// only: a remedy authored by a caller belongs to a Problem (Next /
// NextCommand ProblemOptions); the engine's own policy hints (Confirm's
// non-interactive block) have no caller-visible Problem to ride on.
func (t *TaskHandle) attachActions(actions ...Action) *TaskHandle {
	return t.withTask(func(st *taskState) {
		if t.out.finishing || t.out.finished || t.out.closed {
			t.out.recordMisuse(ErrClosed)
			return
		}
		st.actions = append(st.actions, cloneActions(actions)...)
		t.out.bumpLocked()
	})
}

// nextSelf is a ProblemOption that attaches a command action re-running the
// caller's own binary with args — a self-referencing remedy ("rerun with
// --apply") that doesn't restate which binary to run (I6). Uses the same
// identity source as Confirm's PolicyFlag / I2's Fail fallback: Config.Title
// when set, else the binary's own basename. Use NextCommand instead when the
// remedy is a different (foreign) tool.
func (o *Output) nextSelf(args ...string) ProblemOption {
	return NextCommand(o.policySourceName(), args...)
}
