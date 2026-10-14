package evo

import (
	"context"
	"io"
	"sync/atomic"
	"testing"
	"time"
)

// TestWait_FromOutsideAnyCallbackStillBlocks pins the pattern the deadlock
// rule must never mistake for a cycle: a plain caller — no callback of its
// own to hold still — waiting on a task the scheduler is genuinely running.
// Nobody is stuck, so the wait must block until the work finishes.
func TestWait_FromOutsideAnyCallbackStillBlocks(t *testing.T) {
	t.Parallel()
	out := isolatedScheduler(t, 1, io.Discard, false)

	running := make(chan struct{})
	release := make(chan struct{})
	var ran atomic.Bool
	task := out.Task("slow")
	task.Define(func(ctx context.Context) error {
		close(running)
		<-release
		ran.Store(true)
		return nil
	})

	<-running
	waited := make(chan error, 1)
	go func() { waited <- task.wait() }()

	select {
	case err := <-waited:
		t.Fatalf("Wait returned %v while the task was still running", err)
	case <-time.After(waitProbe):
	}

	close(release)
	var waitErr error
	withinBudget(t, "Wait on a running task from outside any callback", func() {
		waitErr = <-waited
		if err := out.finish(); err != nil {
			t.Errorf("Finish: %v", err)
		}
	})

	if waitErr != nil {
		t.Fatalf("Wait = %v, want nil", waitErr)
	}
	if !ran.Load() {
		t.Fatal("the awaited callback never completed")
	}
	if got := task.snapshot().State; got != succeeded {
		t.Fatalf("task state = %v, want Done", got)
	}
}

// waitProbe is how long a wait that must still be blocking is observed
// before the test accepts that it is. Short: it costs every run this long,
// and a deadlock rule that fires at all fires immediately.
const waitProbe = 100 * time.Millisecond
