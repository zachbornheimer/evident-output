package engine

import (
	"context"
	"errors"
	"sync"

	"github.com/zachbornheimer/evident-output/internal/core"
)

// interruption names why a run stopped early: reason is the text each
// cancelled row carries, cause is the cancelled Conclusion's Explanation,
// and code is the same cause as a stable machine code (the "evo.run"
// document's cancellation.cause), so the band and the wire agree. err is
// what context.Cause reports on every Task scope the interrupt cancels.
type interruption struct {
	reason string
	cause  string
	code   core.CancelCause
	err    error
}

var (
	// interruptionBySignal: the person at the terminal pressed ^C.
	interruptionBySignal = interruption{reason: "interrupted", cause: "by user", code: core.CancelCauseUser, err: context.Canceled}
	// interruptionByCaller: the caller cancelled an embedded Run's context —
	// an HTTP client disconnected, or the embedder shut the request down.
	interruptionByCaller = interruption{reason: "cancelled", cause: "by caller", code: core.CancelCauseCaller, err: context.Canceled}
	// interruptionByDeadline: an embedded Run's caller deadline passed.
	interruptionByDeadline = interruption{reason: "deadline exceeded", cause: "deadline exceeded", code: core.CancelCauseDeadline, err: context.DeadlineExceeded}
)

// interrupt stops the run at the first signal or at the end of the
// caller's context, in the one order that leaves the ledger honest: the
// scheduler is closed to new work, every row is put into the state the
// reader must see, and only then is the run's context cancelled to release
// the callbacks still in flight. Cancelling first would race a finishing
// callback into a ✓ row after the ^C.
func (o *Output) interrupt(why interruption) {
	if o == nil {
		return
	}
	o.mu.Lock()
	if o.stopsNothingLocked() {
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
		cancelRun(why.err)
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

// stopsNothingLocked reports whether an interrupt arriving now has no work
// to stop: the run already concluded (Finish fixed its Conclusion, so a
// late ^C must not rewrite the Output behind the Result Run returns), or
// an embedded run already settled (DEC-CANCEL-004).
func (o *Output) stopsNothingLocked() bool {
	return o.finished || o.settledLocked()
}

// callerInterruption classifies why the caller's context ended.
func callerInterruption(err error) interruption {
	if errors.Is(err, context.DeadlineExceeded) {
		return interruptionByDeadline
	}
	return interruptionByCaller
}

// scopeCaller is the context this run's Tasks descend from. A run that did
// not opt into Embedded hands Tasks the caller's ctx unchanged (the 1.1
// contract: its end fails the running Define). An Embedded run hands them
// the caller's values without its cancellation or its deadline: both reach
// the run only through interrupt (DEC-CANCEL-002/006). A Task that saw the
// deadline could time itself out (as net.Dialer does) and fail its row
// before the interrupt marks it cancelled. context.Cause on a Task's ctx
// still reports context.DeadlineExceeded when the deadline stopped the run.
func (o *Output) scopeCaller(ctx context.Context) context.Context {
	if !o.cfg.embedded {
		return ctx
	}
	return context.WithoutCancel(ctx)
}

// callerWatch turns the end of an embedded run's caller context into the
// same ordered interrupt a ^C performs.
type callerWatch struct {
	callerErr func() error
	interrupt func()
	stop      func() bool
}

// inertCallerWatch never interrupts: a CLI run's ctx is not its lifecycle.
var inertCallerWatch = callerWatch{
	callerErr: func() error { return nil },
	interrupt: func() {},
	stop:      func() bool { return false },
}

// watchCaller interrupts an embedded out once, when ctx ends, until release.
func (o *Output) watchCaller(ctx context.Context) callerWatch {
	if !o.cfg.embedded {
		return inertCallerWatch
	}
	interrupt := sync.OnceFunc(func() { o.interrupt(callerInterruption(ctx.Err())) })
	return callerWatch{callerErr: ctx.Err, interrupt: interrupt, stop: context.AfterFunc(ctx, interrupt)}
}

// ended reports whether the watched caller context has ended.
func (w callerWatch) ended() bool { return w.callerErr() != nil }

// interruptIfEnded reports whether the caller's context has ended and, if
// so, waits until its interrupt has been applied — a run that returned
// because its context ended concludes only after its rows say cancelled.
func (w callerWatch) interruptIfEnded() bool {
	if !w.ended() {
		return false
	}
	w.interrupt()
	return true
}

// release stops watching: a run that already concluded is not interrupted.
func (w callerWatch) release() { w.stop() }

// endRunCallback records that the run callback returned. If an embedded
// run's caller ctx was still live at that moment, the run is settling:
// what remains is whatever its Tasks still do, so an interrupt that later
// finds every Task terminal has nothing left to stop (see settledLocked).
// Checking callerEnded under o.mu orders this against a racing interrupt:
// a caller that ended first always cancels. A CLI run never settles — a ^C
// stops it at any point before it concludes (DEC-CANCEL-004 is scoped to
// embedded runs).
func (o *Output) endRunCallback(callerEnded func() bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.runSettling = o.cfg.embedded && !callerEnded()
}

// settledLocked reports whether an interrupt would stop nothing: the run
// callback returned while the caller was live and every Task is terminal.
func (o *Output) settledLocked() bool {
	if !o.runSettling {
		return false
	}
	for _, t := range o.tasks {
		if !core.IsTerminalTask(t.state) {
			return false
		}
	}
	return true
}
