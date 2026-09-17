package engine

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestDefine_CallerContextCancellation_CancelsTaskScope pins the run-context
// propagation §7 contract: cancelling the ctx a caller passed to Run must
// cancel a Define callback blocked in flight. Before beginRunContext, every
// task scope descended from context.Background() installed at Init, so a
// caller's own cancellation never reached inside Define.
func TestDefine_CallerContextCancellation_CancelsTaskScope(t *testing.T) {
	out := newTestOutput(t)

	callerCtx, cancelCaller := context.WithCancel(context.Background())
	started := make(chan struct{})
	observed := make(chan error, 1)
	runDone := make(chan struct{})

	go func() {
		out.Run(callerCtx, func(context.Context) error {
			out.Task("wait-for-caller-cancel").Define(func(taskCtx context.Context) error {
				close(started)
				<-taskCtx.Done()
				observed <- taskCtx.Err()
				return taskCtx.Err()
			})
			return nil
		})
		close(runDone)
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("Define never started")
	}

	cancelCaller()

	select {
	case err := <-observed:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled inside Define, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("caller ctx cancellation never reached the running Define")
	}
	<-runDone
}

// TestVerify_CallerContextCancellation_CancelsTaskScope is Define's
// counterpart for a blocked Verify observation.
func TestVerify_CallerContextCancellation_CancelsTaskScope(t *testing.T) {
	out := newTestOutput(t)

	callerCtx, cancelCaller := context.WithCancel(context.Background())
	started := make(chan struct{})
	observed := make(chan error, 1)
	runDone := make(chan struct{})

	go func() {
		out.Run(callerCtx, func(context.Context) error {
			task := out.Task("wait-for-caller-cancel-verify")
			task.Verify(func(verifyCtx context.Context) (bool, error) {
				close(started)
				<-verifyCtx.Done()
				observed <- verifyCtx.Err()
				return false, verifyCtx.Err()
			})
			task.Define(func(context.Context) error { return nil })
			return nil
		})
		close(runDone)
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("Verify never started")
	}

	cancelCaller()

	select {
	case err := <-observed:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled inside Verify, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("caller ctx cancellation never reached the running Verify")
	}
	<-runDone
}

// TestDefine_CallerContextDeadline_IsObservable pins that a deadline set on
// the caller's ctx (not just outright cancellation) is visible inside a
// running Define — the same descent beginRunContext establishes.
func TestDefine_CallerContextDeadline_IsObservable(t *testing.T) {
	out := newTestOutput(t)

	deadline := time.Now().Add(time.Hour)
	callerCtx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	seenDeadline := make(chan time.Time, 1)
	runDone := make(chan struct{})

	go func() {
		out.Run(callerCtx, func(context.Context) error {
			out.Task("observe-deadline").Define(func(taskCtx context.Context) error {
				d, ok := taskCtx.Deadline()
				if !ok {
					seenDeadline <- time.Time{}
					return nil
				}
				seenDeadline <- d
				return nil
			})
			return nil
		})
		close(runDone)
	}()

	select {
	case d := <-seenDeadline:
		if !d.Equal(deadline) {
			t.Fatalf("expected Define's ctx deadline %v, got %v", deadline, d)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Define never observed the caller's ctx deadline")
	}
	<-runDone
}

// TestBeginRunContext_SecondRunStartsFresh pins that a run's task scopes
// never inherit a previous run's cancellation: beginRunContext both
// installs a new context.Context and retires the previous run's cancel, so
// a second Run — after the first was cancelled — starts every task scope
// from a context that is not already Done.
func TestBeginRunContext_SecondRunStartsFresh(t *testing.T) {
	out := newTestOutput(t)

	firstCtx := out.beginRunContext(context.Background())
	if err := firstCtx.Err(); err != nil {
		t.Fatalf("first run context should start uncancelled, got %v", err)
	}

	secondCtx := out.beginRunContext(context.Background())

	if err := firstCtx.Err(); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected the first run's context to be retired (Canceled) once superseded, got %v", err)
	}
	if err := secondCtx.Err(); err != nil {
		t.Fatalf("second run context should start fresh/uncancelled, got %v", err)
	}
	if out.Context() != secondCtx {
		t.Fatal("o.Context() must report the most recently begun run's context")
	}
}

func newTestOutput(t *testing.T) *Output {
	t.Helper()
	out := Init(Config{Isolated: true, Stdout: discardWriter{}})
	t.Cleanup(func() { _ = out.Close() })
	return out
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
