package engine

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// A Wait seals the same inputs whatever other Waits ran before it: a
// member declared since an earlier Wait walked the shared predecessor is
// still found.
func TestWaitSealsTheSameAfterAnEarlierWait(t *testing.T) {
	for _, warm := range []bool{false, true} {
		out := isolatedOutput(t)
		release := make(chan struct{})
		var once sync.Once
		releaseM1 := func() { once.Do(func() { close(release) }) }
		t.Cleanup(releaseM1) // runs before out.Close, which waits for m1
		g := out.Group("g")
		g.Task("m1").Define(func(context.Context) error { <-release; return nil })
		b := out.Task("b").After(g).Define(noop)
		if warm {
			d := out.Task("d").After(b).Define(noop)
			go func() { _ = d.Wait() }()
			waitForParkedWait(t, out)
		}
		g.Task("m2") // never Defined
		c := out.Task("c").After(b).Define(noop)
		err := waitWithin(t, "c.Wait", c.Wait)
		releaseM1()
		if !errors.Is(err, ErrNotStarted) {
			t.Errorf("warm=%v: c.Wait = %v, want ErrNotStarted", warm, err)
		}
	}
}

// waitForParkedWait returns once some goroutine is parked in Wait.
func waitForParkedWait(t *testing.T, out *Output) {
	t.Helper()
	deadline := time.Now().Add(waitReturnDeadline)
	for time.Now().Before(deadline) {
		out.mu.Lock()
		parked := len(out.sched.waits) > 0
		out.mu.Unlock()
		if parked {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("no Wait parked")
}

// Waiting on a Task nobody Defined settles it NotStarted, exactly as
// reaching it as a predecessor does, and the error names it.
func TestWaitOnUndefinedTaskSettlesNotStarted(t *testing.T) {
	out := isolatedOutput(t)
	x := out.Task("x")
	err := waitWithin(t, "x.Wait", x.Wait)
	if !errors.Is(err, ErrNotStarted) || !strings.Contains(err.Error(), "x was never defined") {
		t.Errorf("x.Wait = %v, want ErrNotStarted naming x", err)
	}
	if got := x.Snapshot().State; got != NotStarted {
		t.Errorf("x = %v after Wait, want NotStarted", got)
	}
}
