package evo_test

import (
	"io"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestTask_DeclaresPendingNotRunning is the red-first case for evo-rec.md's
// state-model gap: a freshly declared Task must not read as Running (which
// would draw N simultaneous spinners for N predeclared siblings) until it
// receives its first unit of evidence.
func TestTask_DeclaresPendingNotRunning(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("install")

	if got := task.Snapshot().State; got != evo.Pending {
		t.Fatalf("state at declare = %v, want Pending", got)
	}
}

// TestTask_PromotesToRunningOnFirstEvidence pins the promotion side: Phase,
// Progress, and Bytes are all first-evidence calls that move a Pending task
// to Running. Advance shares applyProgressLocked with Progress/Bytes, so the
// same promotion applies there too, but Advance needs a total already
// established (it is a relative helper) so it is not exercised standalone
// here.
func TestTask_PromotesToRunningOnFirstEvidence(t *testing.T) {
	cases := map[string]func(*evo.TaskHandle){
		"Phase":    func(h *evo.TaskHandle) { h.Doing("working") },
		"Progress": func(h *evo.TaskHandle) { h.Progress(1, 2) },
		"Bytes":    func(h *evo.TaskHandle) { h.Bytes(1, 2) },
	}
	for name, evidence := range cases {
		t.Run(name, func(t *testing.T) {
			out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
			t.Cleanup(func() { _ = out.Close() })
			task := out.Task("install")
			evidence(task)
			if got := task.Snapshot().State; got != evo.Running {
				t.Fatalf("state after %s = %v, want Running", name, got)
			}
		})
	}
}

// TestSequence_TwoRunningChildrenRecordsMisuse is the red-first case for the
// "one Running child" heart contract on a Sequence: promoting a
// second sibling to Running while the first is still Running is misuse.
func TestSequence_TwoRunningChildrenRecordsMisuse(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })

	setup := out.Sequence("python")
	scan := setup.Task("scan")
	venv := setup.Task("venv")

	scan.Doing("scanning")      // promotes scan to Running
	venv.Doing("creating venv") // second sibling Running while scan still is

	if err := out.Err(); err == nil {
		t.Fatal("want misuse recorded for two Running siblings in a Sequence")
	}
}

// TestDisplayGroup_ConcurrentIndependentChildrenAreNotMisuse pins the other side of
// the same contract: a plain DisplayGroup collection documents its children as
// independent (worker-pool fan-out), so two Running siblings there is a
// supported pattern, not misuse.
func TestDisplayGroup_ConcurrentIndependentChildrenAreNotMisuse(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })

	jobs := out.DisplayGroup("dependencies")
	a := jobs.Task("discover")
	b := jobs.Task("verify")

	a.Doing("discovering")
	b.Doing("waiting") // second Running sibling — allowed on a plain DisplayGroup collection

	if err := out.Err(); err != nil {
		t.Fatalf("want no misuse on an independent Tasks collection, got %v", err)
	}
}

// TestConclusion_LoneIncompleteTaskIsNotPartialHeadline is the red-first
// case for evo-rec.md's "Partial is a modifier, not a root verdict": an
// unresolved task at Finish must not invent a new headline state of its own
// — Partial stays evidence (Conclusion.Partial) layered over one of the four
// Outcome states (evo.StateReady/.../StateCancelled), never a fifth state.
// There is no evo.StatePartial to compare against: the round-4 release gate
// killed that dead enum member (it was never assigned by inferConclusion)
// rather than keep two competing models of the same fact.
func TestConclusion_LoneIncompleteTaskIsNotPartialHeadline(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
	out.Task("install") // declared, never resolved

	_ = out.Finish()
	conc := out.Conclusion()

	switch conc.State {
	case evo.StateReady, evo.StateChanged, evo.StateWarning,
		evo.StateBlocked, evo.StateFailed, evo.StateCancelled, evo.StatePlanned:
	default:
		t.Fatalf("headline = %v, want one of the documented Outcome states", conc.State)
	}
	if !conc.Partial {
		t.Fatal("want Partial=true retained as evidence")
	}
}
