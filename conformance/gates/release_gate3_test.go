package gates_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestFinish_PhaseOnlyTaskCleanReturn_NeverCancels is the red-first case for
// release-gate round 3 finding 1: a task that only reached Phase (never a
// terminal verb, never any recorded effect/progress/taxonomy) and whose run
// otherwise finished cleanly — no signal, no application error anywhere in
// the run — must never invent Cancelled/130. Nothing ever signaled the
// process; the honest verdict is an incomplete/not-started task under an
// OK-family, Partial-marked conclusion.
func TestFinish_PhaseOnlyTaskCleanReturn_NeverCancels(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})

	out.Task("connect").Doing("connecting")
	// early return nil — no Done/Fail/Block/Skip, no signal, no output-level error.

	if err := out.Finish(); err != nil {
		t.Fatalf("Finish() = %v, want nil (a clean finish must not report the abandoned task as misuse)", err)
	}
	conc := out.Conclusion()
	if conc.State == evo.StateCancelled {
		t.Fatalf("state = %v, want anything but StateCancelled on an uninterrupted run", conc.State)
	}
	if conc.ExitCode != evo.ExitOK {
		t.Fatalf("exit code = %d, want %d (ExitOK) — an uninterrupted abandoned task must never read as cancelled", conc.ExitCode, evo.ExitOK)
	}
	if !conc.Partial {
		t.Fatal("want Partial=true — the run really did leave a task unresolved")
	}
	rendered := buf.String()
	if strings.Contains(rendered, "cancelled") {
		t.Fatalf("rendered output must not claim cancellation on a clean finish:\n%s", rendered)
	}
}

// TestFinish_AbnormalFinish_UnresolvedRunningTaskStillCancels proves the
// paired half of finding 1: when the run really was interrupted (here, an
// application error recorded via Output.Failf before Finish), a leftover
// Running task still reads as Cancelled — the existing signal/error
// behavior must not regress.
func TestFinish_AbnormalFinish_UnresolvedRunningTaskStillCancels(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})

	leftover := out.Task("connect")
	leftover.Doing("connecting")
	out.Failf("stopped: %v", "disk full")

	_ = out.Finish()
	if got := leftover.Snapshot().State; got != evo.Cancelled {
		t.Fatalf("leftover task state = %v, want Cancelled (abnormal finish)", got)
	}
}

// TestRun_NeverResolvedBareTask_BandAndExitCodeAgree is the red-first case
// for release-gate round 3 finding 2: the band that Finish renders and the
// exit code Output.Run ultimately returns must be the same fact — never a
// printed OK-family band with an exit code that silently escalated after the
// fact. Release-gate round 4 finding 3 folded this exact scenario (a bare,
// never-touched task, clean finish) into the same amnesty as an abandoned
// Phase-then-forgotten task: both read OK-family + partial, never misuse, so
// this now also doubles as one half of that unification (see
// release_gate4_test.go's TestFinish_ForgottenTerminalVerb_... for the
// explicit with/without-Phase comparison).
func TestRun_NeverResolvedBareTask_BandAndExitCodeAgree(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})

	code := out.Run(func(o *evo.Output) error {
		o.Task("install") // declared, never started, never resolved
		return nil
	})

	rendered := buf.String()
	bandIsOKFamily := strings.Contains(rendered, "[ready") ||
		strings.Contains(rendered, "[changed") ||
		strings.Contains(rendered, "[unchanged") ||
		strings.Contains(rendered, "[planned")
	if bandIsOKFamily && code != evo.ExitOK {
		t.Fatalf("band read OK-family but exit code = %d (want %d to match); out:\n%s", code, evo.ExitOK, rendered)
	}
	if !bandIsOKFamily && code == evo.ExitOK {
		t.Fatalf("exit code read OK but the printed band did not; out:\n%s", rendered)
	}
	if code != evo.ExitOK {
		t.Fatalf("exit code = %d, want %d (ExitOK) — a bare unresolved task on a clean finish is Partial, not misuse", code, evo.ExitOK)
	}
}

// TestFinish_UnresolvedTask_HintReplacesRawMisuseLine is the red-first case
// for release-gate round 3 finding 3: an unresolved task's misuse must
// render as a named, actionable hint (Confirm's own "→" style), never the
// raw "misuse: <name>: evo: ..." sentinel text.
func TestFinish_UnresolvedTask_HintReplacesRawMisuseLine(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})

	out.Task("install") // declared, never resolved

	_ = out.Finish()
	rendered := buf.String()
	if strings.Contains(rendered, "evo: task has no final state") {
		t.Fatalf("raw sentinel jargon leaked into the user stream:\n%s", rendered)
	}
	if !strings.Contains(rendered, "call Done, Fail, Block, Skip, or a mutation verb on this task") {
		t.Fatalf("want the corrective hint rendered instead, got:\n%s", rendered)
	}
}

// TestChanges_RepeatedIdenticalRecordsMergeQuantities is the red-first case
// for release-gate round 3 finding 6: a naive loop that records the same
// (verb, object) pair repeatedly must merge into one summed row, not one
// row per call plus an overflow ellipsis.
func TestChanges_RepeatedIdenticalRecordsMergeQuantities(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain(), evo.Width(80)}})

	task := out.Task("cleanup")
	for i := 0; i < 12; i++ {
		_ = task.Delete("merged branch", nil, evo.Affected(1))
	}
	task.Done()

	if err := out.Finish(); err != nil {
		t.Fatalf("Finish() = %v, want nil", err)
	}
	rendered := buf.String()
	if strings.Contains(rendered, "more (not shown)") {
		t.Fatalf("identical records must merge instead of overflowing:\n%s", rendered)
	}
	if !strings.Contains(rendered, "deleted") || !strings.Contains(rendered, "12") || !strings.Contains(rendered, "merged branches") {
		t.Fatalf("want one merged row \"deleted 12 merged branches\", got:\n%s", rendered)
	}
}
