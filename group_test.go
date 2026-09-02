package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestGroup_FailureAutoResolvesLaterSiblingsToNotStarted is the red-first
// case for evo-rec.md item #3: after a failure, the user can't tell whether
// later steps ran unless the group renders them "-  <name>  not started"
// with no caller code.
func TestGroup_FailureAutoResolvesLaterSiblingsToNotStarted(t *testing.T) {
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.To(&buf), evo.Plain(), evo.NoColor())
	t.Cleanup(func() { _ = out.Close() })

	setup := out.Group("python")
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
	if got := install.Snapshot().Summary; got != "not started" {
		t.Fatalf("install summary = %q, want %q", got, "not started")
	}
	if !strings.Contains(buf.String(), "-  install  not started") {
		t.Fatalf("rendered output missing \"-  install  not started\":\n%s", buf.String())
	}
}

// TestGroup_EarlierCompletedSiblingKeepsItsResolvedState covers "earlier
// Done rows are never erased" once a later sibling fails.
func TestGroup_EarlierCompletedSiblingKeepsItsResolvedState(t *testing.T) {
	out := evo.NewWithOptions(evo.To(&bytes.Buffer{}), evo.Plain(), evo.NoColor())
	t.Cleanup(func() { _ = out.Close() })

	setup := out.Group("python")
	scan := setup.Task("scan")
	venv := setup.Task("venv")

	scan.Done()
	venv.Fail("uv exited 1")
	_ = out.Finish()

	if got := scan.Snapshot().State; got != evo.Done {
		t.Fatalf("scan state = %v, want Done (must not be rewritten)", got)
	}
}

// TestGroup_ExplicitResolutionWinsOverAutoResolution: a caller that resolved
// a later sibling itself before Finish keeps that resolution — the library
// never overwrites a caller's explicit disposition.
func TestGroup_ExplicitResolutionWinsOverAutoResolution(t *testing.T) {
	out := evo.NewWithOptions(evo.To(&bytes.Buffer{}), evo.Plain(), evo.NoColor())
	t.Cleanup(func() { _ = out.Close() })

	setup := out.Group("python")
	scan := setup.Task("scan")
	venv := setup.Task("venv")
	extras := setup.Task("extras")

	scan.Done()
	venv.Fail("uv exited 1")
	extras.Skip("optional, not needed")

	_ = out.Finish()

	if got := extras.Snapshot().State; got != evo.Skipped {
		t.Fatalf("extras state = %v, want Skipped (explicit resolution must win)", got)
	}
}

// TestGroup_CancelAutoResolvesLaterSiblings covers the SIGINT path
// (evo-rec.md "early termination"): the active task cancels and later
// pending siblings still render "-  not started".
func TestGroup_CancelAutoResolvesLaterSiblings(t *testing.T) {
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.To(&buf), evo.Plain(), evo.NoColor())
	t.Cleanup(func() { _ = out.Close() })

	setup := out.Group("python")
	scan := setup.Task("scan")
	venv := setup.Task("venv")
	install := setup.Task("install")

	scan.Done()
	venv.Cancel("interrupted")

	_ = out.Finish()

	if got := install.Snapshot().State; got != evo.NotStarted {
		t.Fatalf("install state = %v, want NotStarted", got)
	}
	if !strings.Contains(buf.String(), "-  install  not started") {
		t.Fatalf("rendered output missing \"-  install  not started\":\n%s", buf.String())
	}
}

// TestGroup_ConclusionAndExitCodeComeFromFailedChildNotFromNotStarted proves
// NotStarted rows never count as failure in the Conclusion — the verdict and
// exit code come from the failed child alone.
func TestGroup_ConclusionAndExitCodeComeFromFailedChildNotFromNotStarted(t *testing.T) {
	out := evo.NewWithOptions(evo.To(&bytes.Buffer{}), evo.Plain(), evo.NoColor())
	t.Cleanup(func() { _ = out.Close() })

	setup := out.Group("python")
	setup.Task("scan").Done()
	setup.Task("venv").Fail("uv exited 1")
	setup.Task("install")

	_ = out.Finish()

	conc := out.Conclusion()
	if conc.State != evo.StateFailed {
		t.Fatalf("conclusion state = %v, want StateFailed", conc.State)
	}
	if conc.ExitCode != evo.ExitFailed {
		t.Fatalf("exit code = %d, want %d (ExitFailed)", conc.ExitCode, evo.ExitFailed)
	}
}

// TestGroup_AllChildrenDoneRendersAsToday is the non-regression control: a
// group with no failure/cancellation renders exactly as before this change.
func TestGroup_AllChildrenDoneRendersAsToday(t *testing.T) {
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.To(&buf), evo.Plain(), evo.NoColor())
	t.Cleanup(func() { _ = out.Close() })

	setup := out.Group("python")
	setup.Task("scan").Done()
	setup.Task("venv").Done()
	setup.Task("install").Done()

	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if strings.Contains(buf.String(), "not started") {
		t.Fatalf("unexpected \"not started\" row in an all-done group:\n%s", buf.String())
	}
	if got := setup.Snapshot().State; got != evo.Done {
		t.Fatalf("group state = %v, want Done", got)
	}
}

// TestGroup_PackageLevelGetOrCreate mirrors evo.Task's identity contract:
// evo.Group(name) called twice returns the same handle, and Group.Task(name)
// called twice within one group returns the same child.
func TestGroup_PackageLevelGetOrCreate(t *testing.T) {
	var buf bytes.Buffer
	evo.SetDefault(evo.NewWithOptions(evo.To(&buf), evo.Plain(), evo.NoColor()))
	t.Cleanup(func() { _ = evo.Default().Close() })

	g1 := evo.Group("python")
	g2 := evo.Group("python")
	if g1 != g2 {
		t.Fatal("evo.Group(name) called twice returned different handles")
	}

	t1 := g1.Task("venv")
	t2 := g2.Task("venv")
	if t1 != t2 {
		t.Fatal("Group.Task(name) called twice returned different handles")
	}
}
