package engine

import (
	"context"

	"github.com/zachbornheimer/evident-output/internal/core"
)

func (o *Output) ensureOpen() error {
	if o.closed || o.finishing || o.finished {
		return ErrClosed
	}
	return nil
}

// Close is idempotent cleanup; best-effort Finish when needed. The first
// call tears down and returns its error; a concurrent or later call waits
// for that teardown to end and returns nil.
func (o *Output) Close() error {
	o.mu.Lock()
	if o.closing != nil {
		closing := o.closing
		o.mu.Unlock()
		<-closing
		return nil
	}
	o.closing = make(chan struct{})
	defer close(o.closing)
	needFinish := !o.finished
	o.mu.Unlock()
	if needFinish {
		_ = o.Finish()
	}
	o.mu.Lock()
	o.stopSpinnerAnimatorLocked()
	o.stopResizeWatchLocked()
	o.stopPlainHeartbeatsLocked()
	o.closed = true
	cancelRun := o.cancelRun
	manifestStore := o.manifestStore
	o.mu.Unlock()
	if cancelRun != nil {
		cancelRun()
	}
	// Finish already wrote the manifest and warned when it could not; this
	// releases the Run's exclusive manifest lock (spec §11.3) and returns
	// any write or release failure. Already committed Task records on disk
	// are unaffected — Close never rolls anything back.
	return manifestStore.Close()
}

// beginRunContext installs ctx (Run/evo.Run's own ctx parameter) as the
// parent of this run's task scopes, replacing the context.Background()
// Init installed as a placeholder for Define/Verify calls made before any
// Run. Every taskScopeHandle context (see withTaskScope) descends from
// o.Context(), so without this a caller's Run(ctx, ...) cancellation or
// deadline never reached a running Define/Verify — only the Init-time
// background context did (task scopes only ever observed SIGINT/Close via
// cancelRun, never the caller's own ctx or deadline).
//
// The previous run context's cancel is invoked here, not left to leak:
// nothing after this point should still be watching it, and a second Run
// call — on an Output whose caller reuses it after Close resets fields, or
// in a test exercising the mechanism directly — must start every new task
// scope from a fresh, uncancelled context rather than one inheriting a
// prior run's cancellation. Isolated outputs each hold their own o.ctx, so
// this never crosses between them.
func (o *Output) beginRunContext(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	runCtx, cancel := context.WithCancel(ctx)
	o.mu.Lock()
	previousCancel := o.cancelRun
	o.ctx = runCtx
	o.cancelRun = cancel
	o.mu.Unlock()
	if previousCancel != nil {
		previousCancel()
	}
	return runCtx
}

// Context reports the run's cancellation signal. It is cancelled when the
// run is interrupted (SIGINT/SIGTERM) and when the Output closes, so work
// that does I/O can select on it and stop instead of running on past the ^C
// that was supposed to end it. A nil Output reports a never-cancelled
// context so a caller never has to nil-check before selecting.
func (o *Output) Context() context.Context {
	if o == nil {
		return context.Background()
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.ctx == nil {
		return context.Background()
	}
	return o.ctx
}

// Conclusion returns the computed conclusion after Finish.
func (o *Output) Conclusion() Conclusion {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.conclusion != nil {
		return *o.conclusion
	}
	snap := o.snapshotLocked()
	c := core.InferConclusion(snap)
	o.explainCancellationLocked(&c)
	core.ApplyFailedExitCode(&c, o.cfg.failedExitCode)
	return c
}

// cancelCauseUser is the cause an interrupt signal records: the person at
// the terminal stopped the run.
const cancelCauseUser = "by user"

// explainCancellationLocked names the cancellation cause on a cancelled
// conclusion. Any other outcome keeps its own Explanation untouched.
func (o *Output) explainCancellationLocked(c *core.Conclusion) {
	if c.State == core.StateCancelled && c.Explanation == "" {
		c.Explanation = o.cancelCause
	}
}
