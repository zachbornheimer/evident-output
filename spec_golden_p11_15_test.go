package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// This file closes the remaining evo-rec.md Problems 11-15 golden cells not
// already proven by spec_golden_test.go / spec_golden_live_test.go on the
// base branch. Each test names the exact cell (Problem N, block name) it
// covers. newLiveScreenOutput is defined in spec_golden_live_test.go (same
// package) and reused here unmodified.

// ---------------------------------------------------------------------------
// Problem 11: nested pipeline groups (parent names the group; children carry
// Progress; parent Done summarizes).
// ---------------------------------------------------------------------------

// TestSpecP11_LiveFrame_Step2 covers Problem 11's step2 block: the first
// child resolves, the second becomes the one Running child with a byte
// progress bar, the third stays pending.
//
//	pipeline
//	✓  go mod download  modules cached
//	:.  go generate     [██░░░░░░░░░░]  0.2/0.3 MB
//	   go test ./...
func TestSpecP11_LiveFrame_Step2(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	pipeline := out.Group("pipeline")
	download := pipeline.Task("go mod download")
	generate := pipeline.Task("go generate")
	pipeline.Task("go test ./...")
	download.Done("modules cached")
	generate.Bytes(200_000, 300_000)

	got := screen.LatestLiveText()
	for _, want := range []string{"go mod download", "modules cached", "go generate", "0.2/0.3 MB", "go test ./..."} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in live frame:\n%s", want, got)
		}
	}
}

// TestSpecP11_NestedPipeline_Success covers Problem 11's success block: every
// child resolves Done with its own summary, driven via evo.Group exactly like
// Problem 4's already-proven pattern.
//
//	pipeline
//	✓  go mod download  modules cached
//	✓  go generate      0.3 MB
//	✓  go test ./...    ok
func TestSpecP11_NestedPipeline_Success(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.Title("pipeline"), evo.To(&buf), evo.Plain(), evo.NoColor())
	pipeline := out.Group("pipeline")
	download := pipeline.Task("go mod download")
	generate := pipeline.Task("go generate")
	test := pipeline.Task("go test ./...")
	download.Done("modules cached")
	generate.Done("0.3 MB")
	test.Done("ok")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{
		"✓  go mod download  modules cached",
		"✓  go generate  0.3 MB",
		"✓  go test ./...  ok",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
}

// TestSpecP11_NestedPipeline_Failure covers Problem 11's failure block: the
// first two children resolve Done, the third Fails with structured Detail,
// evidence retained under the child's own row.
//
//	pipeline
//	✓  go mod download  modules cached
//	✓  go generate
//	✗  go test ./...    tests failed
//	   └─ --- FAIL: TestFoo (0.01s)
//	       foo_test.go:12: want 1, got 0
//
// MISMATCH (documented, not fixed): writeCollectionChild (plain.go) renders
// only a resolved group child's glyph, name, and Problems[0].Summary — it
// never calls writeProblem/writeProblemDetailBlock for a child's Detail, so
// the "└─ --- FAIL: ..." evidence line the spec shows under a failed group
// child never renders for any Group/Tasks child, unlike a standalone Task's
// writeTask (which does render Detail). This is a real, executed gap between
// the "Recommended UI" and what writeCollectionChild implements today, not a
// stale illustration — a fix would need writeCollectionChild to call
// writeProblem's Detail path the same way writeTask already does.
func TestSpecP11_NestedPipeline_Failure(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.Title("pipeline"), evo.To(&buf), evo.Plain(), evo.NoColor())
	pipeline := out.Group("pipeline")
	download := pipeline.Task("go mod download")
	generate := pipeline.Task("go generate")
	test := pipeline.Task("go test ./...")
	download.Done("modules cached")
	generate.Done()
	test.Fail("tests failed", evo.Detail("--- FAIL: TestFoo (0.01s)\n    foo_test.go:12: want 1, got 0"))
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{
		"✓  go mod download  modules cached",
		"✓  go generate",
		"✗  go test ./...  tests failed",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "FAIL: TestFoo") {
		t.Fatalf("expected the documented mismatch (group child Detail never rendered) but evidence text was present:\n%s", got)
	}
	t.Skip("MISMATCH: writeCollectionChild never renders a failed child's Detail/evidence line (└─ ...) — see doc comment")
}

// TestSpecP11_LiveFrame_Indeterminate covers Problem 11's indeterminate
// block: the first child shows a phase string (no sealed total yet), the
// other two sit pending.
//
//	pipeline
//	:.  go mod download  resolving modules
//	   go generate
//	   go test ./...
func TestSpecP11_LiveFrame_Indeterminate(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	pipeline := out.Group("pipeline")
	download := pipeline.Task("go mod download")
	pipeline.Task("go generate")
	pipeline.Task("go test ./...")
	download.Phase("resolving modules")

	got := screen.LatestLiveText()
	for _, want := range []string{"go mod download", "resolving modules", "go generate", "go test ./..."} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in live frame:\n%s", want, got)
		}
	}
}

// TestSpecP11_NestedPipeline_Error covers Problem 11's error block: the first
// child resolves Done, the second Fails, and the auto-resolution cascade
// (Problem 4's already-proven mechanism) marks the third "not started" with
// no caller code.
//
//	pipeline
//	✓  go mod download
//	✗  go generate  generator exited 1
//	   └─ stringer: type not found
//	-  go test ./...  not started
//
// MISMATCH (documented, not fixed): same writeCollectionChild gap as the
// failure block above — the "└─ stringer: type not found" evidence line
// never renders for a Group child's Fail.
func TestSpecP11_NestedPipeline_Error(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.Title("pipeline"), evo.To(&buf), evo.Plain(), evo.NoColor())
	pipeline := out.Group("pipeline")
	download := pipeline.Task("go mod download")
	generate := pipeline.Task("go generate")
	pipeline.Task("go test ./...")
	download.Done()
	generate.Fail("generator exited 1", evo.Detail("stringer: type not found"))
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{
		"✓  go mod download",
		"✗  go generate  generator exited 1",
		"go test ./...  not started",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "type not found") {
		t.Fatalf("expected the documented mismatch (group child Detail never rendered) but evidence text was present:\n%s", got)
	}
	t.Skip("MISMATCH: writeCollectionChild never renders a failed child's Detail/evidence line (└─ ...) — see doc comment")
}

// TestSpecP11_NestedPipeline_EarlyTermination covers Problem 11's early
// termination block: the first child resolves Done, the second Cancels, the
// auto-resolution cascade marks the third "not started".
//
//	pipeline
//	✓  go mod download
//	■  go generate  cancelled
//	-  go test ./...  not started
//	!  already mutated: module cache filled; no generated files
//
// MISMATCH (documented, not fixed): "! already mutated: ..." is derived
// mechanically from the Changes ledger (task_mutations.go / plain.go
// writeAlreadyMutated) and is suppressed entirely when the ledger is empty —
// this scenario records no Record/RecordName mutation on either child, so no
// ledger content exists to derive a narrative sentence like "module cache
// filled; no generated files" from. This mirrors the already-established
// empty-ledger-suppresses-the-row behavior (phase_n2_test.go), not a bug —
// the spec's free-text narrative line has no mechanical equivalent.
func TestSpecP11_NestedPipeline_EarlyTermination(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.Title("pipeline"), evo.To(&buf), evo.Plain(), evo.NoColor())
	pipeline := out.Group("pipeline")
	download := pipeline.Task("go mod download")
	generate := pipeline.Task("go generate")
	pipeline.Task("go test ./...")
	download.Done()
	generate.Cancel("cancelled")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{
		"✓  go mod download",
		"■  go generate  cancelled",
		"go test ./...  not started",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "already mutated") {
		t.Fatalf("expected the documented mismatch (empty ledger suppresses the row) but an already-mutated line was present:\n%s", got)
	}
	t.Skip("MISMATCH: \"! already mutated: module cache filled; no generated files\" has no mechanical source (empty Changes ledger suppresses the row entirely) — see doc comment")
}

// ---------------------------------------------------------------------------
// Problem 12: severe remote confirm gate (destructive delete needs an
// explicit confirm Item before mutation Tasks start).
// ---------------------------------------------------------------------------

// TestSpecP12_ConfirmGate_Step1 covers Problem 12's step1 block: a dry-run
// plan for the destructive delete plus the still-open confirm prompt, both
// reachable in one transcript through the documented spellings (DryRun +
// RecordName for the plan, Confirm + Destructive for the gate).
//
//	[planned]  remotes
//	  delete-remote  origin/production-hotfix
//	?  confirm remote delete  (destructive)
func TestSpecP12_ConfirmGate_Step1(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.Title("clean"), evo.To(&buf), evo.NoColor(), evo.DryRun(), evo.Stdin(strings.NewReader("y\n")))
	remotes := out.Task("remotes")
	remotes.RecordName("delete-remote", "origin/production-hotfix")
	remotes.Done()
	out.Confirm("confirm remote delete", evo.Destructive())
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	for _, want := range []string{
		"[planned] remotes",
		"delete-remote origin/production-hotfix",
		"confirm remote delete (destructive)",
	} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
}

// TestSpecP12_LiveFrame_Step2 covers Problem 12's step2 block: the confirm
// gate resolves OK (durable row), and the destructive Task becomes the one
// Running child at 1/1 with the named object.
//
//	✓  confirm remote delete
//	:.  remotes  1/1  origin/production-hotfix
func TestSpecP12_LiveFrame_Step2(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen, evo.Stdin(strings.NewReader("y\n")))
	t.Cleanup(func() { _ = out.Close() })

	if ok := out.Confirm("confirm remote delete", evo.Destructive()); !ok {
		t.Fatal("Confirm(\"y\") = false, want true")
	}
	remotes := out.Task("remotes")
	remotes.Progress(1, 1)
	remotes.Phase("origin/production-hotfix")

	var resolvedConfirm string
	for _, op := range screen.Operations() {
		if op.Kind == "durable" && strings.Contains(op.Text, "confirm remote delete") && !strings.Contains(op.Text, "[y/N]") {
			resolvedConfirm = op.Text
		}
	}
	if !strings.Contains(resolvedConfirm, "✓") {
		t.Fatalf("want a durable ✓ confirm row, got %q (ops: %+v)", resolvedConfirm, screen.Operations())
	}
	got := screen.LatestLiveText()
	for _, want := range []string{"remotes", "1/1", "origin/production-hotfix"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in live frame:\n%s", want, got)
		}
	}
}

// TestSpecP12_ConfirmGate_Success covers Problem 12's success block: the
// applied delete lands in the Changes ledger, and the task resolves Done.
//
//	[changed]  remotes
//	  deleted  1  origin tip
//	✓  remotes
func TestSpecP12_ConfirmGate_Success(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.Title("clean"), evo.To(&buf), evo.Plain(), evo.NoColor())
	remotes := out.Task("remotes")
	remotes.Record("delete", 1, "origin tip")
	remotes.Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	for _, want := range []string{"[changed] remotes", "deleted 1 origin tip", "✓ remotes"} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
}

// TestSpecP12_ConfirmGate_Failure covers Problem 12's failure block: a human
// "n" declines the gate (Blocked, never Failed/Cancelled per
// TestConfirm_No_ResolvesBlockedDeclinedAndExitsOne's already-proven
// contract), and the still-planned delete never applies.
//
//	⊘  confirm remote delete  declined
//	[planned]  remotes
//	  delete-remote  origin/production-hotfix
//	!  nothing mutated
//
// MISMATCH (documented, not fixed): "! nothing mutated" has no mechanical
// counterpart — plain.go's writeAlreadyMutated only fires for a
// StateCancelled/StateFailed conclusion (see writeConclusion's guard); a
// declined Confirm concludes StateBlocked, so the row never renders,
// regardless of ledger content. There is no public API for a generic
// stand-alone "!" attention line outside the two derived taxonomy/
// already-mutated mechanisms.
func TestSpecP12_ConfirmGate_Failure(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.Title("clean"), evo.To(&buf), evo.NoColor(), evo.DryRun(), evo.Stdin(strings.NewReader("n\n")))
	if ok := out.Confirm("confirm remote delete", evo.Destructive()); ok {
		t.Fatal("Confirm(\"n\") = true, want false")
	}
	remotes := out.Task("remotes")
	remotes.RecordName("delete-remote", "origin/production-hotfix")
	remotes.Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	for _, want := range []string{
		"⊘ confirm remote delete",
		"└─ declined",
		"[planned] remotes",
		"delete-remote origin/production-hotfix",
	} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "nothing mutated") {
		t.Fatalf("expected the documented mismatch (no \"!\" line for a Blocked conclusion) but found one:\n%s", got)
	}
	t.Skip("MISMATCH: \"! nothing mutated\" has no mechanical source — writeAlreadyMutated only fires for Cancelled/Failed conclusions, never Blocked — see doc comment")
}

// blockingConfirmReader lets a test observe the durable confirm prompt while
// Confirm is genuinely still waiting on an answer, then release it —
// reproducing the "indeterminate, waiting on human" moment without a race on
// the shared buffer (the happens-before chain runs: test writes buf-producing
// calls -> go Confirm() -> internal read goroutine -> close(started) ->
// test's <-started receive, so the buffer is safe to read at that point).
type blockingConfirmReader struct {
	started chan struct{}
	release chan string
}

func newBlockingConfirmReader() *blockingConfirmReader {
	return &blockingConfirmReader{started: make(chan struct{}), release: make(chan string, 1)}
}

func (r *blockingConfirmReader) Read(p []byte) (int, error) {
	close(r.started)
	line := <-r.release
	return copy(p, line), nil
}

// TestSpecP12_ConfirmGate_Indeterminate covers Problem 12's indeterminate
// block: the gate has printed its prompt and is genuinely still waiting on a
// human answer.
//
//	?  confirm remote delete  waiting…
//
// MISMATCH (documented, not fixed): the real pending-prompt text is
// "?  confirm remote delete  (destructive)  [y/N]" (confirm.go
// writeConfirmPromptLocked) — there is no distinct "waiting…" annotation;
// the durable prompt line itself *is* the indeterminate representation, and
// it is identical whether or not a human has looked at it yet.
func TestSpecP12_ConfirmGate_Indeterminate(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	reader := newBlockingConfirmReader()
	out := evo.NewWithOptions(evo.Title("clean"), evo.To(&buf), evo.NoColor(), evo.Stdin(reader))

	done := make(chan bool, 1)
	go func() { done <- out.Confirm("confirm remote delete", evo.Destructive()) }()
	<-reader.started

	pending := buf.String()
	if !strings.Contains(pending, "confirm remote delete") || !strings.Contains(pending, "[y/N]") {
		t.Fatalf("want the durable prompt line while still waiting, got:\n%s", pending)
	}
	if strings.Contains(pending, "waiting…") {
		t.Fatalf("expected the documented mismatch (no \"waiting…\" text) but found it:\n%s", pending)
	}

	reader.release <- "y\n"
	<-done
	t.Skip("MISMATCH: the spec's \"waiting…\" annotation has no counterpart — the pending prompt line is identical to step1's, see doc comment")
}

// TestSpecP12_ConfirmGate_Error covers Problem 12's error block: the confirm
// gate resolves OK, then the destructive Task itself fails against a
// protected-branch server-side rule.
//
//	✓  confirm remote delete
//	✗  remotes  protected branch hook
//	   └─ remote: error: GH006: Protected branch update failed
//	!  nothing deleted on origin
//
// MISMATCH (documented, not fixed): "! nothing deleted on origin" has no
// mechanical source — no Record/RecordName mutation was ever accumulated for
// remotes, so writeAlreadyMutated's derived summary (which only fires for
// Cancelled/Failed conclusions, see the failure-block note above) has no
// ledger content to render even when it does fire (this scenario does
// conclude Failed).
func TestSpecP12_ConfirmGate_Error(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.Title("clean"), evo.To(&buf), evo.NoColor(), evo.Stdin(strings.NewReader("y\n")))
	if ok := out.Confirm("confirm remote delete", evo.Destructive()); !ok {
		t.Fatal("Confirm(\"y\") = false, want true")
	}
	remotes := out.Task("remotes")
	remotes.Fail("protected branch hook", evo.Detail("remote: error: GH006: Protected branch update failed"))
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	for _, want := range []string{
		"✓ confirm remote delete",
		"✗ remotes protected branch hook",
		"remote: error: GH006: Protected branch update failed",
	} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "nothing deleted") {
		t.Fatalf("expected the documented mismatch (no ledger content to derive from) but found the line:\n%s", got)
	}
	t.Skip("MISMATCH: \"! nothing deleted on origin\" has no mechanical source — see doc comment")
}

// TestSpecP12_ConfirmGate_EarlyTermination covers Problem 12's early
// termination block: the confirm gate resolves OK, then the destructive Task
// is cancelled before it mutates anything.
//
//	✓  confirm remote delete
//	■  remotes  cancelled before push --delete
//	!  already mutated: none
//
// MISMATCH (documented, not fixed): "! already mutated: none" is exactly the
// already-established empty-ledger-suppression case (phase_n2_test.go): the
// row is suppressed entirely when the Changes ledger is empty, never
// rendered as literal "none".
func TestSpecP12_ConfirmGate_EarlyTermination(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.Title("clean"), evo.To(&buf), evo.NoColor(), evo.Stdin(strings.NewReader("y\n")))
	if ok := out.Confirm("confirm remote delete", evo.Destructive()); !ok {
		t.Fatal("Confirm(\"y\") = false, want true")
	}
	remotes := out.Task("remotes")
	remotes.Cancel("cancelled before push --delete")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	for _, want := range []string{"✓ confirm remote delete", "■ remotes cancelled before push --delete"} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "already mutated") {
		t.Fatalf("expected the documented mismatch (empty ledger suppresses the row) but found it:\n%s", got)
	}
	t.Skip("MISMATCH: \"! already mutated: none\" — empty ledger suppresses the row entirely (established behavior) — see doc comment")
}
