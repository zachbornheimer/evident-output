package evo_test

// Scope of the 1.2 caller-context change (DEC-CANCEL-005/006): only a run
// whose Config opts in with Embedded treats the end of its caller's ctx as
// an interrupt. Every other run, FormatExternal included, keeps the 1.1
// contract, so a minor release changes no existing caller's exit code.

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

// callerBudget is a caller deadline short enough to end mid-Define.
const callerBudget = 20 * time.Millisecond

// A run that did not opt in keeps the 1.1 verdict when its ctx ends, on
// every format: the Define sees the caller's cancellation and the run
// concludes failed (exit 2), never cancelled (130). FormatExternal is a
// rendering choice, not a lifecycle one (DEC-CANCEL-005).
func TestOutputRun_CallerCancelWithoutEmbeddedKeeps11Verdict(t *testing.T) {
	formats := map[string]evo.Format{"human": evo.FormatHuman, "external": evo.FormatExternal}
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

// A run that did not opt in hands Tasks the caller's deadline, as in 1.1:
// the Task sees ctx.Deadline(), its ctx ends with DeadlineExceeded, and the
// run concludes failed (exit 2). Only Embedded hides it (DEC-CANCEL-006).
func TestOutputRun_CallerDeadlineWithoutEmbeddedReachesTasks(t *testing.T) {
	formats := map[string]evo.Format{"human": evo.FormatHuman, "external": evo.FormatExternal}
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

// Embedded is the whole opt-in, independent of Format: a host that
// streams the FormatJSON document itself gets the same lifecycle as a
// FormatExternal one, and the document names the caller as the cause.
func TestOutputRun_EmbeddedOptsAnyFormatIntoCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var stdout bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Embedded: true, Format: evo.FormatJSON, Stdout: &stdout, Stderr: io.Discard})
	result := out.Run(ctx, func(context.Context) error {
		out.Task("wait").Define(func(taskCtx context.Context) error {
			cancel()
			<-taskCtx.Done()
			return taskCtx.Err()
		})
		return nil
	})
	if result.Conclusion.State != evo.StateCancelled || result.ExitCode() != evo.ExitCancelled {
		t.Fatalf("conclusion = %s/%d, want %s/%d", result.Conclusion.State, result.ExitCode(), evo.StateCancelled, evo.ExitCancelled)
	}
	if doc := decodeRunDoc(t, stdout.Bytes()); doc.Cancellation == nil || doc.Cancellation.Cause != "caller" {
		t.Fatalf("cancellation = %+v, want cause caller\n%s", doc.Cancellation, stdout.Bytes())
	}
}

// deadlineAwareCallee behaves like net.Dialer: it derives its own timer
// from ctx.Deadline() and fails with a timeout when that timer fires,
// before any cancellation reaches it. Without a deadline it waits on Done.
func deadlineAwareCallee(ctx context.Context) error {
	if deadline, ok := ctx.Deadline(); ok {
		select {
		case <-time.After(time.Until(deadline)):
			return errors.New("i/o timeout")
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	<-ctx.Done()
	return ctx.Err()
}

// An embedded run hides its caller's deadline from Tasks: a deadline-aware
// callee must not fail its row before the interrupt marks it cancelled.
// context.Cause still says why the run stopped.
func TestOutputRun_EmbeddedHidesCallerDeadlineFromTasks(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), callerBudget)
	defer cancel()
	out := embedderOutput()
	sawDeadline := make(chan bool, 1)
	cause := make(chan error, 1)
	result := out.Run(ctx, func(context.Context) error {
		out.Task("dial").Define(func(taskCtx context.Context) error {
			_, ok := taskCtx.Deadline()
			sawDeadline <- ok
			err := deadlineAwareCallee(taskCtx)
			cause <- context.Cause(taskCtx)
			return err
		})
		return nil
	})

	if <-sawDeadline {
		t.Fatal("an embedded Task saw the caller's deadline; a deadline-aware callee can then fail before the interrupt")
	}
	if got := <-cause; !errors.Is(got, context.DeadlineExceeded) {
		t.Fatalf("context.Cause(taskCtx) = %v, want context.DeadlineExceeded", got)
	}
	if result.Conclusion.State != evo.StateCancelled {
		t.Fatalf("conclusion = %s, want %s", result.Conclusion.State, evo.StateCancelled)
	}
	if got := taskState(out, "dial"); got != evo.Cancelled {
		t.Fatalf("row dial = %s, want %s", got, evo.Cancelled)
	}
}

// taskState reports the final state of the named Task on out.
func taskState(out *evo.Output, name string) evo.EntityState {
	for _, task := range out.Snapshot().Tasks {
		if task.Name == name {
			return task.State
		}
	}
	return ""
}
