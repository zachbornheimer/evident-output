package evo_test

// 1.2 keeps the 1.1 caller-context contract on every format
// (DEC-CANCEL-005): the ctx passed to Run reaches Tasks unchanged, and its
// end fails the running Define. A caller-owned lifecycle needs new public
// surface and is deferred behind ZYS-947.

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

// callerBudget is a caller deadline short enough to end mid-Define.
const callerBudget = 20 * time.Millisecond

// A run keeps the 1.1 verdict when its ctx ends, on every format: the
// Define sees the caller's cancellation and the run concludes failed
// (exit 2), never cancelled (130). FormatExternal is a rendering choice,
// not a lifecycle one (DEC-CANCEL-005).
func TestOutputRun_CallerCancelKeeps11Verdict(t *testing.T) {
	formats := map[string]evo.Format{"human": evo.FormatHuman, "json": evo.FormatJSON, "external": evo.FormatExternal}
	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			out := evo.Init(evo.Config{Isolated: true, Plain: true, Format: format, Stdout: io.Discard, Stderr: io.Discard})
			result := out.Run(ctx, func(context.Context) error {
				out.Task("wait").Define(func(taskCtx context.Context) error {
					cancel()
					<-taskCtx.Done()
					return taskCtx.Err()
				})
				return nil
			})
			if result.Conclusion.State != evo.StateFailed || result.ExitCode() != evo.ExitFailed {
				t.Fatalf("conclusion = %s/%d, want %s/%d (1.1 contract)", result.Conclusion.State, result.ExitCode(), evo.StateFailed, evo.ExitFailed)
			}
		})
	}
}

// errNoCallerDeadline is what a Task reports when the caller's deadline
// did not reach it.
var errNoCallerDeadline = errors.New("caller deadline did not reach the Task")

// A run hands Tasks the caller's deadline, as in 1.1: the Task sees
// ctx.Deadline(), its ctx ends with DeadlineExceeded, and the run
// concludes failed (exit 2).
func TestOutputRun_CallerDeadlineReachesTasks(t *testing.T) {
	formats := map[string]evo.Format{"human": evo.FormatHuman, "json": evo.FormatJSON, "external": evo.FormatExternal}
	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), callerBudget)
			defer cancel()
			out := evo.Init(evo.Config{Isolated: true, Plain: true, Format: format, Stdout: io.Discard, Stderr: io.Discard})
			taskErr := make(chan error, 1)
			result := out.Run(ctx, func(context.Context) error {
				out.Task("dial").Define(func(taskCtx context.Context) error {
					if _, ok := taskCtx.Deadline(); !ok {
						taskErr <- errNoCallerDeadline
						return errNoCallerDeadline
					}
					<-taskCtx.Done()
					taskErr <- taskCtx.Err()
					return taskCtx.Err()
				})
				return nil
			})
			if got := <-taskErr; !errors.Is(got, context.DeadlineExceeded) {
				t.Fatalf("Task ctx error = %v, want context.DeadlineExceeded (1.1 contract)", got)
			}
			if result.Conclusion.State != evo.StateFailed || result.ExitCode() != evo.ExitFailed {
				t.Fatalf("conclusion = %s/%d, want %s/%d (1.1 contract)", result.Conclusion.State, result.ExitCode(), evo.StateFailed, evo.ExitFailed)
			}
		})
	}
}
