package gates_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestConclusion_WarnedModifierSurvivesOKHeadline is release-gate round 8
// finding 3: a run with one Done task and one warning-severity-Problem task
// must not read as silently clean — a [ready] band may not hide a warning
// that occurred
// during the run. Conclusion.Warned and the "· warned" band modifier make
// it visible without changing the exit code (precedent: "· partial").
func TestConclusion_WarnedModifierSurvivesOKHeadline(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Plain: true, Color: evo.ColorNever})
	t.Cleanup(func() { _ = out.Close() })

	succeed(out.Task("fetch"))
	out.Task("cache").Problem("stale entry ignored", evo.Severity(evo.SeverityWarning))

	if err := out.Finish(); err != nil {
		t.Fatalf("Finish() = %v, want nil", err)
	}

	conc := out.Conclusion()
	if conc.State != evo.StateReady {
		t.Fatalf("state = %v, want StateReady (warning must not override an OK headline)", conc.State)
	}
	if !conc.Warned {
		t.Fatal("Conclusion.Warned = false, want true: a resolved warning-severity-Problem task must be visible on the conclusion")
	}
	if conc.ExitCode != evo.ExitOK {
		t.Fatalf("exit code = %d, want %d — warned must never change the exit code", conc.ExitCode, evo.ExitOK)
	}

	rendered := buf.String()
	if !strings.Contains(rendered, "[ready · warned]") {
		t.Fatalf("want the band to carry the · warned modifier, got:\n%s", rendered)
	}
}

// TestConclusion_WarningSeverityOnlyRunStillCarriesWarnedModifier is
// TestConclusion_WarningOnlyHeadlineOmitsRedundantModifier's P2 replacement:
// a warning-severity Problem no longer resolves its task (13-problem doc
// P2), so a task that only ever calls a Severity(SeverityWarning) Problem
// auto-resolves Done at Finish — the same amnesty a recorded effect gets —
// and the run reads StateReady, not a StateWarning headline. The
// "· warned" modifier still carries the warning forward.
func TestConclusion_WarningSeverityOnlyRunStillCarriesWarnedModifier(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Plain: true, Color: evo.ColorNever})
	t.Cleanup(func() { _ = out.Close() })

	out.Task("cache").Problem("stale entry ignored", evo.Severity(evo.SeverityWarning))

	if err := out.Finish(); err != nil {
		t.Fatalf("Finish() = %v, want nil", err)
	}

	conc := out.Conclusion()
	if conc.State != evo.StateReady {
		t.Fatalf("state = %v, want StateReady (a warning-severity Problem auto-resolves Done, P2)", conc.State)
	}
	if !conc.Warned {
		t.Fatal("Conclusion.Warned = false, want true")
	}
	if !strings.Contains(buf.String(), "· warned") {
		t.Fatalf("want the band to carry the · warned modifier, got:\n%s", buf.String())
	}
}
