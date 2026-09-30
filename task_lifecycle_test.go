package evo_test

import (
	"io"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestTask_DeclaresRunningSoTheRowCanSpin is FP-005 under the rec dialect:
// declare is Pending until submitted work starts. Doing is first evidence
// that promotes to Running so the row can spin. Sequence children stay
// Pending so later siblings do not all spin at once.
func TestTask_DeclaresRunningSoTheRowCanSpin(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("install")
	if got := task.Snapshot().State; got != evo.Pending {
		t.Fatalf("standalone state at declare = %v, want Pending", got)
	}
	task.Doing("working")
	if got := task.Snapshot().State; got != evo.Running {
		t.Fatalf("state after submitted evidence = %v, want Running", got)
	}

	seq := out.Sequence("steps")
	later := seq.Task("two")
	if got := later.Snapshot().State; got != evo.Pending {
		t.Fatalf("sequence child state at declare = %v, want Pending", got)
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
			out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
			t.Cleanup(func() { _ = out.Close() })
			task := out.Task("install")
			evidence(task)
			if got := task.Snapshot().State; got != evo.Running {
				t.Fatalf("state after %s = %v, want Running", name, got)
			}
		})
	}
}
