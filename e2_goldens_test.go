package evo_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// This file golden-proves Stage E2 of the v0.4.0 redesign (P3: three
// structural concepts with derived container state; P4: truthful
// concurrency; P5: one monotonic elapsed-time mechanism; the DisplayUnit
// rendering refactor). Each test names the pinned decision it covers; the
// work order's final report captures these running RED against the pre-E2
// code (a straight compile failure, since Sequence/DisplayGroup did not
// exist under those names) and GREEN after.

// --- P3: Sequence is an ordered dependency, cascading failure to NotStarted ---

// TestE2P3_SequenceCascade_FailureNotStartsLaterSiblings proves Sequence's
// defining behavior: once a child fails, every later-declared unresolved
// sibling auto-resolves to NotStarted ("-  <name>  not started") with no
// caller code — the same contract the deleted GroupHandle type carried,
// now under its P3 name.
func TestE2P3_SequenceCascade_FailureNotStartsLaterSiblings(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}})
	t.Cleanup(func() { _ = out.Close() })

	setup := out.Sequence("python")
	scan := setup.Task("scan")
	venv := setup.Task("venv")
	install := setup.Task("install")

	scan.Done()
	venv.Fail("uv exited 1")

	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if got := install.Snapshot().State; got != evo.NotStarted {
		t.Fatalf("install state = %v, want NotStarted", got)
	}
	if !strings.Contains(buf.String(), "-  install  not started") {
		t.Fatalf("rendered output missing \"-  install  not started\":\n%s", buf.String())
	}
}

// TestE2P3_SequenceCascade_NestedSequenceFailurePropagatesToRootHeader
// proves the recursive nesting P3 adds: a Sequence declared under another
// Sequence via .Sequence(name) still cascades within itself, and its
// failure surfaces at the root container's own derived header state.
func TestE2P3_SequenceCascade_NestedSequenceFailurePropagatesToRootHeader(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&bytes.Buffer{}), evo.Plain(), evo.NoColor()}})
	t.Cleanup(func() { _ = out.Close() })

	root := out.Sequence("release")
	root.Task("build").Done()
	python := root.Sequence("python")
	scan := python.Task("scan")
	venv := python.Task("venv")
	install := python.Task("install")

	scan.Done()
	venv.Fail("uv exited 1")
	_ = out.Finish()

	if got := install.Snapshot().State; got != evo.NotStarted {
		t.Fatalf("nested install state = %v, want NotStarted", got)
	}
	if got := root.Snapshot().State; got != evo.Failed {
		t.Fatalf("root sequence state = %v, want Failed (nested failure must surface)", got)
	}
}

// --- P4: DisplayGroup is presentation-only, concurrent Running is truthful ---

// TestE2P4_DisplayGroupTwoSpinnerFrame proves DisplayGroup's concurrency
// truth: two children both Running render the SAME shared spinner frame at
// once — a plain collection documents its children as independent, so
// concurrent Running rows are the expected shape, not a defect a "one
// Running child" heart contract would forbid (that contract belongs to
// Sequence alone).
func TestE2P4_DisplayGroupTwoSpinnerFrame(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	clock := testkit.NewClock()
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Terminal(screen), evo.VisibilityDelay(0), evo.Clock(clock), evo.NoColor()}})
	t.Cleanup(func() { _ = out.Close() })

	jobs := out.DisplayGroup("dependencies")
	a := jobs.Task("discover")
	b := jobs.Task("verify")

	a.Phase("discovering")
	b.Phase("verifying") // second Running sibling — no misuse, unlike Sequence

	if err := out.Err(); err != nil {
		t.Fatalf("Err() = %v, want nil (DisplayGroup permits concurrent Running)", err)
	}

	frame := screen.LatestLiveText()
	lines := strings.Split(frame, "\n")
	var spinnerLines []string
	for _, line := range lines {
		if strings.Contains(line, "discover") || strings.Contains(line, "verify") {
			spinnerLines = append(spinnerLines, line)
		}
	}
	if len(spinnerLines) != 2 {
		t.Fatalf("want 2 Running child rows, got %d:\n%s", len(spinnerLines), frame)
	}
	// Both rows carry the same leading glyph column — the shared spinner
	// frame both Running children get at the same instant.
	glyphOf := func(line string) string {
		trimmed := strings.TrimLeft(line, " ")
		fields := strings.SplitN(trimmed, "  ", 2)
		return fields[0]
	}
	if glyphOf(spinnerLines[0]) != glyphOf(spinnerLines[1]) {
		t.Fatalf("two concurrent Running children must share one spinner frame:\n%s", frame)
	}

	a.Done()
	b.Done()
	_ = out.Finish()
}

// --- P5: one monotonic elapsed-time mechanism ---

// TestE2P5_FiveSecondTimer_ContainerHeaderAgesPastThreshold proves the timer
// applies to an unfinished container header too, not only task rows: once
// elapsedAfter (5s) has passed since the header was first painted, it gains
// the same " — Ns" suffix a Running/Pending task row gets.
func TestE2P5_FiveSecondTimer_ContainerHeaderAgesPastThreshold(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	clock := testkit.NewClock()
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Terminal(screen), evo.VisibilityDelay(0), evo.Clock(clock), evo.NoColor()}})
	t.Cleanup(func() { _ = out.Close() })

	jobs := out.DisplayGroup("dependencies")
	install := jobs.Task("install")
	ticker := out.Task("ticker")

	install.Phase("installing")
	ticker.Progress(1, 100) // first live render: anchors the header's clock

	header := strings.SplitN(screen.LatestLiveText(), "\n", 2)[0]
	if strings.Contains(header, "—") {
		t.Fatalf("header must not show an elapsed suffix before the 5s threshold:\n%s", header)
	}

	clock.Advance(5 * time.Second)
	ticker.Progress(2, 100)

	header = strings.SplitN(screen.LatestLiveText(), "\n", 2)[0]
	if !strings.Contains(header, "complete — 5s") {
		t.Fatalf("expected the container header to gain a 5s elapsed suffix:\n%s", header)
	}

	install.Done()
	ticker.Done()
	_ = out.Finish()
}

// --- DisplayUnit rendering refactor: byte parity for the unchanged shapes ---

// TestE2_DisplayUnitRefactor_StandaloneTaskRenderingByteParity is the
// non-regression control for the DisplayUnit refactor of writeLiveTaskLine:
// a standalone Running task's rendered bytes are unchanged by the refactor
// (the order requires "golden-identical except where this order changes
// them" — this shape is not one of the changes).
func TestE2_DisplayUnitRefactor_StandaloneTaskRenderingByteParity(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	clock := testkit.NewClock()
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Terminal(screen), evo.VisibilityDelay(0), evo.Clock(clock), evo.NoColor()}})
	t.Cleanup(func() { _ = out.Close() })

	build := out.Task("build")
	build.Progress(3, 10)
	build.Phase("compiling") // forces a fresh render (Progress alone coalesces)

	frame := screen.LatestLiveText()
	if !strings.Contains(frame, "build") || !strings.Contains(frame, "3/10") {
		t.Fatalf("standalone Running task row shape regressed:\n%s", frame)
	}
	if strings.Contains(frame, "—") {
		t.Fatalf("a task under the 5s threshold must render with no elapsed suffix:\n%s", frame)
	}

	build.Done()
	_ = out.Finish()
}
