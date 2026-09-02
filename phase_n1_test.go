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
	out := evo.NewWithOptions(evo.To(io.Discard))
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
		"Phase":    func(h *evo.TaskHandle) { h.Phase("working") },
		"Progress": func(h *evo.TaskHandle) { h.Progress(1, 2) },
		"Bytes":    func(h *evo.TaskHandle) { h.Bytes(1, 2) },
	}
	for name, evidence := range cases {
		t.Run(name, func(t *testing.T) {
			out := evo.NewWithOptions(evo.To(io.Discard))
			t.Cleanup(func() { _ = out.Close() })
			task := out.Task("install")
			evidence(task)
			if got := task.Snapshot().State; got != evo.Running {
				t.Fatalf("state after %s = %v, want Running", name, got)
			}
		})
	}
}

// TestTask_AdvanceAfterSealedTotalPromotesToRunning pins Advance's promotion
// once a total is sealed (Advance is a relative helper over the same total,
// so a Pending task needs one Progress call first to establish it — Advance
// alone on an unknown total is invalid progress, not a promotion case).
func TestTask_AdvanceAfterSealedTotalPromotesToRunning(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("install")
	task.Advance(0) // Advance(0) on the not-yet-sealed Total=0 is valid progress
	if got := task.Snapshot().State; got != evo.Running {
		t.Fatalf("state after Advance = %v, want Running", got)
	}
}

// TestGroup_TwoRunningChildrenRecordsMisuse is the red-first case for the
// "one Running child" heart contract on a sequential Group: promoting a
// second sibling to Running while the first is still Running is misuse.
func TestGroup_TwoRunningChildrenRecordsMisuse(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })

	setup := out.Group("python")
	scan := setup.Task("scan")
	venv := setup.Task("venv")

	scan.Phase("scanning")      // promotes scan to Running
	venv.Phase("creating venv") // second sibling Running while scan still is

	if err := out.Err(); err == nil {
		t.Fatal("want misuse recorded for two Running siblings in a sequential Group")
	}
}

// TestTasks_ConcurrentIndependentChildrenAreNotMisuse pins the other side of
// the same contract: a plain Tasks collection documents its children as
// independent (worker-pool fan-out), so two Running siblings there is a
// supported pattern, not misuse.
func TestTasks_ConcurrentIndependentChildrenAreNotMisuse(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })

	jobs := out.Tasks("dependencies")
	a := jobs.Task("discover")
	b := jobs.Task("verify")

	a.Phase("discovering")
	b.Phase("waiting") // second Running sibling — allowed on a plain Tasks collection

	if err := out.Err(); err != nil {
		t.Fatalf("want no misuse on an independent Tasks collection, got %v", err)
	}
}

// TestConclusion_LoneIncompleteTaskIsNotPartialHeadline is the red-first
// case for evo-rec.md's "Partial is a modifier, not a root verdict": an
// unresolved task at Finish must not invent StatePartial as the headline —
// Partial stays evidence (Conclusion.Partial), the headline stays an
// Outcome, and Finish still reports the unresolved-task misuse.
func TestConclusion_LoneIncompleteTaskIsNotPartialHeadline(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	out.Task("install") // declared, never resolved

	_ = out.Finish()
	conc := out.Conclusion()

	if conc.State == evo.StatePartial {
		t.Fatalf("headline = %v, want an Outcome state, not StatePartial", conc.State)
	}
	if !conc.Partial {
		t.Fatal("want Partial=true retained as evidence")
	}
}
