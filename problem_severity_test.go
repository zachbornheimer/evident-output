package evo_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func TestProblem_DefaultSeverityFailsTaskAndRun(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true, StateDir: t.TempDir()})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("check")
	task.Define(func(context.Context) error {
		task.Problem("x")
		return nil
	})
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if got := task.Snapshot().State; got != evo.Failed {
		t.Fatalf("task state = %s, want Failed", got)
	}
	c := out.Conclusion()
	if c.State != evo.StateFailed || c.ExitCode != evo.ExitFailed {
		t.Fatalf("conclusion = %s exit %d, want failed exit %d\n%s", c.State, c.ExitCode, evo.ExitFailed, buf.String())
	}
}

func TestProblem_WarningSeveritySettlesDoneAndWarns(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true, StateDir: t.TempDir()})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("check")
	task.Define(func(context.Context) error {
		task.Problem("x", evo.Severity(evo.SeverityWarning))
		return nil
	})
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	snap := task.Snapshot()
	if snap.State != evo.Done {
		t.Fatalf("task state = %s, want Done", snap.State)
	}
	if len(snap.Warnings) != 1 || snap.Warnings[0].Summary != "x" {
		t.Fatalf("warnings = %#v, want one warning x", snap.Warnings)
	}
	if len(snap.Problems) != 0 {
		t.Fatalf("problems = %#v, want none on a warning Problem", snap.Problems)
	}
	c := out.Conclusion()
	if !c.Warned || c.ExitCode != evo.ExitOK {
		t.Fatalf("conclusion warned=%v exit %d, want warned exit 0\n%s", c.Warned, c.ExitCode, buf.String())
	}
}

func TestProblem_InvalidSeverityIsRejected(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true, StateDir: t.TempDir()})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("check")
	task.Define(func(context.Context) error {
		task.Problem("x", evo.Severity("nope"))
		return nil
	})
	_ = out.Finish()
	snap := task.Snapshot()
	if len(snap.Problems) != 0 || len(snap.Warnings) != 0 {
		t.Fatalf("invalid severity recorded a problem: problems=%#v warnings=%#v", snap.Problems, snap.Warnings)
	}
	err := out.Err()
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("Err() = %v, want a context-bearing invalid severity error containing nope", err)
	}
}

// TestProblemWarning_SingleShortWarningInlinesOnDoneRow proves the documented
// compact form: one short warning renders directly on the task's own ✓ row.
func TestProblemWarning_SingleShortWarningInlinesOnDoneRow(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})

	branches := out.Task("branches")
	branches.Problem("kept 11 (7 protected, 4 unpushed)", evo.Severity(evo.SeverityWarning))
	succeed(branches)

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	// E2.5 finding 3: the inline warning carries the same "! " bang the
	// nested warning line uses — an inline and a nested warning must signal
	// identically, one row, one line.
	if !strings.Contains(got, "✓ branches  ! kept 11 (7 protected, 4 unpushed)\n") {
		t.Fatalf("want the warning inlined on the ✓ row with its \"! \" prefix, got:\n%s", got)
	}
	if strings.Count(got, "!") != 1 {
		t.Fatalf("a single short warning must inline exactly once, not also render a nested ! line, got:\n%s", got)
	}
}

// TestProblemWarning_MultipleWarningsNestUnderneath proves the second documented
// form: more than one warning moves off the row onto its own nested "!"
// lines below it.
func TestProblemWarning_MultipleWarningsNestUnderneath(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})

	branches := out.Task("branches")
	branches.Problem("kept 11 (7 protected, 4 unpushed)", evo.Severity(evo.SeverityWarning))
	branches.Problem("2 remotes unreachable", evo.Severity(evo.SeverityWarning))
	succeed(branches)

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "✓ branches\n") {
		t.Fatalf("want a bare ✓ row (warnings moved below it), got:\n%s", got)
	}
	if !strings.Contains(got, "! kept 11 (7 protected, 4 unpushed)") || !strings.Contains(got, "! 2 remotes unreachable") {
		t.Fatalf("want both warnings nested under the row, got:\n%s", got)
	}
}

// TestProblemWarning_DoesNotResolveTask proves Warn is non-terminal: the task
// stays Pending immediately after Warn, and a later Done still resolves it.
func TestProblemWarning_DoesNotResolveTask(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})

	task := out.Task("cache")
	task.Problem("stale entry ignored", evo.Severity(evo.SeverityWarning))
	if got := task.Snapshot().State; got == evo.Done || got == evo.Failed || got == evo.Blocked {
		t.Fatalf("state = %v, want non-terminal (Warn must not resolve the task)", got)
	}
	succeed(task)
	if got := task.Snapshot().State; got != evo.Done {
		t.Fatalf("state = %v, want Done", got)
	}
	_ = out.Finish()
}

// TestProblemWarning_UnresolvedTaskAutoResolvesDoneAtFinish proves a task that
// only ever calls Warn (no terminal verb) auto-resolves Done at Finish, the
// same amnesty a recorded effect or sealed progress already gets.
func TestProblemWarning_UnresolvedTaskAutoResolvesDoneAtFinish(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})

	out.Task("cache").Problem("stale entry ignored", evo.Severity(evo.SeverityWarning))
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish() = %v, want nil (Warn-only task should auto-resolve Done)", err)
	}
	conc := out.Conclusion()
	if conc.State != evo.StateReady {
		t.Fatalf("state = %v, want StateReady", conc.State)
	}
	if !conc.Warned {
		t.Fatal("Conclusion.Warned = false, want true")
	}
}

// TestProblemWarning_InlineRendersBangPrefix proves the inline warning
// on a ✓ row carries the same "! " signal a nested warning line does (the
// normative fixture's "! kept 13 (...)" typography) instead of dim text with
// no bang at all.
func TestProblemWarning_InlineRendersBangPrefix(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})
	t.Cleanup(func() { _ = out.Close() })

	branches := out.Task("branches")
	branches.Problem("kept 11 (7 protected, 4 unpushed)", evo.Severity(evo.SeverityWarning))
	succeed(branches)

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "✓ branches  ! kept 11 (7 protected, 4 unpushed)") {
		t.Fatalf("want the inline warning to carry the \"! \" bang prefix, got:\n%s", got)
	}
}

// TestProblemWarning_InlineThresholdMeasuresDisplayWidthNotBytes proves the
// inline-warning length gate measures display cells, not raw bytes — a
// warning built from multi-byte runes that still fits on the row must not be
// forced onto a nested line just because its byte length is inflated.
func TestProblemWarning_InlineThresholdMeasuresDisplayWidthNotBytes(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})
	t.Cleanup(func() { _ = out.Close() })

	// Each "é" is 2 bytes but 1 display cell — 30 of them is 60 bytes but
	// only 30 cells, comfortably under the 40-cell inline threshold.
	warning := strings.Repeat("é", 30)
	branches := out.Task("branches")
	branches.Problem(warning, evo.Severity(evo.SeverityWarning))
	succeed(branches)

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "✓ branches  ! "+warning) {
		t.Fatalf("want the warning inlined (display-width under threshold), got:\n%s", got)
	}
}
