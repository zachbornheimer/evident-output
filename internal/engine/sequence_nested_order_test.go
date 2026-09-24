package engine

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"
)

// stepLog records the order Sequence steps start and end in.
type stepLog struct {
	mu     sync.Mutex
	events []string
}

func (l *stepLog) step(name string, d time.Duration) func(context.Context) error {
	return func(context.Context) error {
		l.add(name + " start")
		time.Sleep(d)
		l.add(name + " end")
		return nil
	}
}

func (l *stepLog) add(e string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, e)
}

// A nested Group is one Sequence step: it starts after the step before it
// ends, and the step after it waits for all of it.
func TestSequenceOrdersNestedCollectionSteps(t *testing.T) {
	out := isolatedOutput(t)
	var log stepLog
	seq := out.Sequence("steps")
	seq.Task("a").Define(log.step("a", 100*time.Millisecond))
	g := seq.Group("g")
	g.Task("x").Define(log.step("x", 50*time.Millisecond))
	g.Task("y").Define(log.step("y", 50*time.Millisecond))
	seq.Task("b").Define(log.step("b", 0))
	if err := waitWithin(t, "seq.Wait", seq.Wait); err != nil {
		t.Fatalf("seq.Wait = %v", err)
	}
	ev := log.events
	idx := func(e string) int { return slices.Index(ev, e) }
	if idx("a end") > idx("x start") || idx("a end") > idx("y start") {
		t.Errorf("nested Group started before the step before it ended: %v", ev)
	}
	if idx("b start") < idx("x end") || idx("b start") < idx("y end") {
		t.Errorf("step after the nested Group started before the Group ended: %v", ev)
	}
}

// An empty nested collection still keeps the step after it behind the
// step before it.
func TestSequenceStepAfterEmptyNestedGroupWaitsForEarlierStep(t *testing.T) {
	out := isolatedOutput(t)
	var log stepLog
	seq := out.Sequence("steps")
	seq.Task("a").Define(log.step("a", 50*time.Millisecond))
	seq.Group("empty")
	b := seq.Task("b").Define(log.step("b", 0))
	if err := waitWithin(t, "b.Wait", b.Wait); err != nil {
		t.Fatalf("b.Wait = %v", err)
	}
	if ev := log.events; slices.Index(ev, "a end") > slices.Index(ev, "b start") {
		t.Errorf("b started before a ended: %v", ev)
	}
}

// A failed step leaves every later step not started, nested ones included,
// whether or not they were Defined.
func TestSequenceFailureStopsLaterNestedSteps(t *testing.T) {
	out := isolatedOutput(t)
	seq := out.Sequence("steps")
	boom := errors.New("boom")
	a := seq.Task("a").Define(func(context.Context) error { return boom })
	g := seq.Group("g")
	x := g.Task("x").Define(noop)
	y := g.Task("y")
	if err := waitWithin(t, "a.Wait", a.Wait); !errors.Is(err, boom) {
		t.Fatalf("a.Wait = %v, want boom", err)
	}
	for _, h := range []*TaskHandle{x, y} {
		if got := h.Snapshot().State; got != NotStarted {
			t.Errorf("%s = %v, want NotStarted", h.Snapshot().Name, got)
		}
	}
}

// A run of empty nested steps forwards the step before them to the step
// after them, however many there are.
func TestSequenceStepAfterEmptyNestedChainWaitsForEarlierStep(t *testing.T) {
	out := isolatedOutput(t)
	var log stepLog
	seq := out.Sequence("steps")
	seq.Task("a").Define(log.step("a", 50*time.Millisecond))
	seq.Group("empty 1")
	seq.Sequence("empty 2")
	seq.Group("empty 3")
	b := seq.Task("b").Define(log.step("b", 0))
	if err := waitWithin(t, "b.Wait", b.Wait); err != nil {
		t.Fatalf("b.Wait = %v", err)
	}
	if ev := log.events; slices.Index(ev, "a end") > slices.Index(ev, "b start") {
		t.Errorf("b started before a ended: %v", ev)
	}
}

// A failure before a run of empty nested steps still stops the step after
// them.
func TestSequenceFailureCrossesEmptyNestedSteps(t *testing.T) {
	out := isolatedOutput(t)
	seq := out.Sequence("steps")
	boom := errors.New("boom")
	seq.Task("a").Define(func(context.Context) error { return boom })
	seq.Group("empty 1")
	seq.Group("empty 2")
	b := seq.Task("b").Define(noop)
	if err := waitWithin(t, "b.Wait", b.Wait); err == nil {
		t.Fatal("b.Wait = nil, want an error: a failed before it")
	}
	if got := b.Snapshot().State; got != NotStarted {
		t.Errorf("b = %v, want NotStarted", got)
	}
}
