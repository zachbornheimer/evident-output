package engine

import "github.com/zachbornheimer/evident-output/internal/core"

// interrupt stops the run at the first signal. The graph owns the order that
// leaves the ledger honest (see graph.Graph.Interrupt); the engine's part is
// putting the active rows into the state the reader must see.
func (o *Output) interrupt(reason string) {
	if o == nil {
		return
	}
	o.graph.Interrupt(cancelCauseUser, func() { o.cancelActive(reason) })
}

// cancelActive cancels the currently running task, or the output itself when
// no task is running, so an interrupt always leaves a typed Cancelled state.
//
// A pending Confirm gate takes priority over the generic task scan below: a
// gate holds sole control of the run (Confirm suspends the live region and
// blocks on stdin) and its abort channel — not TaskHandle.Cancel — is what
// unblocks the stdin read. Since a Confirm gate is an ordinary Task while
// its answer is pending, the generic Pending-task fallback would otherwise
// resolve it to Cancelled without ever closing that channel, leaving
// readConfirmLine blocked forever.
func (o *Output) cancelActive(reason string) {
	o.mu.Lock()
	if o.cancelPendingConfirmLocked(reason) {
		o.mu.Unlock()
		return
	}
	running := make([]*TaskHandle, 0, len(o.tasks))
	for _, t := range o.tasks {
		if t.node.Rec.State() == Running {
			running = append(running, t.handle)
		}
	}
	if len(running) > 0 {
		o.mu.Unlock()
		for _, t := range running {
			t.Cancel(reason)
		}
		return
	}
	// Nothing has reached Running yet: the earliest-declared Pending task is
	// the one about to run next (evo-rec.md "one Running child" — pending
	// siblings are named and idle, waiting their turn), so an interrupt
	// before any evidence still cancels that task rather than falling
	// through to Output-level cancel.
	var active *TaskHandle
	for _, t := range o.tasks {
		if t.node.Rec.State() == Pending {
			active = t.handle
			break
		}
	}
	if active != nil {
		o.mu.Unlock()
		active.Cancel(reason)
		return
	}
	o.mu.Unlock()
	o.Cancel(reason)
}

// cancelPendingConfirmLocked cancels one pending Confirm gate, if any, so ^C
// at a "[y/N]" prompt unblocks Confirm's stdin read and resolves the gate as
// Cancelled — never Blocked "declined" (a human "n" and an interrupt are
// distinct outcomes). Reports whether a gate was cancelled.
func (o *Output) cancelPendingConfirmLocked(reason string) bool {
	id, ok := o.graph.AbortPending()
	if !ok {
		return false
	}
	if st := o.taskStates[id]; st != nil && !core.IsTerminalTask(st.node.Rec.State()) {
		st.node.Rec.SetSummary(reason)
		o.settleLocked(st, Cancelled)
		o.commitResolvedTaskLocked(id)
	}
	return true
}
