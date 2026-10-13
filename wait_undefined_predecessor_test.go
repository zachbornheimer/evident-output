package evo

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"
)

// waitReturnDeadline bounds a Wait these tests expect to return promptly.
// A Wait that hangs is the defect, so the bound is generous, not tight.
const waitReturnDeadline = 5 * time.Second

// waitWithin runs wait on its own goroutine and fails the test when it
// has not returned by waitReturnDeadline.
func waitWithin(t *testing.T, what string, wait func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(waitReturnDeadline):
		t.Fatalf("%s: Wait did not return within %s", what, waitReturnDeadline)
		return nil
	}
}

func isolatedOutput(t *testing.T) *Run {
	t.Helper()
	out := initRun(Config{isolated: true, stateDir: t.TempDir(), stdout: io.Discard, stderr: io.Discard})
	t.Cleanup(func() { _ = out.close() })
	return out
}

func noop(context.Context) error { return nil }

// A Task After a predecessor nobody Defined can never start, so Wait on it
// answers ErrNotStarted instead of parking forever.
func TestWaitAfterUndefinedPredecessorReportsNotStarted(t *testing.T) {
	out := isolatedOutput(t)
	a := out.Task("a")
	b := out.Task("b").After(a).Define(noop)
	if err := waitWithin(t, "b.Wait", b.wait); !errors.Is(err, errNotStarted) {
		t.Fatalf("b.Wait = %v, want ErrNotStarted", err)
	}
}

// The Group counterpart: a member After a sibling nobody Defined.
func TestGroupWaitAfterUndefinedSiblingReportsNotStarted(t *testing.T) {
	out := isolatedOutput(t)
	g := out.Group("g")
	x := g.Task("x")
	g.Task("y").After(x).Define(noop)
	if err := waitWithin(t, "g.Wait", g.wait); !errors.Is(err, errNotStarted) {
		t.Fatalf("g.Wait = %v, want ErrNotStarted", err)
	}
}

// A Task After a Group whose member nobody Defined can never start either.
func TestWaitAfterGroupWithUndefinedMemberReportsNotStarted(t *testing.T) {
	out := isolatedOutput(t)
	g := out.Group("g")
	g.Task("member")
	b := out.Task("b").After(g).Define(noop)
	if err := waitWithin(t, "b.Wait", b.wait); !errors.Is(err, errNotStarted) {
		t.Fatalf("b.Wait = %v, want ErrNotStarted", err)
	}
}

// A Task waiting on itself is a deadlock whatever else the run declared: an
// unrelated row nobody Defined yet cannot resolve it.
func TestSelfWaitDeadlockReleasesDespiteUnrelatedUndefinedTask(t *testing.T) {
	out := isolatedOutput(t)
	out.Task("later row")
	a := out.Task("a")
	a.Define(func(context.Context) error { return a.wait() })
	if err := waitWithin(t, "a.Wait", a.wait); !errors.Is(err, errWaitDeadlock) {
		t.Fatalf("a.Wait = %v, want ErrWaitDeadlock", err)
	}
}

// An After cycle among Tasks nobody Defined must not trap Wait's walk.
func TestWaitOnUndefinedAfterCycleReturns(t *testing.T) {
	out := isolatedOutput(t)
	a, b := out.Task("a"), out.Task("b")
	a.After(b)
	b.After(a)
	if err := waitWithin(t, "a.Wait", a.wait); !errors.Is(err, errNotStarted) {
		t.Fatalf("a.Wait = %v, want ErrNotStarted", err)
	}
}

// A submitted Task whose predecessors reach an undefined cycle is the same
// trap one hop further out.
func TestWaitThroughUndefinedAfterCycleReturns(t *testing.T) {
	out := isolatedOutput(t)
	a, b := out.Task("a"), out.Task("b")
	a.After(b)
	b.After(a)
	c := out.Task("c").After(a).Define(noop)
	if err := waitWithin(t, "c.Wait", c.wait); !errors.Is(err, errNotStarted) {
		t.Fatalf("c.Wait = %v, want ErrNotStarted", err)
	}
}

// A diamond lattice of undefined Tasks has 2^levels paths; Wait's walk
// must visit each Task once, not once per path.
func TestWaitThroughUndefinedDiamondIsBounded(t *testing.T) {
	const levels = 30
	out := isolatedOutput(t)
	top := out.Task("top")
	prev := []*Task{top}
	for i := range levels {
		l, r := out.Task(fmt.Sprintf("l%d", i)), out.Task(fmt.Sprintf("r%d", i))
		l.After(prev[0], prev[len(prev)-1])
		r.After(prev[0], prev[len(prev)-1])
		prev = []*Task{l, r}
	}
	bottom := out.Task("bottom").After(prev[0], prev[1]).Define(noop)
	if err := waitWithin(t, "bottom.Wait", bottom.wait); !errors.Is(err, errNotStarted) {
		t.Fatalf("bottom.Wait = %v, want ErrNotStarted", err)
	}
}
