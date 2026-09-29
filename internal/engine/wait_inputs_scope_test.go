package engine

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// A Wait seals the same inputs whatever other Waits ran before it. Here
// m2 is declared into g after b was wired After the populated g, so it is
// not one of b's inputs (E-093): cold or after an earlier Wait walked b,
// c.Wait answers once m1 finishes and leaves m2 alone.
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
		m2 := g.Task("m2") // never Defined
		c := out.Task("c").After(b).Define(noop)
		time.AfterFunc(20*time.Millisecond, releaseM1)
		if err := waitWithin(t, "c.Wait", c.Wait); err != nil {
			t.Errorf("warm=%v: c.Wait = %v, want nil: m2 was declared after b's After(g)", warm, err)
		}
		if st := m2.Snapshot().State; st != Pending {
			t.Errorf("warm=%v: m2 = %s after c.Wait, want Pending: c does not wait for it", warm, st)
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
