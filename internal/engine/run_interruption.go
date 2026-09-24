package engine

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/zachbornheimer/evident-output/internal/core"
)

// interruption names why a run stopped early: reason is the text each
// cancelled row carries, cause is the cancelled Conclusion's Explanation,
// so the band and the JSON document state the same cause. err is what
// context.Cause reports on every Task scope the interrupt cancels.
type interruption struct {
	reason string
	cause  string
	err    error
}

var (
	// interruptionBySignal: the person at the terminal pressed ^C.
	interruptionBySignal = interruption{reason: "interrupted", cause: "by user", err: context.Canceled}
	// interruptionByCaller: the caller cancelled an embedded Run's context —
	// an HTTP client disconnected, or the embedder shut the request down.
	interruptionByCaller = interruption{reason: "cancelled", cause: "by caller", err: context.Canceled}
	// interruptionByDeadline: an embedded Run's caller deadline passed.
	interruptionByDeadline = interruption{reason: "deadline exceeded", cause: "deadline exceeded", err: context.DeadlineExceeded}
)

// callerInterruption classifies why the caller's context ended.
func callerInterruption(err error) interruption {
	if errors.Is(err, context.DeadlineExceeded) {
		return interruptionByDeadline
	}
	return interruptionByCaller
}

// scopeCaller is the context this run's Tasks descend from. A CLI run
// hands Tasks the caller's ctx unchanged (the 1.1 contract: its end fails
// the running Define). An embedded run hands them a callerScope, so the
// caller's end reaches Tasks only through interrupt (DEC-CANCEL-002/005).
func (o *Output) scopeCaller(ctx context.Context) context.Context {
	if !o.cfg.embedded {
		return ctx
	}
	return callerScope{value: ctx.Value}
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

// callerScope exposes the caller's values to Define/Verify, and neither
// its cancellation nor its deadline. Both reach the run only through
// interrupt: a Task that saw the deadline could time itself out (as
// net.Dialer does) and fail its row before the interrupt marks it
// cancelled (DEC-CANCEL-006). context.Cause on a Task's ctx still reports
// context.DeadlineExceeded when the deadline is why the run stopped.
type callerScope struct {
	value func(key any) any
}

// Deadline is unset; see callerScope.
func (callerScope) Deadline() (deadline time.Time, ok bool) { return }

// Done is nil: the caller's cancellation never ends the scope directly.
func (callerScope) Done() <-chan struct{} { return nil }

// Err is always nil for the same reason.
func (callerScope) Err() error { return nil }

// Value reads through to the caller's values.
func (s callerScope) Value(key any) any { return s.value(key) }

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
