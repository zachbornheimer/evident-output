package engine

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
		a.Fail("x", NextCommand("fix", "--now"))
		return nil
	})
	_ = waitWithin(t, "a.Wait", a.Wait)
	for _, h := range []*TaskHandle{b, c} {
		if got := h.Snapshot().State; got != NotStarted {
			t.Errorf("%s = %v, want NotStarted", h.Snapshot().Name, got)
		}
	}
	if bRan.Load() {
		t.Error("b's callback ran after a failed")
	}
}
