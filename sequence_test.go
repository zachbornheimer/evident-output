package evo

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

// TestSequence_FailureAutoResolvesLaterSiblingsToNotStarted is the red-first
// case for evo-rec.md item #3: after a failure, the user can't tell whether
// later steps ran unless the group renders them "-  <name>  not started"
// with no caller code.
func TestSequence_FailureAutoResolvesLaterSiblingsToNotStarted(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, color: colorNever, plain: true})
	t.Cleanup(func() { _ = out.close() })

	setup := out.Sequence("python")
	scan := setup.Task("scan")
	venv := setup.Task("venv")
	install := setup.Task("install")

	succeed(scan)
	venv.fail("uv exited 1")

	if err := out.finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	if got := install.snapshot().State; got != notStarted {
		t.Fatalf("install state = %v, want NotStarted", got)
	}
	if got := install.snapshot().Summary; got != "not started" {
		t.Fatalf("install summary = %q, want %q", got, "not started")
	}
	if !strings.Contains(buf.String(), "- install  not started") {
		t.Fatalf("rendered output missing \"- install  not started\":\n%s", buf.String())
	}
}

// TestSequence_EarlierCompletedSiblingKeepsItsResolvedState covers "earlier
// Done rows are never erased" once a later sibling fails.
func TestSequence_EarlierCompletedSiblingKeepsItsResolvedState(t *testing.T) {
	out := initRun(Config{stdout: &bytes.Buffer{}, color: colorNever, plain: true})
	t.Cleanup(func() { _ = out.close() })

	setup := out.Sequence("python")
	scan := setup.Task("scan")
	venv := setup.Task("venv")

	succeed(scan)
	venv.fail("uv exited 1")
	_ = out.finish()

	if got := scan.snapshot().State; got != succeeded {
		t.Fatalf("scan state = %v, want Done (must not be rewritten)", got)
	}
}

// TestSequence_ExplicitResolutionWinsOverAutoResolution: a caller that resolved
// a later sibling itself before Finish keeps that resolution — the library
// never overwrites a caller's explicit disposition.
func TestSequence_ExplicitResolutionWinsOverAutoResolution(t *testing.T) {
	out := initRun(Config{stdout: &bytes.Buffer{}, color: colorNever, plain: true})
	t.Cleanup(func() { _ = out.close() })

	setup := out.Sequence("python")
	scan := setup.Task("scan")
	venv := setup.Task("venv")
	extras := setup.Task("extras")

	succeed(scan)
	venv.fail("uv exited 1")
	extras.skipForTest("optional, not needed")

	_ = out.finish()

	if got := extras.snapshot().State; got != excluded {
		t.Fatalf("extras state = %v, want Skipped (explicit resolution must win)", got)
	}
}

// TestSequence_CancelAutoResolvesLaterSiblings covers the SIGINT path
// (evo-rec.md "early termination"): the active task cancels and later
// pending siblings still render "- not started".
func TestSequence_CancelAutoResolvesLaterSiblings(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, color: colorNever, plain: true})
	t.Cleanup(func() { _ = out.close() })

	setup := out.Sequence("python")
	scan := setup.Task("scan")
	venv := setup.Task("venv")
	install := setup.Task("install")

	succeed(scan)
	venv.cancel("interrupted")

	_ = out.finish()

	if got := install.snapshot().State; got != notStarted {
		t.Fatalf("install state = %v, want NotStarted", got)
	}
	if !strings.Contains(buf.String(), "- install  not started") {
		t.Fatalf("rendered output missing \"- install  not started\":\n%s", buf.String())
	}
}

// TestSequence_ConclusionAndExitCodeComeFromFailedChildNotFromNotStarted proves
// NotStarted rows never count as failure in the Conclusion — the verdict and
// exit code come from the failed child alone.
func TestSequence_ConclusionAndExitCodeComeFromFailedChildNotFromNotStarted(t *testing.T) {
	out := initRun(Config{stdout: &bytes.Buffer{}, color: colorNever, plain: true})
	t.Cleanup(func() { _ = out.close() })

	setup := out.Sequence("python")
	succeed(setup.Task("scan"))
	setup.Task("venv").fail("uv exited 1")
	setup.Task("install")

	_ = out.finish()

	conc := out.conclusion()
	if conc.headline() != headlineFailed {
		t.Fatalf("conclusion state = %v, want StateFailed", conc.headline())
	}
	if conc.exitCode != exitFailed {
		t.Fatalf("exit code = %d, want %d (ExitFailed)", conc.exitCode, exitFailed)
	}
}

// TestSequence_AllChildrenDoneRendersAsToday is the non-regression control: a
// group with no failure/cancellation renders exactly as before this change.
func TestSequence_AllChildrenDoneRendersAsToday(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, color: colorNever, plain: true})
	t.Cleanup(func() { _ = out.close() })

	setup := out.Sequence("python")
	succeed(setup.Task("scan"))
	succeed(setup.Task("venv"))
	succeed(setup.Task("install"))

	if err := out.finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if strings.Contains(buf.String(), "not started") {
		t.Fatalf("unexpected \"not started\" row in an all-done group:\n%s", buf.String())
	}
	if got := setup.snapshot().State; got != succeeded {
		t.Fatalf("group state = %v, want Done", got)
	}
}

// TestSequence_SequentialBytesProgressFinishesClean is the red-first case for
// examples/live-progress's release-blocker: a Sequence child that reports
// Task.Bytes progress across several calls, resolved before its sibling
// starts (the "one Running child" sequential contract — evo-rec.md's
// "python" example, phase_n1_test.go's TestSequence_TwoRunningChildrenRecordsMisuse),
// must Finish cleanly with no misuse recorded. It failed before this fix
// because examples/live-progress declared download.Bytes(0, total) then
// started a sibling's Phase before download resolved, tripping
// ErrConcurrentRunning and exiting 2 despite printing "[succeeded]".
func TestSequence_SequentialBytesProgressFinishesClean(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, color: colorNever, plain: true})
	t.Cleanup(func() { _ = out.close() })

	jobs := out.Sequence("dependencies")
	download := jobs.Task("download")
	verify := jobs.Task("verify")

	const total int64 = 18_000_000
	download.bytes(0, total)
	download.bytes(total/2, total)
	download.bytes(total, total)
	succeed(download, "18.0 MB")

	verify.Doing("checking signatures")
	succeed(verify)

	if err := out.finish(); err != nil {
		t.Fatalf("Finish: %v\noutput:\n%s", err, buf.String())
	}
	if err := out.err(); err != nil {
		t.Fatalf("Err() = %v, want nil", err)
	}
}

// TestSequence_PackageLevelRepeatIsDuplicateSibling mirrors Task's
// identity contract (§3.1): Default().Sequence(name) called twice, and
// Sequence.Task(name) called twice within one sequence, are duplicate
// sibling declarations — each second call reports a distinct, orphaned
// handle instead of silently merging into the first.
func TestSequence_PackageLevelRepeatIsDuplicateSibling(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, color: colorNever, plain: true})
	setDefault(out)
	t.Cleanup(func() { _ = defaultRun().close() })

	g1 := defaultRun().Sequence("python")
	g2 := defaultRun().Sequence("python")
	if g1 == g2 {
		t.Fatal("Default().Sequence(name) called twice must return distinct handles")
	}
	if !errors.Is(out.err(), errDuplicateSiblingName) {
		t.Fatalf("Err() = %v, want ErrDuplicateSiblingName", out.err())
	}

	t1 := g1.Task("venv")
	t2 := g1.Task("venv")
	if t1 == t2 {
		t.Fatal("Sequence.Task(name) called twice must return distinct handles")
	}
}

// TestSequence_TwoRunningChildrenRecordsMisuse is the red-first case for the
// "one Running child" heart contract on a Sequence: promoting a
// second sibling to Running while the first is still Running is misuse.
func TestSequence_TwoRunningChildrenRecordsMisuse(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })

	setup := out.Sequence("python")
	scan := setup.Task("scan")
	venv := setup.Task("venv")

	scan.Doing("scanning")      // promotes scan to Running
	venv.Doing("creating venv") // second sibling Running while scan still is

	if err := out.err(); err == nil {
		t.Fatal("want misuse recorded for two Running siblings in a Sequence")
	}
}

// TestSequence_CascadeFailureNotStartsLaterSiblings proves Sequence's
// defining behavior: once a child fails, every later-declared unresolved
// sibling auto-resolves to NotStarted ("-  <name>  not started") with no
// caller code — the same contract the deleted Group type carried,
// now under its P3 name.
func TestSequence_CascadeFailureNotStartsLaterSiblings(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{isolated: true, stdout: &buf, color: colorNever, plain: true})
	t.Cleanup(func() { _ = out.close() })

	setup := out.Sequence("python")
	scan := setup.Task("scan")
	venv := setup.Task("venv")
	install := setup.Task("install")

	succeed(scan)
	venv.fail("uv exited 1")

	if err := out.finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if got := install.snapshot().State; got != notStarted {
		t.Fatalf("install state = %v, want NotStarted", got)
	}
	if !strings.Contains(buf.String(), "- install  not started") {
		t.Fatalf("rendered output missing \"- install  not started\":\n%s", buf.String())
	}
}

// TestSequence_CascadeNestedSequenceFailurePropagatesToRootHeader
// proves the recursive nesting P3 adds: a Sequence declared under another
// Sequence via .Sequence(name) still cascades within itself, and its
// failure surfaces at the root container's own derived header state.
func TestSequence_CascadeNestedSequenceFailurePropagatesToRootHeader(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: &bytes.Buffer{}, color: colorNever, plain: true})
	t.Cleanup(func() { _ = out.close() })

	root := out.Sequence("release")
	succeed(root.Task("build"))
	python := root.Sequence("python")
	scan := python.Task("scan")
	venv := python.Task("venv")
	install := python.Task("install")

	succeed(scan)
	venv.fail("uv exited 1")
	_ = out.finish()

	if got := install.snapshot().State; got != notStarted {
		t.Fatalf("nested install state = %v, want NotStarted", got)
	}
	if got := root.snapshot().State; got != failed {
		t.Fatalf("root sequence state = %v, want Failed (nested failure must surface)", got)
	}
}
