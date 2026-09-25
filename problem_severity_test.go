package evo_test

import (
	"bytes"
	"context"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// severityRun is one Task whose Define records the given Problems and
// returns nil, plus one run-scoped Problem, rendered plain.
func severityRun(t *testing.T, record func(task *evo.TaskHandle), run func(out *evo.Output)) (string, evo.Conclusion) {
	t.Helper()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, StateDir: t.TempDir(), Stdout: &buf, Title: "sev", Color: evo.ColorNever, Plain: true})
	task := out.Task("check tools")
	task.Define(func(context.Context) error {
		record(task)
		return nil
	})
	if run != nil {
		run(out)
	}
	_ = out.Finish()
	c := out.Conclusion()
	_ = out.Close()
	return buf.String(), c
}

// TestProblem_WarningSeverityRendersAsWarning pins that a warning-severity
// Problem is the one warning spelling: it never fails Define, and it
// renders byte-for-byte the way the removed Warn did (glyph, subject,
// "· warned" band), on a Task and on the run.
func TestProblem_WarningSeverityRendersAsWarning(t *testing.T) {
	warning := evo.Severity(evo.SeverityWarning)
	human, c := severityRun(t, func(task *evo.TaskHandle) {
		task.Problem("tool version differs from manifest", warning, evo.On("go")).
			Problem("second", warning, evo.Detail("more"))
	}, func(out *evo.Output) {
		out.Problem("run level warning", warning)
	})
	const want = "! run level warning\n✓ check tools\n  ! go  tool version differs from manifest\n  ! second\n\n[ready · warned]  sev\n"
	if human != want {
		t.Errorf("human output:\n got %q\nwant %q", human, want)
	}
	if c.ExitCode != evo.ExitOK || c.State != evo.StateReady {
		t.Errorf("conclusion = %v exit %d, want ready exit 0", c.State, c.ExitCode)
	}
}

// TestProblem_DefaultSeverityIsError pins that a Problem with no Severity
// (or an explicit SeverityError) fails the owning Define even though the
// callback returned nil.
func TestProblem_DefaultSeverityIsError(t *testing.T) {
	for name, opts := range map[string][]evo.ProblemOption{
		"default":  nil,
		"explicit": {evo.Severity(evo.SeverityError)},
	} {
		_, c := severityRun(t, func(task *evo.TaskHandle) {
			task.Problem("invalid configuration", opts...)
		}, nil)
		if c.State != evo.StateFailed || c.ExitCode != evo.ExitFailed {
			t.Errorf("%s: conclusion = %v exit %d, want failed exit %d", name, c.State, c.ExitCode, evo.ExitFailed)
		}
	}
}

// TestOutput_ProblemErrorSeverityFailsRun pins the run-scoped error
// Problem: the same run-level failure Output.Fail records.
func TestOutput_ProblemErrorSeverityFailsRun(t *testing.T) {
	_, c := severityRun(t, func(*evo.TaskHandle) {}, func(out *evo.Output) {
		out.Problem("state directory unreadable")
	})
	if c.State != evo.StateFailed || c.ExitCode != evo.ExitFailed {
		t.Errorf("conclusion = %v exit %d, want failed exit %d", c.State, c.ExitCode, evo.ExitFailed)
	}
}

// TestFail_IgnoresWarningSeverity pins that Fail is an outcome: a
// Severity(SeverityWarning) option cannot soften it into a warning. It
// asserts both the conclusion AND the recorded Problem's own Severity, so
// deleting applyOutcomeProblemOptions (which is what actually ignores the
// option) turns this test red even though the conclusion alone would still
// read Failed.
func TestFail_IgnoresWarningSeverity(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, StateDir: t.TempDir(), Stdout: &buf, Title: "sev", Color: evo.ColorNever, Plain: true})
	out.Task("probe").Fail("unreachable", evo.Severity(evo.SeverityWarning))
	_ = out.Finish()
	c := out.Conclusion()
	snap := out.Snapshot()
	_ = out.Close()

	if c.State != evo.StateFailed {
		t.Errorf("conclusion = %v, want failed", c.State)
	}
	if len(snap.Tasks) != 1 || len(snap.Tasks[0].Problems) != 1 {
		t.Fatalf("want one Task with one recorded Problem, got %#v", snap.Tasks)
	}
	if got := snap.Tasks[0].Problems[0].Severity; got != evo.SeverityError {
		t.Errorf("recorded Problem Severity = %q, want SeverityError — Fail must ignore Severity(SeverityWarning), not silently accept it", got)
	}
}
