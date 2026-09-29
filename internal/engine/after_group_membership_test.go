package engine

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// startLog records the order Tasks started in.
type startLog struct {
	mu    sync.Mutex
	order []string
}

func (l *startLog) work(name string, d time.Duration) func(context.Context) error {
	return func(context.Context) error {
		l.mu.Lock()
		l.order = append(l.order, name)
		l.mu.Unlock()
		time.Sleep(d)
		return nil
	}
}

func (l *startLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.order...)
}

// TestAfterGroupPopulatedInLoop_WaitsForEveryChild pins E-028's second
// half: a Group wired as a predecessor before its loop declared its
// children counted as succeeded whenever the children declared so far had
// finished, so fetch started after child 0 and never waited for 1-4.
func TestAfterGroupPopulatedInLoop_WaitsForEveryChild(t *testing.T) {
	out := newGraphTestOutput(t)
	var log startLog
	g := out.Group("items")
	out.Task("fetch").After(g).Define(log.work("fetch", 0))
	const n = 5
	for i := range n {
		g.Task(fmt.Sprint(i)).Define(log.work(fmt.Sprint(i), 10*time.Millisecond))
		time.Sleep(15 * time.Millisecond) // child i finishes before i+1 exists
	}
	if err := closeWithin(t, out); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	order := log.snapshot()
	if len(order) != n+1 || order[n] != "fetch" {
		t.Fatalf("start order = %v, want fetch after every child", order)
	}
}

// TestAfterGroupPopulatedFirst_StartsWithoutWaitingForDrain proves the
// ordinary shape stays eager: a Group already populated when named in
// After is taken as declared, so fetch starts once those children finish,
// not when the run drains.
func TestAfterGroupPopulatedFirst_StartsWithoutWaitingForDrain(t *testing.T) {
	out := newGraphTestOutput(t)
	g := out.Group("items")
	for i := range 3 {
		g.Task(fmt.Sprint(i)).Define(nopWork)
	}
	fetched := make(chan struct{})
	out.Task("fetch").After(g).Define(func(context.Context) error {
		close(fetched)
		return nil
	})
	select {
	case <-fetched:
	case <-time.After(waitOutcomeTimeout):
		t.Fatal("fetch never started before the drain")
	}
	_ = closeWithin(t, out)
}

// TestAfterGroup_GroupWaitReleasesDependents proves a Group Wait closes
// the Group's membership: once the caller asked for its outcome, a Task
// wired After it before it was populated may start.
func TestAfterGroup_GroupWaitReleasesDependents(t *testing.T) {
	out := newGraphTestOutput(t)
	g := out.Group("items")
	fetch := out.Task("fetch").After(g).Define(nopWork)
	g.Task("a").Define(nopWork)
	if err := g.Wait(); err != nil {
		t.Fatalf("Group Wait: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- fetch.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("fetch Wait = %v, want nil", err)
		}
	case <-time.After(waitOutcomeTimeout):
		t.Fatal("fetch never started after the Group Wait")
	}
	_ = closeWithin(t, out)
}

// TestSequenceStepAfterPopulatedGroup_StartsBeforeDrain proves the
// Sequence edge to a populated nested Group stays eager too.
func TestSequenceStepAfterPopulatedGroup_StartsBeforeDrain(t *testing.T) {
	out := newGraphTestOutput(t)
	s := out.Sequence("s")
	g := s.Group("items")
	g.Task("a").Define(nopWork)
	next := make(chan struct{})
	s.Task("next").Define(func(context.Context) error {
		close(next)
		return nil
	})
	select {
	case <-next:
	case <-time.After(waitOutcomeTimeout):
		t.Fatal("the step after a populated Group never started before the drain")
	}
	_ = closeWithin(t, out)
}
