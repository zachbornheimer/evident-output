package evo_test

import (
	"bytes"
	"io"
	"strings"
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

// TestTask_LifecycleStatesAreDistinct golden-proves Done/Failed/Blocked/
// Cancelled/NotStarted each render their own distinct glyph and text —
// different causes render differently, never a collapsed generic failure.
func TestTask_LifecycleStatesAreDistinct(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})

	succeed(out.Task("done-task"))
	out.Task("failed-task").Fail("build broke")
	out.Task("blocked-task").Block("needs confirmation")

	seq := out.Sequence("cancel-sequence")
	first, second := seq.Task("first"), seq.Task("second")
	first.Cancel("interrupted")
	_ = second // never resolved; Finish's group lifecycle marks it NotStarted

	if err := out.Finish(); err != nil {
		t.Log(err)
	}

	got := buf.String()
	for _, want := range []string{
		"✓ done-task",
		"✗ failed-task  build broke",
		"⊘ blocked-task  needs confirmation",
		"■ first   interrupted",
		"- second  not started",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q distinctly rendered, got:\n%s", want, got)
		}
	}
}
