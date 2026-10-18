package evo

import (
	"context"
	"sync/atomic"
	"testing"
)

// A step that fails itself with a remedy and returns nil stops every later
// step, Defined or not, before its Wait returns: the canonical
// "fail with a remedy" form must cascade like a returned error does.
func TestSequenceStepFailedWithRemedyStopsLaterSteps(t *testing.T) {
	out := isolatedOutput(t)
	seq := out.Sequence("steps")
	a := seq.Task("a")
	b := seq.Task("b")
	c := seq.Task("c")
	var bRan atomic.Bool
	b.Define(func(context.Context) error { bRan.Store(true); return nil })
	a.Define(func(context.Context) error {
		a.fail("x", nextCommand("fix", "--now"))
		return nil
	})
	_ = waitWithin(t, "a.Wait", a.wait)
	for _, h := range []*Task{b, c} {
		if got := h.snapshot().State; got != notStarted {
			t.Errorf("%s = %v, want NotStarted", h.snapshot().Name, got)
		}
	}
	if bRan.Load() {
		t.Error("b's callback ran after a failed")
	}
}

// A step that blocks or cancels itself stops later steps exactly as a
// failed one does (spec §Sequence: a predecessor that cannot succeed makes
// later dependents NotStarted), Defined or not.
func TestSequenceStepBlockedOrCancelledStopsLaterSteps(t *testing.T) {
	outcomes := map[string]func(*Task){
		"blocked":   func(a *Task) { a.block("needs review") },
		"cancelled": func(a *Task) { a.cancel("superseded") },
	}
	for name, resolve := range outcomes {
		t.Run(name, func(t *testing.T) {
			out := isolatedOutput(t)
			seq := out.Sequence("steps")
			a := seq.Task("a")
			b := seq.Task("b")
			c := seq.Task("c")
			b.Define(func(context.Context) error { return nil })
			a.Define(func(context.Context) error { resolve(a); return nil })
			_ = waitWithin(t, "a.Wait", a.wait)
			for _, h := range []*Task{b, c} {
				if got := h.snapshot().State; got != notStarted {
					t.Errorf("%s = %v, want NotStarted", h.snapshot().Name, got)
				}
			}
		})
	}
}
