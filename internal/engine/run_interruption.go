package engine

import (
	"context"
	"errors"
	"sync"
	"time"
)

// interruption names why a run stopped early: reason is the text each
// cancelled row carries, cause is the cancelled Conclusion's Explanation,
// so the band and the JSON document state the same cause.
type interruption struct {
	reason string
	cause  string
}

var (
	// interruptionBySignal: the person at the terminal pressed ^C.
	interruptionBySignal = interruption{reason: "interrupted", cause: "by user"}
	// interruptionByCaller: the caller cancelled the Run's context — an
	// HTTP client disconnected, or the embedder shut the request down.
	interruptionByCaller = interruption{reason: "cancelled", cause: "by caller"}
	// interruptionByDeadline: the caller's context deadline passed.
	interruptionByDeadline = interruption{reason: "deadline exceeded", cause: "deadline exceeded"}
)

// callerInterruption classifies why the caller's context ended.
func callerInterruption(err error) interruption {
	if errors.Is(err, context.DeadlineExceeded) {
		return interruptionByDeadline
	}
	return interruptionByCaller
}

// callerWatch turns the end of the caller's context into the same ordered
// interrupt a ^C performs. The run's own context does not inherit the
// caller's cancellation directly (see detachCancellation): if it did, a
// Define would observe Done and fail its row before interrupt had marked
// it cancelled.
type callerWatch struct {
	callerErr func() error
	interrupt func()
	stop      func() bool
}

// watchCaller interrupts out once, when ctx ends, until release.
func watchCaller(ctx context.Context, out *Output) callerWatch {
	interrupt := sync.OnceFunc(func() { out.interrupt(callerInterruption(ctx.Err())) })
	return callerWatch{callerErr: ctx.Err, interrupt: interrupt, stop: context.AfterFunc(ctx, interrupt)}
}

// ended reports whether the caller's context has ended and, if so, waits
// until its interrupt has been applied — a run that returned because its
// context ended concludes only after its rows say cancelled.
func (w callerWatch) ended() bool {
	if w.callerErr() == nil {
		return false
	}
	w.interrupt()
	return true
}

// release stops watching: a run that already concluded is not interrupted.
func (w callerWatch) release() { w.stop() }

// callerScope exposes the caller's values and deadline to Define/Verify
// without its cancellation, which reaches the run only through interrupt.
type callerScope struct {
	deadline func() (time.Time, bool)
	value    func(key any) any
}

// detachCancellation wraps the caller's ctx as a callerScope.
func detachCancellation(ctx context.Context) context.Context {
	return callerScope{deadline: ctx.Deadline, value: ctx.Value}
}

// Deadline reports the caller's deadline so task code can budget against it.
func (s callerScope) Deadline() (time.Time, bool) { return s.deadline() }

// Done is nil: the caller's cancellation never ends the scope directly.
func (callerScope) Done() <-chan struct{} { return nil }

// Err is always nil for the same reason.
func (callerScope) Err() error { return nil }

// Value reads through to the caller's values.
func (s callerScope) Value(key any) any { return s.value(key) }
