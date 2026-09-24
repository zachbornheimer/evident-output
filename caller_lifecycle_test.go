package evo_test

// Scope of the 1.2 caller-context change (DEC-CANCEL-005/006): only an
// embedded FormatExternal run treats the end of its caller's ctx as an
// interrupt. Every other format keeps the 1.1 contract, so a minor release
// changes no existing caller's exit code.

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

// A human-format run whose ctx ends keeps the 1.1 verdict: the Define sees
// the caller's cancellation and the run concludes failed (exit 2), never
// cancelled (130).
func TestOutputRun_NonExternalCallerCancelKeeps11Verdict(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard})
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
func TestOutputRun_ExternalHidesCallerDeadlineFromTasks(t *testing.T) {
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
