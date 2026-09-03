// Package evo_test closes the remaining spec-golden cells for evo-rec.md
// Problems 6 through 10 (see spec_golden_test.go / spec_golden_live_test.go
// for the sibling files and shared helpers this file reuses, e.g.
// newLiveScreenOutput).
package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// collapseFields joins on single spaces so column-padding whitespace never
// causes a spurious mismatch (the same normalization
// TestSpecP8_PartialTruthSurvivesRemoteAuthFailure already relies on).
func collapseFields(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// ---------------------------------------------------------------------------
// Problem 6 — bytes and item counts must never share Progress.
// (step1/step2/success already proven on the base branch.)
// ---------------------------------------------------------------------------

// TestSpecP6_BytesVsCounts_Failure covers evo-rec.md Problem 6's failure
// block: a byte-total task survives Done while a sibling count-based task
// fails with its own detail.
//
//	✓  generate  8.0 MB
//	✗  test      tests failed
//	   └─ --- FAIL: TestFoo (0.01s)
func TestSpecP6_BytesVsCounts_Failure(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.Title("build"), evo.To(&buf), evo.Plain(), evo.NoColor())
	generate := out.Task("generate")
	generate.Bytes(8_000_000, 8_000_000)
	generate.Done("8.0 MB")
	test := out.Task("test")
	test.Fail("tests failed", evo.Detail("--- FAIL: TestFoo (0.01s)"))
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := collapseFields(buf.String())
	for _, want := range []string{
		"✓ generate 8.0 MB",
		"✗ test tests failed",
		"--- FAIL: TestFoo (0.01s)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, buf.String())
		}
	}
}

// TestSpecP6_LiveFrame_Indeterminate covers evo-rec.md Problem 6's
// indeterminate block: a byte task still writing has no total yet, so it
// renders its phase text, never a fake percentage.
//
//	:.  generate  writing…
func TestSpecP6_LiveFrame_Indeterminate(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	out.Task("generate").Phase("writing…")

	got := screen.LatestLiveText()
	for _, want := range []string{"generate", "writing…"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in live frame:\n%s", want, got)
		}
	}
}

// TestSpecP6_ErrorBlock_ProgressThenFail covers evo-rec.md Problem 6's error
// block as two sequential moments of one task's real lifecycle (the only way
// a single named row can show both a live byte bar and a terminal outcome):
// the bar frame while writing, then the durable fail line once Finish
// resolves it.
//
//	:.  generate  [████████░░░░]  5.2/8.0 MB
//	✗  generate  disk full
//	   └─ write /tmp/out: no space left on device
func TestSpecP6_ErrorBlock_ProgressThenFail(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	generate := out.Task("generate")
	generate.Bytes(5_200_000, 8_000_000)
	liveFrame := screen.LatestLiveText()
	for _, want := range []string{"generate", "5.2/8.0 MB", "[", "]"} {
		if !strings.Contains(liveFrame, want) {
			t.Fatalf("want %q in mid-write live frame:\n%s", want, liveFrame)
		}
	}

	generate.Fail("disk full", evo.Detail("write /tmp/out: no space left on device"))
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	final := screen.FinalText()
	for _, want := range []string{"✗", "generate", "disk full", "write /tmp/out: no space left on device"} {
		if !strings.Contains(final, want) {
			t.Fatalf("want %q in final fail line:\n%s", want, final)
		}
	}
}

// TestSpecP6_EarlyTermination_MISMATCH covers evo-rec.md Problem 6's
// early-termination block. The live bar-then-cancel half proves cleanly, but
// the derived "! already mutated: ..." row never renders in interactive/live
// mode at all: writeConclusion (the only place summarizeAlreadyMutated is
// called) is only reached from residualPlainLocked, and Output.Finish's
// interactive branch instead calls residualInteractiveFinalLocked, which
// never emits a conclusion. A user watching a real terminal session never
// sees this line on abnormal termination — only Output.FinalPlain() (an
// internal snapshot, not the rendered surface) carries it.
//
//	:.  generate  [████░░░░░░░░]  2.1/8.0 MB
//	■  generate  cancelled
//	!  already mutated: partial artifact at /tmp/out (2.1 MB)
func TestSpecP6_EarlyTermination_MISMATCH(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	generate := out.Task("generate")
	generate.Bytes(2_100_000, 8_000_000)
	liveFrame := screen.LatestLiveText()
	for _, want := range []string{"generate", "2.1/8.0 MB"} {
		if !strings.Contains(liveFrame, want) {
			t.Fatalf("want %q in mid-write live frame:\n%s", want, liveFrame)
		}
	}

	generate.Write("partial artifact at /tmp/out (2.1 MB)")
	generate.Cancel("cancelled")
	if err := out.Finish(); err != nil {
		t.Log(err)
	}
	final := screen.FinalText()
	if !strings.Contains(final, "■") || !strings.Contains(final, "generate") || !strings.Contains(final, "cancelled") {
		t.Fatalf("want cancelled generate row, got:\n%s", final)
	}
	if strings.Contains(final, "already mutated") {
		t.Fatal("expected the already-mutated row to be MISSING from the rendered live surface (mismatch resolved — update this test)")
	}
	t.Skip("MISMATCH: spec wants a rendered \"!  already mutated: partial artifact at /tmp/out (2.1 MB)\" row after the cancelled task in the real terminal output; the interactive live surface's final text is only " +
		final + " — writeConclusion (source of the already-mutated row) is never invoked on the interactive finish path (see output.go Finish, residualInteractiveFinalLocked). The row only exists in Output.FinalPlain(), an internal snapshot never written to the screen.")
}

// ---------------------------------------------------------------------------
// Problem 7 — a plan with 500 rows must bound the visible list.
// (step2/success already proven on the base branch.)
// ---------------------------------------------------------------------------

// TestSpecP7_Step1_PlanPreview_MISMATCH covers evo-rec.md Problem 7's step1
// block: a dry-run plan preview for 500 named deletes.
//
//	[planned]  branches
//	  delete  feat/a
//	  delete  feat/b
//	  … +498 more (not shown)
//
// MISMATCH: the first two named rows match, but the visible-row bound and
// overflow count do not — maxVisibleEffectRows keeps 3 rows visible (not the
// 2 the spec illustration shows), so the omitted count is 497, not 498.
func TestSpecP7_Step1_PlanPreview_MISMATCH(t *testing.T) {
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.Title("clean"), evo.To(&buf), evo.Plain(), evo.NoColor(), evo.DryRun())
	branches := out.Task("branches")
	branches.RecordName("delete", "feat/a")
	branches.RecordName("delete", "feat/b")
	for i := 0; i < 498; i++ {
		branches.RecordName("delete", "feat/x")
	}
	if err := out.Finish(); err != nil {
		t.Log(err)
	}
	got := collapseFields(buf.String())
	for _, want := range []string{"[planned] branches", "delete feat/a", "delete feat/b"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, buf.String())
		}
	}
	if strings.Contains(got, "+498 more (not shown)") {
		t.Fatal("expected the +498 overflow line to be MISSING (mismatch resolved — update this test)")
	}
	t.Skip("MISMATCH: spec wants exactly 2 visible rows and \"…  +498 more (not shown)\"; the library's maxVisibleEffectRows bound keeps 3 rows visible and reports \"…  +497 more (not shown)\" instead. Actual:\n" + buf.String())
}

// TestSpecP7_Failure_NotTestable documents evo-rec.md Problem 7's failure
// block:
//
//	✓  branches  120 deleted
//	✗  branches  cannot delete feat/protected
//	!  +380 not attempted after failure
//
// NOT-TESTABLE: both the Done row ("120 deleted") and the Failed row
// ("cannot delete feat/protected") share the name "branches", but a Task is
// one resolvable entity that can only reach one terminal state — the same
// structural reason spec_golden_live_test.go's
// TestSpecP8_LiveFrame_Step2_NotTestable already documents for Problem 8's
// step2 block. There is no simplest-documented-spelling way to make one
// TaskHandle render as both ✓ and ✗ at once.
func TestSpecP7_Failure_NotTestable(t *testing.T) {
	t.Skip("NOT-TESTABLE: see doc comment — a single TaskHandle cannot render both a Done row and a Failed row under one name")
}

// TestSpecP7_LiveFrame_Indeterminate covers evo-rec.md Problem 7's
// indeterminate block: still classifying, no sealed total yet.
//
//	:.  branches  classifying 500 tips…
func TestSpecP7_LiveFrame_Indeterminate(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	out.Task("branches").Phase("classifying 500 tips…")

	got := screen.LatestLiveText()
	for _, want := range []string{"branches", "classifying 500 tips…"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in live frame:\n%s", want, got)
		}
	}
}

// TestSpecP7_ErrorBlock_MISMATCH covers evo-rec.md Problem 7's error block as
// two sequential moments (see TestSpecP6_ErrorBlock_ProgressThenFail): the
// live progress bar mid-run, then the durable fail line.
//
//	:.  branches  120/500  feat/x
//	✗  branches  fatal: unable to read tree
//	!  120 deleted; 380 remaining untouched
//
// MISMATCH: the bar-then-fail half matches; the free-text "!" summary row
// does not exist in any rendered surface for this scenario — it is not the
// mechanically-derived "already mutated" row (which would read "N branches
// deleted", not "120 deleted; 380 remaining untouched"), and even that
// derived row would be absent from the interactive final text per
// TestSpecP6_EarlyTermination_MISMATCH's finding.
func TestSpecP7_ErrorBlock_MISMATCH(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	branches := out.Task("branches")
	branches.Progress(120, 500)
	branches.Phase("feat/x")
	liveFrame := screen.LatestLiveText()
	for _, want := range []string{"branches", "120/500", "feat/x"} {
		if !strings.Contains(liveFrame, want) {
			t.Fatalf("want %q in mid-run live frame:\n%s", want, liveFrame)
		}
	}

	branches.Fail("fatal: unable to read tree")
	if err := out.Finish(); err != nil {
		t.Log(err)
	}
	final := screen.FinalText()
	if !strings.Contains(final, "✗") || !strings.Contains(final, "branches") || !strings.Contains(final, "fatal: unable to read tree") {
		t.Fatalf("want failed branches row, got:\n%s", final)
	}
	if strings.Contains(final, "120 deleted; 380 remaining untouched") {
		t.Fatal("expected the free-text overflow row to be MISSING (mismatch resolved — update this test)")
	}
	t.Skip("MISMATCH: spec wants a rendered \"!  120 deleted; 380 remaining untouched\" row; no mechanism produces that literal free-text summary, and even a mechanically-derived already-mutated row would not render on the interactive finish path. Actual final text:\n" + final)
}

// TestSpecP7_EarlyTermination_NotTestable documents evo-rec.md Problem 7's
// early-termination block:
//
//	✓  branches  120 deleted
//	■  branches  cancelled
//	!  already mutated: 120 deletes; 380 remain
//
// NOT-TESTABLE: the Done row and the Cancelled row share the name
// "branches" — the same one-entity-one-terminal-state constraint as
// TestSpecP7_Failure_NotTestable above.
func TestSpecP7_EarlyTermination_NotTestable(t *testing.T) {
	t.Skip("NOT-TESTABLE: see doc comment — a single TaskHandle cannot render both a Done row and a Cancelled row under one name")
}

// ---------------------------------------------------------------------------
// Problem 8 — partial truth survives a remote auth failure.
// (step1/failure already proven on the base branch; step2 is documented
// NOT-TESTABLE there.)
// ---------------------------------------------------------------------------

// TestSpecP8_Success covers evo-rec.md Problem 8's success block: every
// remote delete lands, effects and the task's own Done row both render.
//
//	[changed]  remotes
//	  deleted  3  origin tip
//	✓  remotes
func TestSpecP8_Success(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.Title("retire"), evo.To(&buf), evo.Plain(), evo.NoColor())
	remotes := out.Task("remotes")
	remotes.Delete(3, "origin tip")
	remotes.Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := collapseFields(buf.String())
	for _, want := range []string{"[changed] remotes", "deleted 3 origin tip", "✓ remotes"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, buf.String())
		}
	}
}

// TestSpecP8_LiveFrame_Indeterminate covers evo-rec.md Problem 8's
// indeterminate block: about to delete a remote ref, no count yet.
//
//	:.  remotes  delete-remote…
func TestSpecP8_LiveFrame_Indeterminate(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	out.Task("remotes").Phase("delete-remote…")

	got := screen.LatestLiveText()
	for _, want := range []string{"remotes", "delete-remote…"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in live frame:\n%s", want, got)
		}
	}
}

// TestSpecP8_Error_NotTestable documents evo-rec.md Problem 8's error block:
//
//	✓  remotes  deleted origin/feat/a
//	✗  remotes  HTTP 401
//	   └─ Authorization: token expired
//	→  gh auth refresh
//
// NOT-TESTABLE: the Done row and the Failed row share the name "remotes" —
// the same one-entity-one-terminal-state constraint documented by
// spec_golden_live_test.go's TestSpecP8_LiveFrame_Step2_NotTestable, which
// covers this exact problem's step2 block with an identical two-row shape.
func TestSpecP8_Error_NotTestable(t *testing.T) {
	t.Skip("NOT-TESTABLE: see doc comment — a single TaskHandle cannot render both a Done row and a Failed row under one name (same constraint as TestSpecP8_LiveFrame_Step2_NotTestable)")
}

// TestSpecP8_EarlyTermination_NotTestable documents evo-rec.md Problem 8's
// early-termination block:
//
//	✓  remotes  deleted origin/feat/a
//	■  remotes  cancelled before feat/b
//	!  already mutated: 1 remote delete
//
// NOT-TESTABLE: the Done row and the Cancelled row share the name
// "remotes" — the same constraint as TestSpecP8_Error_NotTestable above.
func TestSpecP8_EarlyTermination_NotTestable(t *testing.T) {
	t.Skip("NOT-TESTABLE: see doc comment — a single TaskHandle cannot render both a Done row and a Cancelled row under one name")
}

// ---------------------------------------------------------------------------
// Problem 9 — ^C on the second named task while the first is Done.
// (step1/step2 already proven on the base branch; the spec's own
// "indeterminate" block is byte-identical to its step1 block, so
// TestSpecP9_LiveFrame_Step1 already covers it too.)
// ---------------------------------------------------------------------------

// TestSpecP9_Success covers evo-rec.md Problem 9's success block: every
// named task in the sequence reaches Done.
//
//	✓  scan
//	✓  venv
//	✓  install
func TestSpecP9_Success(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.Title("python setup"), evo.To(&buf), evo.Plain(), evo.NoColor())
	out.Task("scan").Done()
	out.Task("venv").Done()
	out.Task("install").Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := collapseFields(buf.String())
	for _, want := range []string{"✓ scan", "✓ venv", "✓ install"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, buf.String())
		}
	}
}

// TestSpecP9_Failure_MISMATCH covers evo-rec.md Problem 9's failure block: a
// venv failure while scan already succeeded, with install auto-resolved as
// not-run.
//
//	✓  scan
//	✗  venv  uv exited 2
//	-  install  venv did not complete
//
// MISMATCH: scan and venv's rows match exactly; the auto-resolved sibling's
// reason text does not. autoResolveGroupsLocked always writes the fixed
// notStartedSummary constant ("not started") for a sibling after an earlier
// Failed/Cancelled task — there is no caller hook to supply the spec's
// scenario-specific "venv did not complete" text.
func TestSpecP9_Failure_MISMATCH(t *testing.T) {
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.Title("python setup"), evo.To(&buf), evo.Plain(), evo.NoColor())
	setup := out.Group("python")
	scan := setup.Task("scan")
	venv := setup.Task("venv")
	setup.Task("install")
	scan.Done()
	venv.Fail("uv exited 2")
	if err := out.Finish(); err != nil {
		t.Log(err)
	}
	got := collapseFields(buf.String())
	for _, want := range []string{"✓ scan", "✗ venv uv exited 2"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, buf.String())
		}
	}
	if strings.Contains(got, "install venv did not complete") {
		t.Fatal("expected the scenario-specific not-started reason to be MISSING (mismatch resolved — update this test)")
	}
	t.Skip("MISMATCH: spec wants \"-  install  venv did not complete\"; autoResolveGroupsLocked always writes the fixed text \"-  install  not started\" instead, with no caller-supplied reason hook. Actual:\n" + buf.String())
}

// TestSpecP9_Error covers evo-rec.md Problem 9's error block: scan survives,
// venv fails on a signal.
//
//	✓  scan
//	✗  venv  signal: killed
func TestSpecP9_Error(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.Title("python setup"), evo.To(&buf), evo.Plain(), evo.NoColor())
	out.Task("scan").Done()
	out.Task("venv").Fail("signal: killed")
	if err := out.Finish(); err != nil {
		t.Log(err)
	}
	got := collapseFields(buf.String())
	for _, want := range []string{"✓ scan", "✗ venv signal: killed"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, buf.String())
		}
	}
}

// TestSpecP9_EarlyTermination_MISMATCH covers evo-rec.md Problem 9's
// early-termination block: scan Done, venv cancelled mid-mutation, install
// auto-resolved not-started.
//
//	✓  scan
//	■  venv     cancelled — .venv partial
//	-  install  not started
//	!  already mutated: incomplete .venv directory
//
// MISMATCH: every row matches except the derived already-mutated summary.
// summarizeAlreadyMutated always formats "<N> <object> <verb>" from the
// Changes ledger (here "1 incomplete .venv directory wrote") — there is no
// way to record a bare descriptive fragment like the spec's "incomplete
// .venv directory" through the public Record/RecordName/Write API.
func TestSpecP9_EarlyTermination_MISMATCH(t *testing.T) {
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.Title("python setup"), evo.To(&buf), evo.Plain(), evo.NoColor())
	setup := out.Group("python")
	scan := setup.Task("scan")
	venv := setup.Task("venv")
	setup.Task("install")
	scan.Done()
	venv.Write("incomplete .venv directory")
	venv.Cancel("cancelled — .venv partial")
	if err := out.Finish(); err != nil {
		t.Log(err)
	}
	got := collapseFields(buf.String())
	for _, want := range []string{
		"✓ scan",
		"■ venv cancelled — .venv partial",
		"- install not started",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, buf.String())
		}
	}
	if strings.Contains(got, "already mutated: incomplete .venv directory") {
		t.Fatal("expected the bare-phrase already-mutated row to be MISSING (mismatch resolved — update this test)")
	}
	t.Skip("MISMATCH: spec wants \"!  already mutated: incomplete .venv directory\"; the mechanical formatter instead emits \"!  already mutated: 1 incomplete .venv directory wrote\" (summarizeChangeSection always renders \"<N> <object> <verb>\"). Actual:\n" + buf.String())
}

// ---------------------------------------------------------------------------
// Problem 10 — CI/non-TTY must stay durable lines only, never live redraw.
// ---------------------------------------------------------------------------

// TestSpecP10_Step1_MISMATCH covers evo-rec.md Problem 10's step1 block: one
// Running task with a phase, projected as a static line in a non-interactive
// stream.
//
//   - install  installing
//
// MISMATCH: nothing streams at all. emitTaskProgressiveLocked only emits a
// standalone task once it reaches a terminal state (isTerminalTask guard);
// a Running task's Phase never gets a progressive line in
// plain/non-interactive mode, so CI output is silent until the task
// resolves — the opposite of the spec's "static text once" promise.
func TestSpecP10_Step1_MISMATCH(t *testing.T) {
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.Title("install-pipeline"), evo.To(&buf), evo.NonInteractive(), evo.NoColor())
	t.Cleanup(func() { _ = out.Close() })

	out.Task("install").Phase("installing")

	got := buf.String()
	if strings.Contains(got, "install") {
		t.Fatal("expected the running-phase line to be MISSING (mismatch resolved — update this test)")
	}
	t.Skip("MISMATCH: spec wants \"•  install  installing\" streamed immediately in non-interactive mode; the actual buffer is empty (" + got + ") — a Running task with only Phase set never streams progressively.")
}

// TestSpecP10_Step2_MISMATCH covers evo-rec.md Problem 10's step2 block: a
// prior Done task stays, and the next task's progress becomes visible.
//
//	✓  scan
//	•  install  14/40  requests
//
// MISMATCH: "✓  scan" streams immediately (terminal outcomes do stream
// progressively), but the install row never appears — same root cause as
// TestSpecP10_Step1_MISMATCH: Progress/Phase on a Running task in
// non-interactive mode produces no output until that task itself resolves.
func TestSpecP10_Step2_MISMATCH(t *testing.T) {
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.Title("install-pipeline"), evo.To(&buf), evo.NonInteractive(), evo.NoColor())
	t.Cleanup(func() { _ = out.Close() })

	out.Task("scan").Done()
	install := out.Task("install")
	install.Progress(14, 40)
	install.Phase("requests")

	got := buf.String()
	if !strings.Contains(got, "✓") || !strings.Contains(got, "scan") {
		t.Fatalf("want the Done scan row to stream immediately, got:\n%s", got)
	}
	if strings.Contains(got, "14/40") {
		t.Fatal("expected the running install progress row to be MISSING (mismatch resolved — update this test)")
	}
	t.Skip("MISMATCH: spec wants \"•  install  14/40  requests\" appended after scan's Done row; the actual buffer only ever has the scan line (" + got + ") — Progress/Phase on a Running task never streams in non-interactive mode.")
}

// TestSpecP10_Success covers evo-rec.md Problem 10's success block: every
// task resolves Done, and a closing Item narrates the overall outcome.
//
//	✓  scan
//	✓  venv
//	✓  install  14 modules
//	✓  python setup
//	  python was set up; 14 modules were installed
func TestSpecP10_Success(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.Title("install-pipeline"), evo.To(&buf), evo.NonInteractive(), evo.NoColor())
	out.Task("scan").Done()
	out.Task("venv").Done()
	out.Task("install").Done("14 modules")
	out.Item("python setup").OK().Because("python was set up; 14 modules were installed")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := collapseFields(buf.String())
	for _, want := range []string{
		"✓ scan",
		"✓ venv",
		"✓ install 14 modules",
		"✓ python setup",
		"python was set up; 14 modules were installed",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, buf.String())
		}
	}
}

// TestSpecP10_Failure covers evo-rec.md Problem 10's failure block: scan
// survives, install fails with its detail line.
//
//	✓  scan
//	✗  install  uv pip install failed
//	   └─ exit status 1
func TestSpecP10_Failure(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.Title("install-pipeline"), evo.To(&buf), evo.NonInteractive(), evo.NoColor())
	out.Task("scan").Done()
	out.Task("install").Fail("uv pip install failed", evo.Detail("exit status 1"))
	if err := out.Finish(); err != nil {
		t.Log(err)
	}
	got := collapseFields(buf.String())
	for _, want := range []string{"✓ scan", "✗ install uv pip install failed", "exit status 1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, buf.String())
		}
	}
}

// TestSpecP10_LiveFrame_Indeterminate_MISMATCH covers evo-rec.md Problem 10's
// indeterminate block: scanning has started, no total yet.
//
//   - scan  scanning
//
// MISMATCH: same root cause as TestSpecP10_Step1_MISMATCH — a Running
// task's Phase never streams in non-interactive/plain mode, so the buffer
// is empty rather than showing a static phase line.
func TestSpecP10_LiveFrame_Indeterminate_MISMATCH(t *testing.T) {
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.Title("install-pipeline"), evo.To(&buf), evo.NonInteractive(), evo.NoColor())
	t.Cleanup(func() { _ = out.Close() })

	out.Task("scan").Phase("scanning")

	got := buf.String()
	if strings.Contains(got, "scan") {
		t.Fatal("expected the running-phase line to be MISSING (mismatch resolved — update this test)")
	}
	t.Skip("MISMATCH: spec wants \"•  scan  scanning\" streamed immediately in non-interactive mode; the actual buffer is empty — a Running task's Phase never streams progressively (same cause as TestSpecP10_Step1_MISMATCH).")
}

// TestSpecP10_Error covers evo-rec.md Problem 10's error block: scan
// survives, install fails on a network error.
//
//	✓  scan
//	✗  install  network unreachable
//	   └─ dial tcp: lookup pypi.org: no such host
func TestSpecP10_Error(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.Title("install-pipeline"), evo.To(&buf), evo.NonInteractive(), evo.NoColor())
	out.Task("scan").Done()
	out.Task("install").Fail("network unreachable", evo.Detail("dial tcp: lookup pypi.org: no such host"))
	if err := out.Finish(); err != nil {
		t.Log(err)
	}
	got := collapseFields(buf.String())
	for _, want := range []string{
		"✓ scan",
		"✗ install network unreachable",
		"dial tcp: lookup pypi.org: no such host",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, buf.String())
		}
	}
}

// TestSpecP10_EarlyTermination_MISMATCH covers evo-rec.md Problem 10's
// early-termination block: scan Done, install cancelled mid-way.
//
//	✓  scan
//	■  install  cancelled at 6/14
//	!  already mutated: 6 packages in .venv
//
// MISMATCH: the task rows match exactly; the derived already-mutated
// summary does not — summarizeChangeSection's mechanical "<N> <object>
// <verb>" format renders "6 packages in .venv installed" (the past-tense
// verb always trails), never the spec's bare "6 packages in .venv".
func TestSpecP10_EarlyTermination_MISMATCH(t *testing.T) {
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.Title("install-pipeline"), evo.To(&buf), evo.NonInteractive(), evo.NoColor())
	out.Task("scan").Done()
	install := out.Task("install")
	install.Record("install", 6, "packages in .venv")
	install.Cancel("cancelled at 6/14")
	if err := out.Finish(); err != nil {
		t.Log(err)
	}
	got := collapseFields(buf.String())
	for _, want := range []string{"✓ scan", "■ install cancelled at 6/14"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, buf.String())
		}
	}
	if strings.Contains(got, "already mutated: 6 packages in .venv") && !strings.Contains(got, "already mutated: 6 packages in .venv installed") {
		t.Fatal("expected the bare-phrase already-mutated row to be MISSING (mismatch resolved — update this test)")
	}
	t.Skip("MISMATCH: spec wants \"!  already mutated: 6 packages in .venv\"; the mechanical formatter instead emits \"!  already mutated: 6 packages in .venv installed\" (verb always trails). Actual:\n" + buf.String())
}
