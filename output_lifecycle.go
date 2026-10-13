package evo

import (
	"context"

	"github.com/zachbornheimer/evident-output/internal/record"
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
	// A Confirm still waiting for an answer would hold its caller forever
	// once the run is gone.
	for o.cancelPendingConfirmLocked(cancelCauseClose) {
	}
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
	o.graph.Close()
	o.mu.Unlock()
	o.graph.Cancel()
	// Finish already wrote the manifest and warned when it could not; this
	// releases the Run's exclusive manifest lock (spec §11.3) and returns
	// any write or release failure. Already committed Task records on disk
	// are unaffected — Close never rolls anything back.
	return o.manifest.Close()
}

// beginRunContext installs ctx (Run/evo.Run's own ctx parameter) as the
// parent of this run's task scopes, replacing the placeholder Init installed
// for Define/Verify calls made before any Run (see graph.Graph.BeginRunContext).
func (o *Output) beginRunContext(ctx context.Context) context.Context {
	return o.graph.BeginRunContext(ctx)
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
	return o.graph.Context()
}

// Conclusion returns the computed conclusion after Finish.
func (o *Output) Conclusion() Conclusion {
	o.mu.Lock()
	defer o.mu.Unlock()
	if c, ok := o.rec.Conclusion(); ok {
		return c
	}
	snap := o.snapshotLocked()
	c := record.InferConclusion(snap)
	o.explainCancellationLocked(&c)
	record.ApplyFailedExitCode(&c, o.cfg.failedExitCode)
	return c
}

// cancelCauseUser is the cause an interrupt signal records: the person at
// the terminal stopped the run.
const cancelCauseUser = "by user"

// cancelCauseClose is the cause a Close records on a Confirm still waiting.
const cancelCauseClose = "run closed"

// explainCancellationLocked names the cancellation cause on a cancelled
// conclusion. Any other outcome keeps its own Explanation untouched.
func (o *Output) explainCancellationLocked(c *record.Conclusion) {
	if c.State == record.StateCancelled && c.Explanation == "" {
		c.Explanation = o.graph.CancelCause()
	}
}
