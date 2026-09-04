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
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("pipeline"), evo.To(&buf), evo.Plain(), evo.NoColor()}})
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
func TestSpecP11_NestedPipeline_Failure(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("pipeline"), evo.To(&buf), evo.Plain(), evo.NoColor()}})
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
		"--- FAIL: TestFoo (0.01s)",
		"foo_test.go:12: want 1, got 0",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
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
func TestSpecP11_NestedPipeline_Error(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("pipeline"), evo.To(&buf), evo.Plain(), evo.NoColor()}})
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
		"stringer: type not found",
		"go test ./...  not started",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
}

// TestSpecP11_NestedPipeline_EarlyTermination covers Problem 11's early
// termination block: the first child resolves Done, the second Cancels, the
// auto-resolution cascade marks the third "not started".
//
//	pipeline
//	✓  go mod download
//	■  go generate  cancelled
//	-  go test ./...  not started
//
// The early-termination "! already mutated: ..." row is derived mechanically
// from the Changes ledger (task_mutations.go / plain.go writeAlreadyMutated)
// and is suppressed entirely when the ledger is empty ("!" is
// attention-only; an empty ledger earns none) — this scenario records no
// Record/RecordName mutation on either child, so the row is absent.
func TestSpecP11_NestedPipeline_EarlyTermination(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("pipeline"), evo.To(&buf), evo.Plain(), evo.NoColor()}})
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
		t.Fatalf("expected the empty-ledger suppression (no already-mutated row) but one was present:\n%s", got)
	}
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
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("clean"), evo.To(&buf), evo.NoColor(), evo.DryRun(), evo.Stdin(strings.NewReader("y\n"))}})
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
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("clean"), evo.To(&buf), evo.Plain(), evo.NoColor()}})
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
//
// A declined Confirm concludes StateBlocked; writeAlreadyMutated only fires
// for a Cancelled/Failed conclusion (plain.go writeConclusion's guard), so no
// "! nothing mutated" row ever renders here — the same "Blocked renders no
// already-mutated row" rule the early-termination cell below documents.
//
// beginner-3 de-echo: the "declined" problem row is dropped since it
// repeats the task's own summary with no Detail beyond it.
func TestSpecP12_ConfirmGate_Failure(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("clean"), evo.To(&buf), evo.NoColor(), evo.DryRun(), evo.Stdin(strings.NewReader("n\n"))}})
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
		"⊘ confirm remote delete declined",
		"[planned] remotes",
		"delete-remote origin/production-hotfix",
	} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "nothing mutated") {
		t.Fatalf("expected no \"!\" line for a Blocked conclusion, but found one:\n%s", got)
	}
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
//	?  confirm remote delete  (destructive)
//
// The real pending-prompt text is "?  confirm remote delete  (destructive)
// [y/N]" (confirm.go writeConfirmPromptLocked) — there is no distinct
// "waiting…" annotation; the durable prompt line itself *is* the
// indeterminate representation, identical to step1's, whether or not a
// human has looked at it yet.
func TestSpecP12_ConfirmGate_Indeterminate(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	reader := newBlockingConfirmReader()
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("clean"), evo.To(&buf), evo.NoColor(), evo.Stdin(reader)}})

	done := make(chan bool, 1)
	go func() { done <- out.Confirm("confirm remote delete", evo.Destructive()) }()
	<-reader.started

	pending := buf.String()
	if !strings.Contains(pending, "confirm remote delete") || !strings.Contains(pending, "[y/N]") {
		t.Fatalf("want the durable prompt line while still waiting, got:\n%s", pending)
	}
	if strings.Contains(pending, "waiting…") {
		t.Fatalf("expected no \"waiting…\" text (the prompt line itself is the indeterminate state) but found it:\n%s", pending)
	}

	reader.release <- "y\n"
	<-done
}

// TestSpecP12_ConfirmGate_Error covers Problem 12's error block: the confirm
// gate resolves OK, then the destructive Task itself fails against a
// protected-branch server-side rule.
//
//	✓  confirm remote delete
//	✗  remotes  protected branch hook
//	   └─ remote: error: GH006: Protected branch update failed
//
// No Record/RecordName mutation was ever accumulated for remotes, so
// writeAlreadyMutated's derived summary — which does fire on this scenario's
// Failed conclusion — has an empty ledger and renders no row at all.
func TestSpecP12_ConfirmGate_Error(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("clean"), evo.To(&buf), evo.NoColor(), evo.Stdin(strings.NewReader("y\n"))}})
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
		t.Fatalf("expected no already-mutated row (empty ledger) but found one:\n%s", got)
	}
}

// TestSpecP12_ConfirmGate_EarlyTermination covers Problem 12's early
// termination block: the confirm gate resolves OK, then the destructive Task
// is cancelled before it mutates anything.
//
//	✓  confirm remote delete
//	■  remotes  cancelled before push --delete
//
// An empty Changes ledger suppresses the "! already mutated: ..." row
// entirely (phase_n2_test.go's established behavior), never rendering the
// literal "none".
func TestSpecP12_ConfirmGate_EarlyTermination(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("clean"), evo.To(&buf), evo.NoColor(), evo.Stdin(strings.NewReader("y\n"))}})
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
		t.Fatalf("expected the empty-ledger suppression (no already-mutated row) but found one:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// Problem 13: retries must set absolute Progress, never Advance/re-Advance,
// so a retry cannot double-count or regress.
// ---------------------------------------------------------------------------

// TestSpecP13_LiveFrame_Step1 covers Problem 13's step1 block: absolute
// count progress with the named current item.
//
//	:.  install  13/40  urllib3
func TestSpecP13_LiveFrame_Step1(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	install := out.Task("install")
	install.Progress(13, 40)
	install.Phase("urllib3")

	got := screen.LatestLiveText()
	for _, want := range []string{"install", "13/40", "urllib3"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in live frame:\n%s", want, got)
		}
	}
}

// TestSpecP13_LiveFrame_Step2 covers Problem 13's step2 block: a retry on
// the same item holds the absolute count steady (still 13/40) while the
// phase names the retry.
//
//	:.  install  13/40  retrying urllib3
//	# still 13/40 — not 12, not 14 until success
func TestSpecP13_LiveFrame_Step2(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	install := out.Task("install")
	install.Progress(13, 40)
	install.Phase("retrying urllib3")

	got := screen.LatestLiveText()
	for _, want := range []string{"install", "13/40", "retrying urllib3"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in live frame:\n%s", want, got)
		}
	}
}

// TestSpecP13_Retry_Success covers Problem 13's success block: the resolved
// Done count plus a skip-taxonomy line whose single-reason format matches
// the spec's own literal text exactly.
//
//	✓  install  40/40
//	!  skipped 2  (optional)
func TestSpecP13_Retry_Success(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("install"), evo.To(&buf), evo.Plain(), evo.NoColor()}})
	install := out.Task("install")
	optional := evo.Reason("optional")
	install.Skipped(optional, "extras")
	install.Skipped(optional, "docs")
	install.Progress(40, 40)
	install.Done("40/40")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{"✓  install  40/40", "!  skipped 2  (optional)"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
}

// TestSpecP13_Retry_Failure covers Problem 13's failure block: the running
// count/phase survives into a Fail with structured Detail evidence.
//
//	:.  install  13/40  urllib3
//	✗  install  13/40  urllib3 failed after 3 tries
//	   └─ HTTP 503 from mirror
//
// Progress counts are not converted into Changes-ledger content the way
// Record/RecordName mutations are, so there is no already-mutated row to
// derive here.
func TestSpecP13_Retry_Failure(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("install"), evo.To(&buf), evo.Plain(), evo.NoColor()}})
	install := out.Task("install")
	install.Progress(13, 40)
	install.Phase("urllib3")
	install.Fail("urllib3 failed after 3 tries", evo.Detail("HTTP 503 from mirror"))
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	for _, want := range []string{
		"✗ install 13/40 urllib3 failed after 3 tries",
		"HTTP 503 from mirror",
	} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "remaining 27 not attempted") {
		t.Fatalf("expected no already-mutated row (Progress isn't Changes-ledger content) but found one:\n%s", got)
	}
}

// TestSpecP13_LiveFrame_Indeterminate covers Problem 13's indeterminate
// block: a retry announced before any count is known for this attempt.
//
//	:.  install  retrying urllib3…
func TestSpecP13_LiveFrame_Indeterminate(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	out.Task("install").Phase("retrying urllib3…")

	got := screen.LatestLiveText()
	if !strings.Contains(got, "install") || !strings.Contains(got, "retrying urllib3…") {
		t.Fatalf("want indeterminate retry phase in live frame:\n%s", got)
	}
}

// TestSpecP13_Retry_Error covers Problem 13's error block, spelled the way
// the adjacent "Progress invariants" section documents this exact scenario
// (ErrProgressRegression on a backwards Advance/retry double-count): the
// caller checks Output.Err() after a rejected regression and Fails the task
// with the true held count, reachable byte-for-byte through the public API.
//
//	:.  install  13/40  urllib3
//	✗  install  13/40  progress misuse avoided — absolute 13/40 held
//	   └─ connection reset by peer
func TestSpecP13_Retry_Error(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("install"), evo.To(&buf), evo.Plain(), evo.NoColor()}})
	install := out.Task("install")
	install.Progress(13, 40)
	install.Phase("urllib3")
	install.Progress(12, 40) // a backwards retry report: rejected, absolute 13/40 held
	if out.Err() == nil {
		t.Fatal("want recorded misuse when Progress regresses")
	}
	install.Fail("progress misuse avoided — absolute 13/40 held", evo.Detail("connection reset by peer"))
	// Finish surfaces the already-recorded, already-handled misuse (the rejected
	// regression above) as its return value — that is the point of this
	// scenario, not a new failure to fatal on.
	if err := out.Finish(); err != nil && err != out.Err() {
		t.Fatal(err)
	}
	got := buf.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	for _, want := range []string{
		"✗ install 13/40 progress misuse avoided — absolute 13/40 held",
		"connection reset by peer",
	} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
}

// TestSpecP13_Retry_EarlyTermination covers Problem 13's early termination
// block: a Cancel mid-retry, with the completed-so-far count recorded as a
// real mutation so "already mutated" has ledger content to derive from.
//
//	:.  install  13/40
//	■  install  cancelled during retry
//	!  already mutated: 13 packages installed
//
// summarizeChangeSection (plain.go) derives exactly "13 packages installed"
// from a Record("install", 13, "packages") call; the mechanism has no way to
// append a second clause naming which specific item was mid-flight when
// cancelled.
func TestSpecP13_Retry_EarlyTermination(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("install"), evo.To(&buf), evo.Plain(), evo.NoColor()}})
	install := out.Task("install")
	install.Progress(13, 40)
	install.Record("install", 13, "package")
	install.Cancel("cancelled during retry")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	for _, want := range []string{
		"■ install cancelled during retry",
		"already mutated: 13 packages installed",
	} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "urllib3 absent") {
		t.Fatalf("expected no second-clause narration (no mechanical source) but found it:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// Problem 14: capture streams may include secrets; Fail's DetailTail must
// never paste them into the durable ledger.
// ---------------------------------------------------------------------------

// TestSpecP14_LiveFrame_Step1 covers Problem 14's step1 block: an
// indeterminate phase while a scan/salvage capture is still classifying.
//
//	:.  capture  packing tips…
func TestSpecP14_LiveFrame_Step1(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	out.Task("capture").Phase("packing tips…")

	got := screen.LatestLiveText()
	if !strings.Contains(got, "capture") || !strings.Contains(got, "packing tips…") {
		t.Fatalf("want indeterminate packing phase in live frame:\n%s", got)
	}
}

// TestSpecP14_LiveFrame_Step2 covers Problem 14's step2 block: sealed count
// progress with the current branch name.
//
//	:.  capture  2/5  feat/secret-work
func TestSpecP14_LiveFrame_Step2(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	capture := out.Task("capture")
	capture.Progress(2, 5)
	capture.Phase("feat/secret-work")

	got := screen.LatestLiveText()
	for _, want := range []string{"capture", "2/5", "feat/secret-work"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in live frame:\n%s", want, got)
		}
	}
}

// TestSpecP14_Capture_Success covers Problem 14's success block: a resolved
// capture task, a dry-run "salvage" plan section, and a skip-taxonomy line.
//
//	✓  capture
//	[planned]  capture
//	  salvage  2  tip
//	!  skipped 3  (has-pr)
func TestSpecP14_Capture_Success(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("capture"), evo.To(&buf), evo.Plain(), evo.NoColor(), evo.DryRun()}})
	capture := out.Task("capture")
	hasPR := evo.Reason("has-pr")
	capture.Record("salvage", 2, "tip")
	capture.Skipped(hasPR, "feat/a")
	capture.Skipped(hasPR, "feat/b")
	capture.Skipped(hasPR, "feat/c")
	capture.Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	for _, want := range []string{
		"✓ capture",
		"[planned] capture",
		"salvage 2 tip",
	} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
	if !strings.Contains(collapsed, "skipped 3 (has-pr)") {
		t.Fatalf("want the real derived taxonomy line \"skipped 3 (has-pr)\", got:\n%s", got)
	}
	if strings.Contains(got, "skip-has-pr") {
		t.Fatalf("expected no fused verb-reason spelling but found it:\n%s", got)
	}
}

// bearerTokenRedactor redacts a "Bearer <token>" credential to "Bearer ***",
// following the same shape as platform_test.go's secretRedactor.
type bearerTokenRedactor struct{}

func (bearerTokenRedactor) RedactString(s string) string {
	const marker = "Bearer "
	i := strings.Index(s, marker)
	if i < 0 {
		return s
	}
	rest := s[i+len(marker):]
	end := strings.IndexAny(rest, " \n")
	if end < 0 {
		end = len(rest)
	}
	return s[:i] + marker + "***" + rest[end:]
}

// TestSpecP14_Capture_Failure covers Problem 14's failure block: a captured
// child-process line carrying a bearer token is redacted before it ever
// reaches Fail's Detail evidence — the documented spelling (Task.Capture +
// Redact + DetailTail), same shape as platform_test.go's already-proven
// TestCapture_RedactsOnRetention.
//
//	✗  capture  git push failed
//	   └─ remote rejected (see redacted stderr)
//	      Authorization: Bearer ***
func TestSpecP14_Capture_Failure(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Title:    "capture",
		Stdout:   &buf,
		Stderr:   &buf,
		Redactor: bearerTokenRedactor{},
		Color:    evo.ColorNever,
	})
	capture := out.Task("capture")
	cap := capture.Evidence()
	_, _ = cap.Write([]byte("remote rejected (see redacted stderr)\nAuthorization: Bearer sk-live-abc123\n"))
	_ = cap.Close()
	capture.Fail("git push failed", cap.DetailTail())
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if strings.Contains(got, "sk-live-abc123") {
		t.Fatalf("raw secret leaked into presentation:\n%s", got)
	}
	for _, want := range []string{
		"✗  capture  git push failed",
		"remote rejected (see redacted stderr)",
		"Authorization: Bearer ***",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
}

// TestSpecP14_LiveFrame_Indeterminate covers Problem 14's indeterminate
// block: a distinct phase text while scanning for local-only branches.
//
//	:.  capture  scanning local-only…
func TestSpecP14_LiveFrame_Indeterminate(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	out.Task("capture").Phase("scanning local-only…")

	got := screen.LatestLiveText()
	if !strings.Contains(got, "capture") || !strings.Contains(got, "scanning local-only…") {
		t.Fatalf("want indeterminate scanning phase in live frame:\n%s", got)
	}
}

// TestSpecP14_Capture_Error covers Problem 14's error block: the capture
// mechanism itself detects a leaking credential helper and fails safely
// without ever pushing.
//
//	✗  capture  credential helper printed a secret
//	   └─ stderr redacted (1 line held)
//
// No Record/RecordName mutation exists for capture in this scenario, so no
// already-mutated row renders.
func TestSpecP14_Capture_Error(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("capture"), evo.To(&buf), evo.Plain(), evo.NoColor()}})
	capture := out.Task("capture")
	capture.Fail("credential helper printed a secret", evo.Detail("stderr redacted (1 line held)"))
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	for _, want := range []string{
		"✗ capture credential helper printed a secret",
		"stderr redacted (1 line held)",
	} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "nothing pushed") {
		t.Fatalf("expected no already-mutated row (empty ledger) but found one:\n%s", got)
	}
}

// TestSpecP14_Capture_EarlyTermination covers Problem 14's early termination
// block: a Cancel mid-capture with the running count/phase preserved on the
// live frame just before it.
//
//	:.  capture  1/5
//	■  capture  cancelled
//
// An empty Changes ledger suppresses the "! already mutated: ..." row
// entirely (phase_n2_test.go's established behavior).
func TestSpecP14_Capture_EarlyTermination(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	capture := out.Task("capture")
	capture.Progress(1, 5)
	before := screen.LatestLiveText()
	if !strings.Contains(before, "capture") || !strings.Contains(before, "1/5") {
		t.Fatalf("want running 1/5 in live frame:\n%s", before)
	}

	capture.Cancel("cancelled")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	// Cancel commits the resolved row durably at resolution time
	// (release-gate round 5 finding 3, commitResolvedTaskLocked) rather than
	// waiting for WriteFinal — PersistedText covers both durable and final.
	persisted := screen.PersistedText()
	if !strings.Contains(persisted, "capture") || !strings.Contains(persisted, "cancelled") {
		t.Fatalf("want cancelled capture in persisted text:\n%s", persisted)
	}
	if strings.Contains(persisted, "already mutated") {
		t.Fatalf("expected the empty-ledger suppression (no already-mutated row) but found one:\n%s", persisted)
	}
}

// ---------------------------------------------------------------------------
// Problem 15: nothing-to-do paths must not invent fake progress or a
// warning; the generic empty-success default is a quiet Item OK plus one
// plain line (already-proven success block: TestSpecP15_NothingToDo_Success
// in spec_golden_test.go).
// ---------------------------------------------------------------------------

// TestSpecP15_NothingToDo_Step1 covers Problem 15's step1 block — textually
// identical to the already-proven success block, driven the same documented
// way (Item(...).Done() + Println), covering this problem's step1 cell in its
// own right.
//
//	✓  clean
//	nothing to clean
func TestSpecP15_NothingToDo_Step1(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("clean"), evo.To(&buf), evo.Plain(), evo.NoColor()}})
	out.Task("clean").Done()
	out.Println("nothing to clean")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{"✓  clean", "nothing to clean"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
}

// TestSpecP15_NothingToDo_Step2 covers Problem 15's step2 block: the same
// quiet-OK-plus-one-line default applied to a second named condition.
//
//	✓  capture plan
//	nothing to capture (tips already on remote or no long-tail work)
func TestSpecP15_NothingToDo_Step2(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("capture"), evo.To(&buf), evo.Plain(), evo.NoColor()}})
	out.Task("capture plan").Done()
	out.Println("nothing to capture (tips already on remote or no long-tail work)")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{"✓  capture plan", "nothing to capture (tips already on remote or no long-tail work)"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
}

// TestSpecP15_NothingToDo_Failure covers Problem 15's failure block, which
// the spec itself annotates "# not applicable — empty is success" and then
// repeats the identical success text — there is no distinct failure
// behavior for this default; this test proves the identical spelling still
// renders identically, confirming the spec's own annotation rather than
// exercising new behavior.
//
//	# not applicable — empty is success
//	✓  clean
//	nothing to clean
func TestSpecP15_NothingToDo_Failure(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("clean"), evo.To(&buf), evo.Plain(), evo.NoColor()}})
	out.Task("clean").Done()
	out.Println("nothing to clean")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{"✓  clean", "nothing to clean"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
	if strings.ContainsAny(got, "✗■⊘") {
		t.Fatalf("empty-success path must never render a failure/blocked/cancelled glyph, got:\n%s", got)
	}
}

// TestSpecP15_LiveFrame_Indeterminate covers Problem 15's indeterminate
// block: still classifying whether there is anything to do.
//
//	:.  clean  classifying…
func TestSpecP15_LiveFrame_Indeterminate(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	out.Task("clean").Phase("classifying…")

	got := screen.LatestLiveText()
	if !strings.Contains(got, "clean") || !strings.Contains(got, "classifying…") {
		t.Fatalf("want indeterminate classifying phase in live frame:\n%s", got)
	}
}

// TestSpecP15_NothingToDo_Error covers Problem 15's error block: a repository
// read failure means the classification itself could not run, spelled as a
// Task Fail with structured Detail evidence.
//
//	✗  clean  cannot read repository
//	   └─ fatal: not a git repository
func TestSpecP15_NothingToDo_Error(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("clean"), evo.To(&buf), evo.Plain(), evo.NoColor()}})
	clean := out.Task("clean")
	clean.Fail("cannot read repository", evo.Detail("fatal: not a git repository"))
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	for _, want := range []string{
		"✗ clean cannot read repository",
		"fatal: not a git repository",
	} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
}

// TestSpecP15_NothingToDo_EarlyTermination covers Problem 15's early
// termination block: classification is cancelled mid-flight, before any
// conclusion is reached about whether there is anything to do.
//
//	:.  clean  classifying…
//	■  clean  cancelled
//
// No classification work ever records a mutation, so the empty-ledger
// suppression (phase_n2_test.go's established behavior) leaves no
// already-mutated row.
func TestSpecP15_NothingToDo_EarlyTermination(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	clean := out.Task("clean")
	clean.Phase("classifying…")
	before := screen.LatestLiveText()
	if !strings.Contains(before, "clean") || !strings.Contains(before, "classifying…") {
		t.Fatalf("want classifying phase in live frame:\n%s", before)
	}

	clean.Cancel("cancelled")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	// Cancel commits the resolved row durably at resolution time
	// (release-gate round 5 finding 3, commitResolvedTaskLocked) rather than
	// waiting for WriteFinal — PersistedText covers both durable and final.
	persisted := screen.PersistedText()
	if !strings.Contains(persisted, "clean") || !strings.Contains(persisted, "cancelled") {
		t.Fatalf("want cancelled clean in persisted text:\n%s", persisted)
	}
	if strings.Contains(persisted, "already mutated") {
		t.Fatalf("expected the empty-ledger suppression (no already-mutated row) but found one:\n%s", persisted)
	}
}
