package evo_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

const (
	// callerDeadline is the deadline the caller gives Execute.
	callerDeadline = 150 * time.Millisecond
	// cancelledWithin bounds how long after the deadline Execute may return.
	cancelledWithin = time.Second
)

// TestExecuteHonoursCallerContext pins CONTRACT.md §1: Execute binds the
// caller's ctx for the whole run, including the drain after the body
// returned, so a deadline reaches a running callback and the run ends
// Cancelled with exit 130.
func TestExecuteHonoursCallerContext(t *testing.T) {
	for _, mode := range contractOutputs {
		t.Run(outputName[mode], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), callerDeadline)
			defer cancel()
			var reached atomic.Bool
			start := time.Now()
			run := executeContract(ctx, t, evo.Config{Name: "caller ctx", Output: mode},
				func(_ context.Context, r *evo.Run) error {
					r.Task("wait").Define(func(taskCtx context.Context) error {
						select {
						case <-taskCtx.Done():
							reached.Store(true)
							return taskCtx.Err()
						case <-time.After(callbackPatience):
							return errors.New("the caller's deadline never reached the callback")
						}
					})
					return nil
				})
			took := time.Since(start)
			if !reached.Load() {
				t.Error("the caller's deadline never reached the Define ctx")
			}
			if took > callerDeadline+cancelledWithin {
				t.Errorf("Execute returned after %s, want within %s of the %s deadline",
					took.Round(time.Millisecond), cancelledWithin, callerDeadline)
			}
			if got := run.Result.Conclusion.Outcome; got != evo.Outcome(evo.Cancelled) {
				t.Errorf("Conclusion.Outcome = %q, want %q", got, evo.Cancelled)
			}
			if got := run.Result.ExitCode(); got != 130 {
				t.Errorf("Result.ExitCode() = %d, want 130", got)
			}
		})
	}
}
