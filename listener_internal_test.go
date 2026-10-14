package evo

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// lockTakingListener takes the Output's lock on every Task change before it
// hands the change to the engine's own listener, the way a handler that
// repaints render state must.
type lockTakingListener struct {
	o     *Run
	inner record.Listener
	heard chan struct{}
}

func (l lockTakingListener) TaskChanged(id record.TaskID, from, to record.State) {
	l.o.mu.Lock()
	select {
	case l.heard <- struct{}{}:
	default:
	}
	l.o.mu.Unlock()
	l.inner.TaskChanged(id, from, to)
}

func (l lockTakingListener) TaskSettled(id record.TaskID, from, to record.State) {
	l.o.mu.Lock()
	select {
	case l.heard <- struct{}{}:
	default:
	}
	l.o.mu.Unlock()
	l.inner.TaskSettled(id, from, to)
}

func newListenerTestOutput(t *testing.T) *Run {
	t.Helper()
	o := initRun(Config{stdout: io.Discard, stderr: io.Discard})
	t.Cleanup(func() { _ = o.close() })
	return o
}

// TestListenerTakingTheOutputLockDoesNotDeadlockInsideDefine proves a
// handler that takes Output.mu runs after the writer released it: a Define
// callback settles its own Task from inside a critical section, and the
// handler for that settle takes the same lock.
func TestListenerTakingTheOutputLockDoesNotDeadlockInsideDefine(t *testing.T) {
	o := newListenerTestOutput(t)
	heard := make(chan struct{}, 1)
	o.rec.Listen(lockTakingListener{o: o, inner: o.show, heard: heard})

	done := make(chan error, 1)
	go func() {
		task := o.Task("settles inside define")
		task.Define(func(context.Context) error {
			task.fail("by the callback")
			return nil
		})
		done <- task.wait()
	}()

	select {
	case <-done:
	case <-time.After(waitOutcomeTimeout):
		t.Fatal("a listener taking Output.mu deadlocked the writer that settled the Task")
	}
	select {
	case <-heard:
	default:
		t.Error("the lock-taking listener never heard the settle")
	}
}
