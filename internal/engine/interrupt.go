package engine

import (
	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// interruption names why a run stopped early: reason is the text each
// cancelled row carries, and cause is the cancelled Conclusion's
// Explanation, so the band and the JSON document state the same cause.
type interruption struct {
	reason string
	cause  string
}

// interruptionBySignal: the person at the terminal pressed ^C (or the
// process got SIGTERM). It is the only interruption 1.2 has; a
// caller-owned lifecycle is deferred behind ZYS-947 (DEC-CANCEL-005).
var interruptionBySignal = interruption{reason: "interrupted", cause: "by user"}

// interrupt stops the run at the first signal, in the one order that
// leaves the ledger honest: the scheduler is closed to new work, every row
// is put into the state the reader must see, and only then is the run's
// context cancelled to release the callbacks still in flight. Cancelling
// first would race a finishing callback into a ✓ row after the ^C. Once
// Finish has fixed the Conclusion there is nothing left to stop, so a late
// interrupt never rewrites the Output behind the Result Run returned.
func (o *Output) interrupt(why interruption) {
	if o == nil {
		return
	}
	o.mu.Lock()
	if o.finished {
		o.mu.Unlock()
		return
	}
	o.sched.cancelled = true
	o.cancelledBy = why
	cancelRun := o.cancelRun
	o.mu.Unlock()

	o.cancelActive(why.reason)
	o.abandonQueuedWork()

	if cancelRun != nil {
		cancelRun()
	}
}

// explainCancellationLocked names the interruption's cause on a cancelled
// conclusion, unless something more specific already explained it. Any
// other outcome keeps its own Explanation untouched.
func (o *Output) explainCancellationLocked(c *core.Conclusion) {
	if c.State == core.StateCancelled && c.Explanation == "" {
		c.Explanation = o.cancelledBy.cause
	}
}

// abandonQueuedWork resolves every task that had not begun as NotStarted —
// the interrupt's answer to "and what about the rest?", which the reader
// would otherwise never get.
//
// Submitted or not: a task the caller declared and never Defined is work the
// interrupt took away just as surely as one sitting in the scheduler's
// queue. Sweeping only the submitted ones left a declared row Pending, and
// Finish then charged the caller with ErrUnresolvedTask and told them to
// "call Define, Fail, Block, or Skipped on this task" about a
// run the user had just cancelled.
func (o *Output) abandonQueuedWork() {
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, st := range o.tasks {
		if st.sched.phase == phaseRunning || core.IsTerminalTask(st.state) {
			continue
		}
		o.markNotStartedLocked(st)
	}
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
		if t.state == Running {
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
		if t.state == Pending {
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
	for id, abort := range o.confirmAbort {
		close(abort)
		delete(o.confirmAbort, id)
		if st := o.taskByRef[id]; st != nil && !core.IsTerminalTask(st.state) {
			st.summary = txt.Text(reason)
			o.settleLocked(st, Cancelled)
			o.commitResolvedTaskLocked(id)
		}
		return true
	}
	return false
}
