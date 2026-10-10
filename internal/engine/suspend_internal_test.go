package engine

import (
	"context"
	"io"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// suspendQuiet is how long a test watches for work that must not start.
const suspendQuiet = 50 * time.Millisecond

func TestSuspend_StartsNoNewTaskUntilTheWindowEnds(t *testing.T) {
	out := Init(Config{Isolated: true, Stdout: io.Discard, Stderr: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	var ran atomic.Bool
	var task *TaskHandle

	_ = out.Suspend(func() error {
		task = out.Task("queued").Define(func(context.Context) error { ran.Store(true); return nil })
		time.Sleep(suspendQuiet)
		if ran.Load() {
			t.Error("a Task started inside the Suspend window")
		}
		return nil
	})

	if err := task.Wait(); err != nil || !ran.Load() {
		t.Errorf("after the window: Wait = %v, ran = %v, want the Task to run", err, ran.Load())
	}
}

func TestConfirm_StartsNoNewTaskWhileTheQuestionIsOpen(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	defer func() { _ = w.Close() }()
	out := newOutput("", to(io.Discard), withNoColor(), stdin(r))
	var ran atomic.Bool
	answered := make(chan bool)
	go func() { answered <- out.Confirm("proceed?") }()
	for !out.graph.Suspended() {
		time.Sleep(time.Millisecond)
	}

	task := out.Task("queued").Define(func(context.Context) error { ran.Store(true); return nil })
	time.Sleep(suspendQuiet)
	if ran.Load() {
		t.Error("a Task started while the Confirm question was open")
	}
	_, _ = w.Write([]byte("y\n"))

	if !<-answered {
		t.Error("Confirm = false, want true for y")
	}
	if err := task.Wait(); err != nil || !ran.Load() {
		t.Errorf("after the answer: Wait = %v, ran = %v, want the Task to run", err, ran.Load())
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
