package gates_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestConclusion_WarnedModifierSurvivesOKHeadline is release-gate round 8
// finding 3: a run with one Done task and one Warn task must not read as
// silently clean — a [ready] band may not hide a warning that occurred
// during the run. Conclusion.Warned and the "· warned" band modifier make
// it visible without changing the exit code (precedent: "· partial").
func TestConclusion_WarnedModifierSurvivesOKHeadline(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}})
	t.Cleanup(func() { _ = out.Close() })

	out.Task("fetch").Done()
	out.Task("cache").Warn("stale entry ignored")

	if err := out.Finish(); err != nil {
		t.Fatalf("Finish() = %v, want nil", err)
	}

	conc := out.Conclusion()
	if conc.State != evo.StateReady {
		t.Fatalf("state = %v, want StateReady (warning must not override an OK headline)", conc.State)
	}
	if !conc.Warned {
		t.Fatal("Conclusion.Warned = false, want true: a resolved Warn task must be visible on the conclusion")
	}
	if conc.ExitCode != evo.ExitOK {
		t.Fatalf("exit code = %d, want %d — warned must never change the exit code", conc.ExitCode, evo.ExitOK)
	}

	rendered := buf.String()
	if !strings.Contains(rendered, "[ready · warned]") {
		t.Fatalf("want the band to carry the · warned modifier, got:\n%s", rendered)
	}
}

// TestConclusion_WarningOnlyHeadlineOmitsRedundantModifier is the green
// counterpart: when Warning is itself the headline (nothing else in the run
// resolved), the band never doubles up as "[warning · warned]".
func TestConclusion_WarningOnlyHeadlineOmitsRedundantModifier(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}})
	t.Cleanup(func() { _ = out.Close() })

	out.Task("cache").Warn("stale entry ignored")

	if err := out.Finish(); err != nil {
		t.Fatalf("Finish() = %v, want nil", err)
	}

	conc := out.Conclusion()
	if conc.State != evo.StateWarning {
		t.Fatalf("state = %v, want StateWarning", conc.State)
	}
	if conc.Warned {
		t.Fatal("Conclusion.Warned = true, want false when Warning is already the headline")
	}
	if strings.Contains(buf.String(), "· warned") {
		t.Fatalf("band must not double up when Warning is already the headline, got:\n%s", buf.String())
	}
}
