package engine

import (
	"context"
	"io"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// suspendHangLimit is how long a test waits for work that must happen before
// it calls the window hung.
const suspendHangLimit = 3 * time.Second

// startsInsideWindow waits for a Task defined inside a Suspend window to
// start before the window closes.
func startsInsideWindow(t *testing.T, started <-chan struct{}) {
	t.Helper()
	select {
	case <-started:
	case <-time.After(suspendHangLimit):
		t.Error("a Task that became eligible inside the window did not start before it closed")
	}
}

func TestSuspend_StartsAnEligibleSiblingBeforeTheWindowCloses(t *testing.T) {
	out := Init(Config{Isolated: true, Stdout: io.Discard, Stderr: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	started := make(chan struct{})

	_ = out.Suspend(func() error {
		out.Task("sibling").Define(func(context.Context) error { close(started); return nil })
		startsInsideWindow(t, started)
		return nil
	})
}

func TestConfirm_StartsAnEligibleSiblingWhileTheQuestionIsOpen(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	defer func() { _ = w.Close() }()
	out := newOutput("", to(io.Discard), withNoColor(), stdin(r))
	answered := make(chan bool)
	go func() { answered <- out.Confirm("proceed?") }()
	for out.graph.PendingAborts() == 0 {
		time.Sleep(time.Millisecond)
	}
	started := make(chan struct{})

	out.Task("sibling").Define(func(context.Context) error { close(started); return nil })
	startsInsideWindow(t, started)
	_, _ = w.Write([]byte("y\n"))

	if !<-answered {
		t.Error("Confirm = false, want true for y")
	}
}

// suspendWindowHold is how long a test keeps a Suspend window open after its
// sibling started, so a frame painted inside it has time to land.
const suspendWindowHold = 150 * time.Millisecond

// TestSuspend_PaintsNoFrameInsideTheWindowWhenASiblingStartsInIt pins that
// the window owns the surface: a sibling that starts inside it runs at once
// but its row does not paint over the prompt until the window closes.
func TestSuspend_PaintsNoFrameInsideTheWindowWhenASiblingStartsInIt(t *testing.T) {
	surface := &paintRecorder{}
	out := newOutput("job", withTerminal(surface), visibilityDelay(0), withNoColor(), maxConcurrency(4))
	t.Cleanup(func() { _ = out.Close() })
	release := make(chan struct{})
	out.Task("slow").Define(func(context.Context) error { <-release; return nil })
	for deadline := time.Now().Add(suspendHangLimit); len(surface.written()) == 0 && time.Now().Before(deadline); {
		time.Sleep(time.Millisecond)
	}
	var framesInside int

	_ = out.Suspend(func() error {
		before := len(surface.written())
		started := make(chan struct{})
		out.Task("sibling").Define(func(context.Context) error { close(started); return nil })
		startsInsideWindow(t, started)
		time.Sleep(suspendWindowHold)
		framesInside = len(surface.written()) - before
		return nil
	})
	close(release)

	if framesInside != 0 {
		t.Errorf("%d frames painted inside the Suspend window, want none", framesInside)
	}
}

func TestSuspend_CallbackErrorPropagates(t *testing.T) {
	out := Init(Config{Isolated: true})
	t.Cleanup(func() { _ = out.Close() })
	err := out.Suspend(func() error { return io.EOF })
	if err != io.EOF {
		t.Fatal(err)
	}
}

func TestSuspend_NestedDoesNotPanic(t *testing.T) {
	out := Init(Config{Isolated: true})
	t.Cleanup(func() { _ = out.Close() })
	_ = out.Suspend(func() error {
		return out.Suspend(func() error { return nil })
	})
}

// suspendThenWait runs wait inside a Suspend window and fails the test when
// the window never returns.
func suspendThenWait(t *testing.T, out *Output, wait func() error) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- out.Suspend(wait) }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Wait inside Suspend = %v, want nil", err)
		}
	case <-time.After(suspendHangLimit):
		t.Fatal("Wait inside a Suspend window hung: the work it awaits never started")
	}
}

func TestSuspend_WaitOnWorkDefinedInsideTheWindowReturns(t *testing.T) {
	out := newOutput("job", to(io.Discard))
	t.Cleanup(func() { _ = out.Close() })

	suspendThenWait(t, out, func() error {
		return out.Task("x").Define(func(context.Context) error { return nil }).Wait()
	})
}

func TestSuspend_WaitOnQueuedWorkBehindAFullPoolReturns(t *testing.T) {
	out := newOutput("job", to(io.Discard), maxConcurrency(1))
	t.Cleanup(func() { _ = out.Close() })
	release := make(chan struct{})
	var holderRan atomic.Bool
	out.Task("holder").Define(func(context.Context) error { <-release; holderRan.Store(true); return nil })
	queued := out.Task("queued").Define(func(context.Context) error { return nil })
	time.AfterFunc(50*time.Millisecond, func() { close(release) })

	suspendThenWait(t, out, queued.Wait)

	if !holderRan.Load() {
		t.Error("the Task holding the only slot never finished")
	}
}
