package engine

// Next attaches actions.
func (t *TaskHandle) Next(actions ...Action) *TaskHandle {
	return t.withTask(func(st *taskState) {
		if t.out.finishing || t.out.finished || t.out.closed {
			t.out.recordMisuse(ErrClosed)
			return
		}
		st.actions = append(st.actions, cloneActions(actions)...)
		t.out.bumpLocked()
	})
}

// NextCommand attaches a command action. args names a foreign tool's own
// executable explicitly — the common case, since most remedies point at a
// different tool than the one running right now.
func (t *TaskHandle) NextCommand(executable string, args ...string) *TaskHandle {
	return t.Next(Command(executable, args...))
}

// nextSelf attaches a command action that re-runs the caller's own binary
// with args — a self-referencing remedy ("rerun with --apply") that doesn't
// restate which binary to run (I6). Uses the same identity source as
// Confirm's PolicyFlag / I2's Fail fallback: Config.Title when set, else
// the binary's own basename. Use NextCommand instead when the remedy is a
// different (foreign) tool.
func (t *TaskHandle) nextSelf(args ...string) *TaskHandle {
	if t == nil || t.out == nil {
		return t
	}
	return t.NextCommand(t.out.policySourceName(), args...)
}
