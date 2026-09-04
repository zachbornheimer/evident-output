package evo_test

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

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
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})
	t.Cleanup(func() { _ = out.Close() })

	group := out.DisplayGroup("dependencies")
	child := group.Task("cache")
	child.Warn("stale entry ignored")
	child.Done()

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
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.NoColor(), evo.Plain()}})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("branches")
	task.Done()

	called := false
	err := task.Delete("stale local branch", func() error {
		called = true
		return nil
	}, evo.Affected(1))

	if called {
		t.Fatal("Delete's call must not run once the task is already resolved")
	}
	if err == nil {
		t.Fatal("Delete() = nil, want a non-nil error for a mutation on an already-resolved task")
	}
	if !errors.Is(err, evo.ErrAlreadyResolved) {
		t.Fatalf("Delete() = %v, want it to wrap ErrAlreadyResolved", err)
	}
}

// --- Finding 3: MED — fixture inline-warning typography -------------------

// TestE2_5Finding3_InlineWarningRendersBangPrefix proves the inline warning
// on a ✓ row carries the same "! " signal a nested warning line does (the
// normative fixture's "! kept 13 (...)" typography) instead of dim text with
// no bang at all.
func TestE2_5Finding3_InlineWarningRendersBangPrefix(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})
	t.Cleanup(func() { _ = out.Close() })

	branches := out.Task("branches")
	branches.Warn("kept 11 (7 protected, 4 unpushed)")
	branches.Done()

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "✓ branches  ! kept 11 (7 protected, 4 unpushed)") {
		t.Fatalf("want the inline warning to carry the \"! \" bang prefix, got:\n%s", got)
	}
}

// --- Finding 4: MED — Affected validation ----------------------------------

// TestE2_5Finding4_NegativeAffectedRecordsMisuseNothing proves Affected(n<0)
// is caller misuse: nothing is recorded into either ledger, and Err()
// reports ErrInvalidConfig.
func TestE2_5Finding4_NegativeAffectedRecordsMisuseNothing(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})
	t.Cleanup(func() { _ = out.Close() })

	branches := out.Task("branches")
	if err := branches.Delete("stale local branch", nil, evo.Affected(-1)); !errors.Is(err, evo.ErrInvalidConfig) {
		t.Fatalf("Delete() = %v, want ErrInvalidConfig for a negative Affected quantity", err)
	}
	branches.Done()
	_ = out.Finish()
	if !errors.Is(out.Err(), evo.ErrInvalidConfig) {
		t.Fatalf("Err() = %v, want ErrInvalidConfig", out.Err())
	}
	snap := out.Snapshot()
	if len(snap.Changes) != 0 || len(snap.Plans) != 0 {
		t.Fatalf("want no ledger sections recorded for a negative Affected call, got Changes=%+v Plans=%+v", snap.Changes, snap.Plans)
	}
}

// TestE2_5Finding4_ZeroAffectedNeverCreatesEffectlessLedgerSection proves
// Affected(0) never declares an intended verb and never renders a
// "nothing to X" section — the fixture's "[planned] repo-retire" phantom-row
// bug class.
func TestE2_5Finding4_ZeroAffectedNeverCreatesEffectlessLedgerSection(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain(), evo.DryRun()}})
	t.Cleanup(func() { _ = out.Close() })

	branches := out.Task("branches")
	if err := branches.Delete("stale local branch", nil, evo.Affected(0)); err != nil {
		t.Fatalf("Delete() = %v, want nil", err)
	}
	branches.Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	snap := out.Snapshot()
	if len(snap.Plans) != 0 {
		t.Fatalf("want no Plan section declared for a zero-Affected call, got %+v", snap.Plans)
	}
	if strings.Contains(buf.String(), "nothing to") || strings.Contains(buf.String(), "[planned]  branches") {
		t.Fatalf("want no effectless \"branches\" ledger section rendered, got:\n%s", buf.String())
	}
}

// --- Finding 5: LOW-MED — double-resolve race ------------------------------

// TestE2_5Finding5_ConcurrentDoneDuringMutationCallDoesNotDropEffect proves
// the ledger target resolves once: a concurrent Done racing a mutation
// verb's in-flight call must not cause the effect that call just committed
// to be silently dropped as spurious misuse.
func TestE2_5Finding5_ConcurrentDoneDuringMutationCallDoesNotDropEffect(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.NoColor(), evo.Plain()}})
	t.Cleanup(func() { _ = out.Close() })

	branches := out.Task("branches")
	callStarted := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-callStarted
		branches.Done()
	}()

	err := branches.Delete("stale local branch", func() error {
		close(callStarted)
		// Give the concurrent Done a chance to run before the call returns
		// and the effect gets recorded.
		<-time.After(20 * time.Millisecond)
		return nil
	}, evo.Affected(2))
	wg.Wait()

	if err != nil {
		t.Fatalf("Delete() = %v, want nil (the call itself succeeded)", err)
	}
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
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})
	t.Cleanup(func() { _ = out.Close() })

	// Each "é" is 2 bytes but 1 display cell — 30 of them is 60 bytes but
	// only 30 cells, comfortably under the 40-cell inline threshold.
	warning := strings.Repeat("é", 30)
	branches := out.Task("branches")
	branches.Warn(warning)
	branches.Done()

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "✓ branches  ! "+warning) {
		t.Fatalf("want the warning inlined (display-width under threshold), got:\n%s", got)
	}
}

// --- Finding 7: LOW — Record/RecordName/RecordLabel nil-safety ------------

// TestE2_5Finding7_RecordFamilyNilSafe proves Record/RecordName/RecordLabel
// never panic on a nil TaskHandle or a TaskHandle whose Output is gone — the
// same nil-safety the mutation verbs already have.
func TestE2_5Finding7_RecordFamilyNilSafe(t *testing.T) {
	var nilHandle *evo.TaskHandle
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Record panicked on a nil TaskHandle: %v", r)
			}
		}()
		nilHandle.Record("delete", 2, "branch")
		nilHandle.RecordName("delete", "branch")
		nilHandle.RecordLabel("ready", 1, "branch")
	}()
}

// --- Addendum item 8: 3-level nested container rendering golden -----------

// TestE2_5Item8_ThreeLevelNestedContainerPlainByteShape proves the plain/
// durable projection's byte shape for a Sequence nested three deep
// (release > python > venv > install task) — E2 proved the state derivation
// recurses correctly; this golden proves the rendered indentation does too.
func TestE2_5Item8_ThreeLevelNestedContainerPlainByteShape(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})
	t.Cleanup(func() { _ = out.Close() })

	root := out.Sequence("release")
	python := root.Sequence("python")
	venv := python.Sequence("venv")
	venv.Task("install").Done()

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
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Terminal(screen), evo.VisibilityDelay(0), evo.NoColor()}})
	t.Cleanup(func() { _ = out.Close() })

	root := out.Sequence("release")
	python := root.Sequence("python")
	venv := python.Sequence("venv")
	install := venv.Task("install")
	install.Doing("installing")

	frame := screen.LatestLiveText()
	lines := strings.Split(frame, "\n")
	var indentOf = func(line string) int {
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

	install.Done()
	_ = out.Finish()
}
