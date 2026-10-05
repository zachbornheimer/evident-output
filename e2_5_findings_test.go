package evo_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// This file golden-proves Stage E2.5: the 7 fresh-context E1 review findings
// plus addendum item 8 (3-level nested container rendering). Each test names
// the finding it covers; the work order's final report captures these
// running RED against pre-E2.5 code and GREEN after.

// --- Finding 1: HIGH — group-child warnings never reach the conclusion ----

// TestE2_5Finding1_WarnedGroupChildReachesConclusion proves a warned-but-Done
// child nested inside a container still surfaces the "· warned" modifier on
// the run's own conclusion band, and the container's own success summary is
// suppressed rather than papering over the warning underneath it.
func TestE2_5Finding1_WarnedGroupChildReachesConclusion(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})
	t.Cleanup(func() { _ = out.Close() })

	group := out.Group("dependencies")
	child := group.Task("cache")
	child.Problem("stale entry ignored", evo.Severity(evo.SeverityWarning))
	succeed(child)

	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	conc := out.Conclusion()
	if conc.State != evo.StateReady {
		t.Fatalf("state = %v, want StateReady", conc.State)
	}
	if !conc.Warned {
		t.Fatal("Conclusion.Warned = false, want true (group-child warning must reach the conclusion)")
	}
	if conc.ExitCode != evo.ExitOK {
		t.Fatalf("exit code = %d, want %d", conc.ExitCode, evo.ExitOK)
	}
	if !strings.Contains(buf.String(), "[ready · warned]") {
		t.Fatalf("want the \"[ready · warned]\" conclusion band, got:\n%s", buf.String())
	}
}

// --- Finding 2: MED-HIGH — mutation verb returns nil without running call --

// TestE2_5Finding2_MutationOnResolvedTaskReturnsErrorNeverNil proves a
// mutation verb called after the task already resolved (Done) never
// executes the call and never silently swallows the misuse as a nil error.
func TestE2_5Finding2_MutationOnResolvedTaskReturnsErrorNeverNil(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Color: evo.ColorNever, Plain: true})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("branches")
	succeed(task)
	task.Define(effectOf(evo.EffectDelete, "stale local branch", 1))
	if !errors.Is(out.Err(), evo.ErrAlreadyResolved) {
		t.Fatalf("Err() = %v, want ErrAlreadyResolved for a mutation on an already-resolved task", out.Err())
	}
}

// --- Finding 3: MED — fixture inline-warning typography -------------------

// TestE2_5Finding3_InlineWarningRendersBangPrefix proves the inline warning
// on a ✓ row carries the same "! " signal a nested warning line does (the
// normative fixture's "! kept 13 (...)" typography) instead of dim text with
// no bang at all.
func TestE2_5Finding3_InlineWarningRendersBangPrefix(t *testing.T) {
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

// --- Finding 4: MED — quantity validation ---------------------------------
// Owned by EffectSpec validation now (TestEffect_RejectsContentFreeSpec):
// a Quantity <= 0 Effect is refused before any ledger row exists.

// --- Finding 5: LOW-MED — double-resolve race ------------------------------

// TestE2_5Finding5_ConcurrentSummaryDuringMutationCallDoesNotDropEffect proves
// the ledger target resolves once: a concurrent Summary racing an Effect's
// in-flight call must not cause the effect that call just committed
// to be silently dropped as spurious misuse.
func TestE2_5Finding5_ConcurrentSummaryDuringMutationCallDoesNotDropEffect(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Color: evo.ColorNever, Plain: true})
	t.Cleanup(func() { _ = out.Close() })

	started := make(chan struct{})
	release := make(chan struct{})
	branches := out.Task("branches")
	branches.Define(func(ctx context.Context) error {
		return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectDelete, Object: "stale local branch", Quantity: 2}, func(context.Context) error {
			close(started)
			<-release
			return nil
		})
	})
	<-started
	branches.Summary("2 deleted")
	close(release)
	_ = out.Finish()
	snap := out.Snapshot()
	found := false
	for _, ch := range snap.Changes {
		if len(ch.Records) > 0 {
			found = true
		}
	}
	if !found {
		t.Fatal("want the effect committed despite the concurrent Done, got no Changes records")
	}
}

// --- Finding 6: LOW — inline threshold unit --------------------------------

// TestE2_5Finding6_InlineThresholdMeasuresDisplayWidthNotBytes proves the
// inline-warning length gate measures display cells, not raw bytes — a
// warning built from multi-byte runes that still fits on the row must not be
// forced onto a nested line just because its byte length is inflated.
func TestE2_5Finding6_InlineThresholdMeasuresDisplayWidthNotBytes(t *testing.T) {
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

// --- Addendum item 8: 3-level nested container rendering golden -----------

// TestE2_5Item8_ThreeLevelNestedContainerPlainByteShape proves the plain/
// durable projection's byte shape for a Sequence nested three deep
// (release > python > venv > install task) — E2 proved the state derivation
// recurses correctly; this golden proves the rendered indentation does too.
func TestE2_5Item8_ThreeLevelNestedContainerPlainByteShape(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})
	t.Cleanup(func() { _ = out.Close() })

	root := out.Sequence("release")
	python := root.Sequence("python")
	venv := python.Sequence("venv")
	succeed(venv.Task("install"))

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	want := "✓ release\n" +
		"   ✓ python\n" +
		"      ✓ venv\n" +
		"         ✓ install\n"
	if !strings.Contains(buf.String(), want) {
		t.Fatalf("want the 3-level nested container to indent 3 spaces per level, got:\n%s", buf.String())
	}
}

// TestE2_5Item8_ThreeLevelNestedContainerLiveByteShape proves the same
// nesting depth in the interactive live region: each container level still
// indents its children 3 spaces deeper while Running.
func TestE2_5Item8_ThreeLevelNestedContainerLiveByteShape(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Isolated: true, Terminal: screen, VisibilityDelay: evo.DelayForTest(0), Color: evo.ColorNever})
	t.Cleanup(func() { _ = out.Close() })

	root := out.Sequence("release")
	python := root.Sequence("python")
	venv := python.Sequence("venv")
	install := venv.Task("install")
	install.Doing("installing")

	frame := screen.LatestLiveText()
	lines := strings.Split(frame, "\n")
	indentOf := func(line string) int {
		return len(line) - len(strings.TrimLeft(line, " "))
	}
	var releaseIndent, pythonIndent, venvIndent, installIndent int
	found := 0
	for _, line := range lines {
		switch {
		case strings.Contains(line, "release"):
			releaseIndent = indentOf(line)
			found++
		case strings.Contains(line, "python"):
			pythonIndent = indentOf(line)
			found++
		case strings.Contains(line, "venv"):
			venvIndent = indentOf(line)
			found++
		case strings.Contains(line, "install"):
			installIndent = indentOf(line)
			found++
		}
	}
	if found != 4 {
		t.Fatalf("want all 4 nesting levels present in the live frame, got %d:\n%s", found, frame)
	}
	if releaseIndent >= pythonIndent || pythonIndent >= venvIndent || venvIndent >= installIndent {
		t.Fatalf("want strictly increasing indent per nesting level (release=%d python=%d venv=%d install=%d):\n%s",
			releaseIndent, pythonIndent, venvIndent, installIndent, frame)
	}

	succeed(install)
	_ = out.Finish()
}
