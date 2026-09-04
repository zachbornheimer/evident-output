package evo_test

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// This file golden-proves Stage E1 of the v0.4.0 redesign (P1: caller
// reports success/effects, evo derives the result; P2: warnings annotate
// lifecycle, never replace it; P9: distinct lifecycle states). Each test
// names the pinned decision it covers; the work order's final report
// captures these running RED against the pre-E1 code and GREEN after.

// --- P1: mutation-verb effect boundary ---------------------------------

// TestE1P1_MutationVerb_SuccessCommitsChangedEffect proves the ordinary
// path: call executes, succeeds, and the effect commits into the Changes
// ledger — evo derives StateChanged, the caller never chose it.
func TestE1P1_MutationVerb_SuccessCommitsChangedEffect(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})

	called := false
	branches := out.Task("branches")
	if err := branches.Delete("stale local branch", func() error {
		called = true
		return nil
	}, evo.Affected(2)); err != nil {
		t.Fatalf("Delete() = %v, want nil", err)
	}
	branches.Done()

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("want call executed on a normal (non-dry-run) run")
	}
	if got := out.Conclusion().State; got != evo.StateChanged {
		t.Fatalf("state = %v, want StateChanged", got)
	}
	if !strings.Contains(buf.String(), "deleted 2 stale local branches") {
		t.Fatalf("want the derived past-tense ledger row, got:\n%s", buf.String())
	}
}

// TestE1P1_MutationVerb_NilCallRecordsWithoutExecuting proves call == nil
// still commits the effect (there is nothing to execute, so nothing can
// fail) on a normal run.
func TestE1P1_MutationVerb_NilCallRecordsWithoutExecuting(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})

	branches := out.Task("branches")
	if err := branches.Delete("stale local branch", nil, evo.Affected(2)); err != nil {
		t.Fatalf("Delete() = %v, want nil", err)
	}
	branches.Done()

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if got := out.Conclusion().State; got != evo.StateChanged {
		t.Fatalf("state = %v, want StateChanged", got)
	}
}

// TestE1P1_MutationVerb_CallErrorCommitsNothing proves a failing call
// commits no effect and returns the error verbatim — the caller decides
// Fail/Block from there, evo never guesses.
func TestE1P1_MutationVerb_CallErrorCommitsNothing(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard), evo.NoColor(), evo.Plain()}})

	branches := out.Task("branches")
	wantErr := errors.New("permission denied")
	err := branches.Delete("stale local branch", func() error { return wantErr }, evo.Affected(2))
	if err == nil {
		t.Fatal("Delete() = nil, want the call's error")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("Delete() = %v, want it to wrap %v", err, wantErr)
	}
	_ = branches.Failf("delete stale branches: %w", err)

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	snap := out.Snapshot()
	for _, ch := range snap.Changes {
		if len(ch.Records) > 0 {
			t.Fatalf("want no committed records after a call error, got %+v", ch.Records)
		}
	}
	if got := out.Conclusion().State; got != evo.StateFailed {
		t.Fatalf("state = %v, want StateFailed", got)
	}
}

// TestE1P1_MutationVerb_DryRunNeverExecutesCallAndPlansEffect proves the
// dry-run half: call is never invoked, and the effect is recorded as
// planned, never changed.
func TestE1P1_MutationVerb_DryRunNeverExecutesCallAndPlansEffect(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain(), evo.DryRun()}})

	called := false
	branches := out.Task("branches")
	if err := branches.Delete("stale local branch", func() error {
		called = true
		return nil
	}, evo.Affected(2)); err != nil {
		t.Fatalf("Delete() = %v, want nil", err)
	}
	branches.Done()

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("dry run must never execute the call")
	}
	if got := out.Conclusion().State; got != evo.StatePlanned {
		t.Fatalf("state = %v, want StatePlanned", got)
	}
	if !strings.Contains(buf.String(), "delete 2 stale local branches") {
		t.Fatalf("want the imperative planned ledger row, got:\n%s", buf.String())
	}
}

// --- P2: warnings annotate, never resolve -------------------------------

// TestE1P2_Warn_SingleShortWarningInlinesOnDoneRow proves the documented
// compact form: one short warning renders directly on the task's own ✓ row.
func TestE1P2_Warn_SingleShortWarningInlinesOnDoneRow(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})

	branches := out.Task("branches")
	branches.Warn("kept 11 (7 protected, 4 unpushed)")
	branches.Done()

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	// E2.5 finding 3: the inline warning carries the same "! " bang the
	// normative repo-retire dry-run fixture uses ("! kept 13 (...)") — an
	// inline and a nested warning must signal identically, one row, one line.
	if !strings.Contains(got, "✓ branches  ! kept 11 (7 protected, 4 unpushed)\n") {
		t.Fatalf("want the warning inlined on the ✓ row with its \"! \" prefix, got:\n%s", got)
	}
	if strings.Count(got, "!") != 1 {
		t.Fatalf("a single short warning must inline exactly once, not also render a nested ! line, got:\n%s", got)
	}
}

// TestE1P2_Warn_MultipleWarningsNestUnderneath proves the second documented
// form: more than one warning moves off the row onto its own nested "!"
// lines below it.
func TestE1P2_Warn_MultipleWarningsNestUnderneath(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})

	branches := out.Task("branches")
	branches.Warn("kept 11 (7 protected, 4 unpushed)")
	branches.Warn("2 remotes unreachable")
	branches.Done()

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

// TestE1P2_Warn_DoesNotResolveTask proves Warn is non-terminal: the task
// stays Pending immediately after Warn, and a later Done still resolves it.
func TestE1P2_Warn_DoesNotResolveTask(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})

	task := out.Task("cache")
	task.Warn("stale entry ignored")
	if got := task.Snapshot().State; got != evo.Pending {
		t.Fatalf("state = %v, want Pending (Warn must not resolve the task)", got)
	}
	task.Done()
	if got := task.Snapshot().State; got != evo.Done {
		t.Fatalf("state = %v, want Done", got)
	}
	_ = out.Finish()
}

// TestE1P2_Warn_UnresolvedTaskAutoResolvesDoneAtFinish proves a task that
// only ever calls Warn (no terminal verb) auto-resolves Done at Finish, the
// same amnesty a recorded effect or sealed progress already gets.
func TestE1P2_Warn_UnresolvedTaskAutoResolvesDoneAtFinish(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})

	out.Task("cache").Warn("stale entry ignored")
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

// --- P9: distinct lifecycle states ---------------------------------------

// TestE1P9_LifecycleStatesAreDistinct golden-proves Done/Failed/Blocked/
// Cancelled/NotStarted each render their own distinct glyph and text —
// different causes render differently, never a collapsed generic failure.
func TestE1P9_LifecycleStatesAreDistinct(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})

	out.Task("done-task").Done()
	out.Task("failed-task").Fail("build broke")
	out.Task("blocked-task").Block("needs confirmation")

	seq := out.Sequence("cancel-sequence")
	first, second := seq.Task("first"), seq.Task("second")
	first.Cancel("interrupted")
	_ = second // never resolved; Finish's group lifecycle marks it NotStarted

	if err := out.Finish(); err != nil {
		t.Log(err)
	}

	got := buf.String()
	for _, want := range []string{
		"✓ done-task",
		"✗ failed-task  build broke",
		"⊘ blocked-task  needs confirmation",
		"■ first   interrupted",
		"- second  not started",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q distinctly rendered, got:\n%s", want, got)
		}
	}
}

// TestE1P9_DeclinedConfirm_ResolvesBlockedWithExitOne golden-proves the
// doc's example: a declined confirmation is Blocked, never Failed, and the
// process-level exit code is 1 (ExitBlocked), not 2 (ExitFailed).
func TestE1P9_DeclinedConfirm_ResolvesBlockedWithExitOne(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})

	// Plain/non-interactive mode without AssumeYes blocks by policy rather
	// than reading stdin — the same declined-by-policy path a real TTY
	// decline resolves through.
	if confirmed := out.Confirm("delete origin/production-hotfix?"); confirmed {
		t.Fatal("Confirm() = true, want false")
	}

	if err := out.Finish(); err != nil {
		t.Log(err)
	}
	conc := out.Conclusion()
	if conc.State != evo.StateBlocked {
		t.Fatalf("state = %v, want StateBlocked (a declined confirm is Blocked, never Failed)", conc.State)
	}
	if conc.ExitCode != evo.ExitBlocked {
		t.Fatalf("exit code = %d, want %d (ExitBlocked)", conc.ExitCode, evo.ExitBlocked)
	}
}
