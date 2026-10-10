package engine

import (
	"context"
)

// TaskHandle is a handle for one operation with phases or progress.
type TaskHandle struct {
	out *Output
	id  string
	// rejected is why the declaration was refused (see rejectedTask); nil
	// for a declared Task.
	rejected error
	// facade holds this handle's public wrapper (see FacadeSlot).
	facade FacadeSlot
}

// Context reports the cancellation signal this task's work runs under — the
// run's own (see Output.Context), so a Define or Effect callback
// doing I/O selects on it and stops when the run is interrupted.
func (t *TaskHandle) Context() context.Context {
	if t == nil {
		return context.Background()
	}
	return t.out.Context()
}

// Snapshot returns the task snapshot.
func (t *TaskHandle) Snapshot() TaskSnapshot {
	t.out.mu.Lock()
	defer t.out.mu.Unlock()
	st := t.out.taskStates[t.id]
	if st == nil {
		return TaskSnapshot{ID: t.id, State: Pending, Progress: Progress{Kind: Indeterminate}}
	}
	return st.snapshot()
}

// Basis declares freshness inputs for this Task. Calls accumulate. Basis is
// Task configuration frozen at Define: a call after Define records
// ErrBasisAfterDefine and is ignored.
func (t *TaskHandle) Basis(inputs ...BasisSource) *TaskHandle {
	if t == nil || t.out == nil {
		return t
	}
	o := t.out
	o.mu.Lock()
	defer o.mu.Unlock()
	st := o.taskStates[t.id]
	if st == nil {
		return t
	}
	if !st.neverDefined() {
		o.recordMisuseFor(st.name, ErrBasisAfterDefine)
		return t
	}
	for _, in := range inputs {
		if in != nil {
			st.basisInputs = append(st.basisInputs, in)
		}
	}
	return t
}
