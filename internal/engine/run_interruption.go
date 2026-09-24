package engine

import "github.com/zachbornheimer/evident-output/internal/core"

// interruption names why a run stopped early: reason is the text each
// cancelled row carries, cause is the cancelled Conclusion's Explanation,
// and code is the same cause as a stable machine code (the "evo.run"
// document's cancellation.cause), so the band and the wire agree.
type interruption struct {
	reason string
	cause  string
	code   core.CancelCause
}

// interruptionBySignal: the person at the terminal pressed ^C (or the
// process got SIGTERM). It is the only interruption 1.2 has; a caller-owned
// lifecycle is deferred behind ZYS-947 (DEC-CANCEL-005).
var interruptionBySignal = interruption{reason: "interrupted", cause: "by user", code: core.CancelCauseUser}

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
	o.schedCancelled = true
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
// conclusion: Explanation in words (unless something more specific already
// explained it) and the machine code the wire document carries. Any other
// outcome is left untouched.
func (o *Output) explainCancellationLocked(c *core.Conclusion) {
	if c.State != core.StateCancelled {
		return
	}
	if c.Explanation == "" {
		c.Explanation = o.cancelledBy.cause
	}
	core.SetCancelCause(c, o.cancelledBy.code)
}
