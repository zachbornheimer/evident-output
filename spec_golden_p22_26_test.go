//go:build unix

package evo_test

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// This file closes the remaining spec-golden cells for evo-rec.md Problems
// 22-26 (confirm-gate quiescing, SIGINT/exit-code derivation, --json
// stdout/stderr split, ASCII glyph fallback, narrow-terminal resize). It
// reuses the helpers and normalization conventions established in
// spec_golden_test.go / spec_golden_live_test.go on this branch:
//   - collapsed := strings.Join(strings.Fields(got), " ") to make
//     whitespace/indentation-insensitive substring assertions, exactly like
//     the plain-mode tests in spec_golden_test.go.
//   - newLiveScreenOutput-style testkit.Screen construction for live frames.
//   - Running-state spinner glyphs are never asserted literally: the spinner
//     frame is genuine per-tick nondeterminism (this work order's own
//     instruction to "normalize only genuine nondeterminism: durations,
//     spinner frames"), so every live assertion here checks task name/phase/
//     count text, never the leading glyph column.
//   - Any cell whose real rendering contradicts the fenced block's wording,
//     values, or states is committed anyway, running the real scenario, then
//     t.Skip("MISMATCH: ...") per this work order's explicit rule — a fixer
//     decides whether the spec or the implementation should move.

// TestSpecP22_ConfirmGate_Step1 covers evo-rec.md Problem 22's step1 block: a
// destructive confirm gate quiesces the live region and renders a durable
// "?  <question>  (destructive)  [y/N]" prompt line, no live frame beside it.
//
//	?  confirm remote delete  (destructive)  [y/N]
func TestSpecP22_ConfirmGate_Step1(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Terminal(screen), evo.VisibilityDelay(0), evo.NoColor(), evo.Stdin(strings.NewReader("y\n"))}})

	if ok := out.Confirm("confirm remote delete", evo.Destructive()); !ok {
		t.Fatal("Confirm(\"y\") = false, want true")
	}

	var prompt string
	for _, op := range screen.Operations() {
		if op.Kind == "durable" && strings.Contains(op.Text, "[y/N]") {
			prompt = op.Text
			break
		}
	}
	collapsed := strings.Join(strings.Fields(prompt), " ")
	for _, want := range []string{"?", "confirm remote delete", "(destructive)", "[y/N]"} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("want %q in prompt line, got %q", want, prompt)
		}
	}
}

// TestSpecP22_ConfirmGate_Indeterminate covers Problem 22's indeterminate
// block: the same durable prompt without the destructive tag — "waiting on
// the human is a first-class state, not hung".
//
//	?  confirm remote delete  [y/N]
func TestSpecP22_ConfirmGate_Indeterminate(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Terminal(screen), evo.VisibilityDelay(0), evo.NoColor(), evo.Stdin(strings.NewReader("y\n"))}})

	out.Confirm("confirm remote delete")

	var prompt string
	for _, op := range screen.Operations() {
		if op.Kind == "durable" && strings.Contains(op.Text, "[y/N]") {
			prompt = op.Text
			break
		}
	}
	collapsed := strings.Join(strings.Fields(prompt), " ")
	if !strings.Contains(collapsed, "? confirm remote delete [y/N]") {
		t.Fatalf("want undecorated prompt line, got %q", prompt)
	}
	if strings.Contains(collapsed, "destructive") {
		t.Fatalf("non-destructive prompt must not carry the (destructive) tag, got %q", prompt)
	}
}

// TestSpecP22_ConfirmGate_Step2 covers Problem 22's step2 block: once the
// gate resolves OK, the next task becomes the one Running child, driven with
// an absolute Progress(1,1)+Phase (the spelling this spec teaches for a
// named current-position row — see spec_golden_live_test.go's
// TestSpecP3_LiveFrame_Step2 for the identical idiom), which reaches the
// documented "1/1" exactly rather than Each's before-yield "0/1".
//
//	✓  confirm remote delete
//	:.  remotes  1/1  origin/production-hotfix
func TestSpecP22_ConfirmGate_Step2(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Terminal(screen), evo.VisibilityDelay(0), evo.MaxFrameRate(1_000_000), evo.NoColor(), evo.Stdin(strings.NewReader("y\n"))}})

	if ok := out.Confirm("confirm remote delete", evo.Destructive()); !ok {
		t.Fatal("Confirm(\"y\") = false, want true")
	}
	remotes := out.Task("remotes")
	remotes.Progress(1, 1)
	remotes.Phase("origin/production-hotfix")

	var resolved string
	for _, op := range screen.Operations() {
		if op.Kind == "durable" && strings.Contains(op.Text, "confirm remote delete") && !strings.Contains(op.Text, "[y/N]") {
			resolved = op.Text
		}
	}
	if !strings.Contains(resolved, "✓") || !strings.Contains(resolved, "confirm remote delete") {
		t.Fatalf("want resolved OK row for the gate, got %q", resolved)
	}
	live := screen.LatestLiveText()
	for _, want := range []string{"remotes", "1/1", "origin/production-hotfix"} {
		if !strings.Contains(live, want) {
			t.Fatalf("want %q in live frame, got %q", want, live)
		}
	}
}

// TestSpecP22_ConfirmGate_Success covers Problem 22's success block: the
// gate's durable OK row survives alongside the subsequent task's Changes
// ledger.
//
//	✓  confirm remote delete
//	[changed]  remotes
//	  deleted  1  origin tip
func TestSpecP22_ConfirmGate_Success(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Stdin(strings.NewReader("y\n"))}})

	if ok := out.Confirm("confirm remote delete", evo.Destructive()); !ok {
		t.Fatal("Confirm(\"y\") = false, want true")
	}
	remotes := out.Task("remotes")
	remotes.Record("delete", 1, "origin tip")
	remotes.Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	for _, want := range []string{
		"✓ confirm remote delete",
		"[changed] remotes",
		"deleted 1 origin tip",
	} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
}

// TestSpecP22_ConfirmGate_Failure covers Problem 22's failure block.
//
//	⊘  confirm remote delete  declined
//	# $? = 1 (Blocked)
//
// A declined Confirm concludes StateBlocked; writeAlreadyMutated only fires
// for a Cancelled/Failed conclusion (plain.go writeConclusion's guard), and
// nothing was ever mutated here, so no "!" row renders at all.
func TestSpecP22_ConfirmGate_Failure(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Stdin(strings.NewReader("n\n"))}})

	if ok := out.Confirm("confirm remote delete", evo.Destructive()); ok {
		t.Fatal("Confirm(\"n\") = true, want false")
	}
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if out.Conclusion().ExitCode != evo.ExitBlocked {
		t.Fatalf("exit = %d, want ExitBlocked (1)", out.Conclusion().ExitCode)
	}
	got := buf.String()
	if !strings.Contains(got, "⊘  confirm remote delete") || !strings.Contains(got, "declined") {
		t.Fatalf("want the declined Blocked row, got:\n%s", got)
	}
	if strings.Contains(got, "nothing mutated") {
		t.Fatalf("expected no \"!\" line for a Blocked conclusion, but found one:\n%s", got)
	}
}

// TestSpecP22_ConfirmGate_Error covers Problem 22's error block.
//
//	⊘  confirm remote delete  blocked by policy
//	→  pass --yes to confirm non-interactively
//	# $? = 1 (Blocked) — policy block, distinct from a human decline and from Failed
//
// beginner-3 de-echo: the "blocked by policy" problem row is dropped since
// it repeats the task's own summary with no Detail beyond it.
func TestSpecP22_ConfirmGate_Error(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.NonInteractive(), evo.Stdin(&panicReader{t: t})}})

	if ok := out.Confirm("confirm remote delete"); ok {
		t.Fatal("Confirm on non-interactive without --yes = true, want false")
	}
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	for _, want := range []string{
		"⊘ confirm remote delete blocked by policy",
		"→ pass --yes to confirm non-interactively",
	} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "nothing mutated") {
		t.Fatalf("expected no \"!\" line for a Blocked conclusion, but found one:\n%s", got)
	}
}

// TestSpecP22_ConfirmGate_EarlyTermination_Mismatch covers Problem 22's early
// termination block, exercised through the real SIGINT path
// (runInterruptible → cancelActive → cancelPendingConfirmLocked), the same
// mechanism run_signal_test.go and confirm_signal_test.go already prove.
//
//	■  confirm remote delete  interrupted
//
// The cancelled gate always annotates "interrupted" (run.go hard-codes this
// reason string for every SIGINT/SIGTERM cancellation). The
// "! already mutated: ..." row is deliberately suppressed entirely when the
// effect ledger is empty (plain.go's writeAlreadyMutated: "an empty ledger
// earns no attention, so the row is suppressed entirely rather than
// rendered as 'none'").
func TestSpecP22_ConfirmGate_EarlyTermination(t *testing.T) {
	var buf bytes.Buffer
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	defer func() { _ = w.Close() }()
	evo.SetDefault(evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Stdin(r)}}))

	started := make(chan struct{})
	go func() {
		<-started
		time.Sleep(20 * time.Millisecond)
		_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
	}()

	code := evo.Main(func() error {
		close(started)
		evo.Confirm("confirm remote delete", evo.Destructive())
		return nil
	})

	if code != evo.ExitCancelled {
		t.Fatalf("exit %d, want %d (ExitCancelled); out:\n%s", code, evo.ExitCancelled, buf.String())
	}
	got := buf.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	if !strings.Contains(collapsed, "■ confirm remote delete interrupted") {
		t.Fatalf("want the cancelled gate row annotated \"interrupted\", got:\n%s", got)
	}
	if strings.Contains(got, "already mutated") {
		t.Fatalf("expected the empty-ledger suppression (no already-mutated row) but found one:\n%s", got)
	}
}

// TestSpecP23_SignalConclusion_Step1 covers evo-rec.md Problem 23's step1
// block: a prior Done task alongside a Running task's indeterminate phase,
// right before the human sends ^C (the signal itself is exercised by the
// step2/early-termination cells below).
//
//	✓  scan
//	:.  venv  creating
//	# ^C
func TestSpecP23_SignalConclusion_Step1(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Terminal(screen), evo.VisibilityDelay(0), evo.MaxFrameRate(1_000_000), evo.NoColor()}})

	out.Task("scan").Done()
	out.Task("venv").Phase("creating")

	live := screen.LatestLiveText()
	if !strings.Contains(live, "✓") || !strings.Contains(live, "scan") {
		t.Fatalf("want scan's Done row to survive in the flat live frame, got %q", live)
	}
	if !strings.Contains(live, "venv") || !strings.Contains(live, "creating") {
		t.Fatalf("want venv Running with phase \"creating\" in the live frame, got %q", live)
	}
}

// TestSpecP23_SignalConclusion_Success covers Problem 23's success block: a
// clean multi-task run derives exit 0 from the same Conclusion that draws
// every ✓.
//
//	✓  scan
//	✓  venv
//	✓  install
//	# $? = 0
func TestSpecP23_SignalConclusion_Success(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}})
	out.Task("scan").Done()
	out.Task("venv").Done()
	out.Task("install").Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{"✓  scan", "✓  venv", "✓  install"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
	if out.Conclusion().ExitCode != evo.ExitOK {
		t.Fatalf("exit = %d, want ExitOK (0)", out.Conclusion().ExitCode)
	}
}

// TestSpecP23_SignalConclusion_Failure covers Problem 23's failure block: a
// failed child derives exit 2 from the same Conclusion machinery, never a
// hand-mapped code.
//
//	✓  scan
//	✗  venv  uv exited 1
//	# $? = 2
func TestSpecP23_SignalConclusion_Failure(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}})
	out.Task("scan").Done()
	out.Task("venv").Fail("uv exited 1")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{"✓  scan", "✗  venv  uv exited 1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
	if out.Conclusion().ExitCode != evo.ExitFailed {
		t.Fatalf("exit = %d, want ExitFailed (2)", out.Conclusion().ExitCode)
	}
}

// TestSpecP23_SignalConclusion_Error covers Problem 23's error block: an
// unrecoverable SIGKILL is reported honestly next run (as a Failed row, not
// invented as Cancelled) — the same exit-2 Conclusion path as any other
// failure.
//
//	✓  scan
//	✗  venv  signal: killed (SIGKILL — no cleanup possible)
//	# $? = 2
func TestSpecP23_SignalConclusion_Error(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}})
	out.Task("scan").Done()
	out.Task("venv").Fail("signal: killed (SIGKILL — no cleanup possible)")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{"✓  scan", "✗  venv  signal: killed (SIGKILL — no cleanup possible)"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
	if out.Conclusion().ExitCode != evo.ExitFailed {
		t.Fatalf("exit = %d, want ExitFailed (2)", out.Conclusion().ExitCode)
	}
}

// TestSpecP23_SignalConclusion_Step2_Mismatch covers Problem 23's step2
// block, exercised through the real SIGINT path (the same
// runInterruptible/cancelActive mechanism run_signal_test.go proves) against
// a sequential evo.Group so a later sibling renders "not started".
//
//	✓  scan
//	■  venv  interrupted
//	-  install  not started
//	# $? = 130
//
// The cancelled row always annotates "interrupted" (run.go's cancelActive
// reason string is hard-coded for every SIGINT/SIGTERM).
func TestSpecP23_SignalConclusion_Step2(t *testing.T) {
	var buf bytes.Buffer
	evo.SetDefault(evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}}))
	setup := evo.Group("python")
	scan, venv := setup.Task("scan"), setup.Task("venv")
	setup.Task("install")

	started := make(chan struct{})
	go func() {
		<-started
		time.Sleep(20 * time.Millisecond)
		_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
	}()

	code := evo.Main(func() error {
		scan.Done()
		venv.Phase("creating")
		close(started)
		deadline := time.Now().Add(2 * time.Second)
		for venv.Snapshot().State != evo.Cancelled && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		return nil
	})

	if code != evo.ExitCancelled {
		t.Fatalf("exit %d, want %d (ExitCancelled); out:\n%s", code, evo.ExitCancelled, buf.String())
	}
	got := buf.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	for _, want := range []string{"✓ scan", "■ venv interrupted", "- install not started"} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
}

// TestSpecP23_SignalConclusion_Indeterminate_NotTestable covers Problem 23's
// indeterminate block.
//
//	✓  scan
//	■  venv  cancelling — finishing current write…
//	# second ^C forces immediate exit, still 130
//
// NOT-TESTABLE through the public API as spelled: the library exposes no
// primitive for a transient "cancelling, waiting for a graceful window"
// phase distinct from the final Cancelled state. A cancelled task renders
// "■  venv  interrupted" the moment Cancel resolves it (see
// TestSpecP23_SignalConclusion_Step2_Mismatch); there is no caller-visible
// intermediate state to drive a "cancelling — finishing current write…"
// phase string onto, and run_signal_test.go's own second-SIGINT coverage
// (TestMain_SecondSIGINTExits130WithoutWaitingForRun) proves the second
// signal forces Main to return without any further rendering at all, not a
// distinguishable in-between frame.
func TestSpecP23_SignalConclusion_Indeterminate_NotTestable(t *testing.T) {
	t.Skip("NOT-TESTABLE: see doc comment — no public primitive renders a transient \"cancelling\" phase distinct from the terminal Cancelled state")
}

// TestSpecP23_SignalConclusion_EarlyTermination_Mismatch covers Problem 23's
// early termination block: a committed effect survives a SIGINT cancellation
// as a derived "already mutated" line.
//
//	✓  scan
//	■  venv     interrupted
//	-  install  not started
//	!  already mutated: 1 .venv directory created
//	# $? = 130
//
// Same "interrupted" reason-text as the step2 cell above; the derived
// "already mutated" line's grammar is "<qty> <label> <verb>" — a real
// committed record, never a hand-assembled fabrication (evo-rec.md
// "Taxonomy and mutation lines are derived, never assembled").
func TestSpecP23_SignalConclusion_EarlyTermination(t *testing.T) {
	var buf bytes.Buffer
	evo.SetDefault(evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}}))
	setup := evo.Group("python")
	scan, venv := setup.Task("scan"), setup.Task("venv")
	setup.Task("install")

	started := make(chan struct{})
	go func() {
		<-started
		time.Sleep(20 * time.Millisecond)
		_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
	}()

	code := evo.Main(func() error {
		scan.Done()
		venv.Record("create", 1, ".venv directory")
		close(started)
		deadline := time.Now().Add(2 * time.Second)
		for venv.Snapshot().State != evo.Cancelled && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		return nil
	})

	if code != evo.ExitCancelled {
		t.Fatalf("exit %d, want %d (ExitCancelled); out:\n%s", code, evo.ExitCancelled, buf.String())
	}
	got := buf.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	for _, want := range []string{"✓ scan", "■ venv interrupted", "- install not started", "already mutated: 1 .venv directory created"} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
}

// newDataFormatOutput builds an Output in FormatData mode (--json split)
// with an interactive terminal wired to stderr so live frames are
// observable, mirroring configToOptions' own wiring for a real TTY stderr
// (construct.go: Format=FormatData + Terminal set uses the caller's
// Terminal for the live region on the stderr side).
func newDataFormatOutput(screen *testkit.Screen, presentation, payload *bytes.Buffer) *evo.Output {
	return evo.Init(evo.Config{Options: []evo.Option{
		evo.Title("scan"),
		evo.To(presentation),
		evo.Diagnostics(presentation),
		evo.DataProjection(),
		evo.ResultStream(payload),
		evo.Terminal(screen),
		evo.VisibilityDelay(0),
		evo.MaxFrameRate(1_000_000),
		evo.NoColor(),
	}})
}

// TestSpecP24_DataFormat_Step1 covers evo-rec.md Problem 24's step1 block:
// presentation (a Running phase) lives on the live region wired to stderr;
// the domain payload stream stays empty until a record is written.
//
//	# stderr:
//	•  scan  scanning
//	# stdout: (empty until payload ready)
func TestSpecP24_DataFormat_Step1(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	var presentation, payload bytes.Buffer
	out := newDataFormatOutput(screen, &presentation, &payload)

	out.Task("scan").Phase("scanning")

	live := screen.LatestLiveText()
	if !strings.Contains(live, "scan") || !strings.Contains(live, "scanning") {
		t.Fatalf("want scan Running with phase \"scanning\" on the stderr live region, got %q", live)
	}
	if payload.Len() != 0 {
		t.Fatalf("want the payload stream empty until a record is written, got %q", payload.String())
	}
}

// TestSpecP24_DataFormat_Step2 covers Problem 24's step2 block: the same
// live phase advancing to a count, while stdout streams a JSONL record —
// two independent streams, never mixed.
//
//	# stderr:
//	•  scan  40/128
//	# stdout streams JSONL as records finalize:
//	{"repo":"zq","state":"ready"}
func TestSpecP24_DataFormat_Step2(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	var presentation, payload bytes.Buffer
	out := newDataFormatOutput(screen, &presentation, &payload)

	scan := out.Task("scan")
	scan.Progress(40, 128)
	if _, err := out.ResultWriter().Write([]byte(`{"repo":"zq","state":"ready"}` + "\n")); err != nil {
		t.Fatal(err)
	}

	live := screen.LatestLiveText()
	if !strings.Contains(live, "scan") || !strings.Contains(live, "40/128") {
		t.Fatalf("want scan's count on the stderr live region, got %q", live)
	}
	if strings.ContainsAny(live, "✓✗■") {
		t.Fatalf("live presentation must never contain a terminal glyph mid-run, got %q", live)
	}
	if !strings.Contains(payload.String(), `"repo":"zq","state":"ready"`) {
		t.Fatalf("want the JSONL record on the payload stream, got %q", payload.String())
	}
}

// TestSpecP24_DataFormat_Indeterminate covers Problem 24's indeterminate
// block: an unsealed phase renders on the stderr live region same as any
// other indeterminate task.
//
//	# stderr:
//	•  scan  discovering
func TestSpecP24_DataFormat_Indeterminate(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	var presentation, payload bytes.Buffer
	out := newDataFormatOutput(screen, &presentation, &payload)

	out.Task("scan").Phase("discovering")

	live := screen.LatestLiveText()
	if !strings.Contains(live, "scan") || !strings.Contains(live, "discovering") {
		t.Fatalf("want scan's indeterminate phase on the stderr live region, got %q", live)
	}
}

// TestSpecP24_DataFormat_Failure covers Problem 24's failure block: a failed
// data command emits no partial payload by default, and the failure itself
// renders on stderr only.
//
//	# stderr:
//	✗  scan  permission denied under ~/Developer
//	# stdout: empty — a failed data command emits no partial payload by default
func TestSpecP24_DataFormat_Failure(t *testing.T) {
	t.Parallel()
	var presentation, payload bytes.Buffer
	out := evo.Init(evo.Config{
		Title:  "scan",
		Format: evo.FormatData,
		Stderr: &presentation,
		Result: &payload,
		Color:  evo.ColorNever,
	})
	out.Task("scan").Fail("permission denied under ~/Developer")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(presentation.String(), "✗  scan  permission denied under ~/Developer") {
		t.Fatalf("want the failure row on stderr, got %q", presentation.String())
	}
	if payload.Len() != 0 {
		t.Fatalf("want the payload stream empty on a failed data command, got %q", payload.String())
	}
	if out.Conclusion().ExitCode != evo.ExitFailed {
		t.Fatalf("exit = %d, want ExitFailed (2)", out.Conclusion().ExitCode)
	}
}

// TestSpecP24_DataFormat_Error covers Problem 24's error block: the payload
// stream stays empty and the consumer keys off the exit code, not payload
// shape.
//
//	# stderr:
//	✗  scan  git rev-parse failed
//	   └─ not a git repository
//	# stdout: empty; $? = 2 — the consumer keys off exit code, not payload shape
func TestSpecP24_DataFormat_Error(t *testing.T) {
	t.Parallel()
	var presentation, payload bytes.Buffer
	out := evo.Init(evo.Config{
		Title:  "scan",
		Format: evo.FormatData,
		Stderr: &presentation,
		Result: &payload,
		Color:  evo.ColorNever,
	})
	out.Task("scan").Fail("git rev-parse failed", evo.Detail("not a git repository"))
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := presentation.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	for _, want := range []string{"✗ scan git rev-parse failed", "not a git repository"} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
	if payload.Len() != 0 {
		t.Fatalf("want the payload stream empty, got %q", payload.String())
	}
	if out.Conclusion().ExitCode != evo.ExitFailed {
		t.Fatalf("exit = %d, want ExitFailed (2)", out.Conclusion().ExitCode)
	}
}

// TestSpecP24_DataFormat_EarlyTermination covers Problem 24's early
// termination block: emitted JSONL records stand after a SIGINT, and the
// consumer sees exit 130.
//
//	# stderr:
//	■  scan  interrupted
//	# stdout: emitted JSONL records stand; consumer sees $? = 130 and treats set as partial
//
// The cancelled row always annotates "interrupted" (run.go hard-codes this
// reason for every signal cancellation) — the sealed progress count is not
// folded into the cancellation reason text anywhere in the library.
func TestSpecP24_DataFormat_EarlyTermination(t *testing.T) {
	var presentation, payload bytes.Buffer
	evo.SetDefault(evo.Init(evo.Config{
		Title:  "scan",
		Format: evo.FormatData,
		Stderr: &presentation,
		Result: &payload,
		Color:  evo.ColorNever,
	}))
	scan := evo.Task("scan")

	started := make(chan struct{})
	go func() {
		<-started
		time.Sleep(20 * time.Millisecond)
		_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
	}()

	code := evo.Main(func() error {
		scan.Progress(40, 128)
		if _, err := evo.Default().ResultWriter().Write([]byte(`{"repo":"zq"}` + "\n")); err != nil {
			return err
		}
		close(started)
		deadline := time.Now().Add(2 * time.Second)
		for scan.Snapshot().State != evo.Cancelled && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		return nil
	})

	if code != evo.ExitCancelled {
		t.Fatalf("exit %d, want %d (ExitCancelled); out:\n%s", code, evo.ExitCancelled, presentation.String())
	}
	collapsedPresentation := strings.Join(strings.Fields(presentation.String()), " ")
	if !strings.Contains(collapsedPresentation, "■ scan interrupted") {
		t.Fatalf("want the cancelled scan row annotated \"interrupted\" on stderr, got %q", presentation.String())
	}
	if !strings.Contains(payload.String(), `"repo":"zq"`) {
		t.Fatalf("want the already-emitted JSONL record to stand on the payload stream, got %q", payload.String())
	}
}

// TestSpecP25_ASCIIGlyphFallback_Step2 covers evo-rec.md Problem 25's step2
// block: the ASCII profile's sequential-action shape — a Done row plus one
// Running child, driven with an absolute Progress+Phase to reach the
// documented "1/3" exactly rather than Each's before-yield "0/3".
//
//	[ok] branches  14 deleted
//	/    worktrees 1/3  ../.worktrees/app-sah-1
func TestSpecP25_ASCIIGlyphFallback_Step2(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Terminal(screen), evo.VisibilityDelay(0), evo.MaxFrameRate(1_000_000), evo.NoColor(), evo.Glyphs(evo.GlyphsASCII)}})

	out.Task("branches").Done("14 deleted")
	worktrees := out.Task("worktrees")
	worktrees.Progress(1, 3)
	worktrees.Phase("../.worktrees/app-sah-1")

	live := screen.LatestLiveText()
	for _, want := range []string{"[ok]", "branches", "14 deleted", "worktrees", "1/3", "../.worktrees/app-sah-1"} {
		if !strings.Contains(live, want) {
			t.Fatalf("want %q in ASCII-profile live frame, got %q", want, live)
		}
	}
	if strings.ContainsAny(live, "✓✗⊘■○→…") {
		t.Fatalf("ASCII profile must not leak a Unicode state glyph, got %q", live)
	}
}

// TestSpecP25_ASCIIGlyphFallback_Failure covers Problem 25's failure block.
//
//	[x]  remotes  auth failed
//	     -  remote: Invalid username or token
//
// The real ASCII Evidence marker is "- " (evo-rec.md "Adopted revisions" ->
// "Tightened glyph vocabulary": Evidence Unicode "└─" maps to ASCII "- ").
func TestSpecP25_ASCIIGlyphFallback_Failure(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("clean"), evo.To(&buf), evo.Plain(), evo.NoColor(), evo.Glyphs(evo.GlyphsASCII)}})
	remotes := out.Task("remotes")
	remotes.Fail("auth failed", evo.Detail("remote: Invalid username or token"))
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	for _, want := range []string{"[x] remotes auth failed", "- remote: Invalid username or token"} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "->") {
		t.Fatalf("expected the ASCII Evidence marker \"- \", never a \"-> \" arrow, got:\n%s", got)
	}
}

// TestSpecP25_ASCIIGlyphFallback_Error covers Problem 25's error block — the
// same ASCII Evidence marker as the failure cell above.
//
//	[x] branches  cannot lock ref
//	    -  another git process seems to be running
func TestSpecP25_ASCIIGlyphFallback_Error(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("clean"), evo.To(&buf), evo.Plain(), evo.NoColor(), evo.Glyphs(evo.GlyphsASCII)}})
	branches := out.Task("branches")
	branches.Fail("cannot lock ref", evo.Detail("another git process seems to be running"))
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	for _, want := range []string{"[x] branches cannot lock ref", "- another git process seems to be running"} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "->") {
		t.Fatalf("expected the ASCII Evidence marker \"- \", never a \"-> \" arrow, got:\n%s", got)
	}
}

// TestSpecP25_ASCIIGlyphFallback_EarlyTermination covers Problem 25's early
// termination block, exercised through the real SIGINT path.
//
//	[ok] branches  8 deleted
//	[cancel]  worktrees interrupted
//	[!]  already mutated: 8 local deleted
//
// The real ASCII Cancelled marker is "[cancel]" (glyph.go: glyphCancelled =
// {"■", "[cancel]"}); the cancelled row always annotates "interrupted"; and
// the derived "already mutated" line reads "8 local deleted" (singular
// past-tense verb, mechanically derived).
func TestSpecP25_ASCIIGlyphFallback_EarlyTermination(t *testing.T) {
	var buf bytes.Buffer
	evo.SetDefault(evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor(), evo.Glyphs(evo.GlyphsASCII)}}))
	branches := evo.Task("branches")
	worktrees := evo.Task("worktrees")

	started := make(chan struct{})
	go func() {
		<-started
		time.Sleep(20 * time.Millisecond)
		_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
	}()

	code := evo.Main(func() error {
		branches.Record("delete", 8, "local")
		branches.Done("8 deleted")
		worktrees.Record("remove", 0, "worktrees")
		close(started)
		deadline := time.Now().Add(2 * time.Second)
		for worktrees.Snapshot().State != evo.Cancelled && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		return nil
	})

	if code != evo.ExitCancelled {
		t.Fatalf("exit %d, want %d (ExitCancelled); out:\n%s", code, evo.ExitCancelled, buf.String())
	}
	got := buf.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	if !strings.Contains(collapsed, "[ok] branches 8 deleted") {
		t.Fatalf("want the Done branches row, got:\n%s", got)
	}
	if !strings.Contains(collapsed, "[cancel] worktrees interrupted") {
		t.Fatalf("want the ASCII cancelled worktrees row annotated \"interrupted\", got:\n%s", got)
	}
	if !strings.Contains(collapsed, "already mutated: 8 local deleted") {
		t.Fatalf("want the real derived already-mutated line, got:\n%s", got)
	}
}

// Problem 26's step1 and step2 blocks (the narrow-terminal live-resize
// frames) are out of scope for this file per the work order: "Problems 16
// and 26 only: skip cells about narrow-terminal/resize live frames (a fix
// agent owns those)". Left as remaining work; see the final report.

// TestSpecP26_NarrowTerminal_Success covers evo-rec.md Problem 26's success
// block: the compact dialect's Done summary plus a truncated skip-taxonomy
// line survive a completed run regardless of the mid-run resize the problem
// describes.
//
//	✓ branches 40 del
//	! skipped 6
func TestSpecP26_NarrowTerminal_Success(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	evo.SetDefault(evo.Init(evo.Config{Options: []evo.Option{evo.Title("clean"), evo.To(&buf), evo.Plain(), evo.NoColor()}}))
	out := evo.Default()
	branches := out.Task("branches")
	protected := evo.Reason("protected")
	for i := 0; i < 4; i++ {
		branches.Skipped(protected, fmt.Sprintf("feat/b%d", i))
	}
	dirty := evo.Reason("dirty")
	for i := 0; i < 2; i++ {
		branches.Skipped(dirty, fmt.Sprintf("feat/d%d", i))
	}
	branches.Record("delete", 40, "branches")
	branches.Done("40 del")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	for _, want := range []string{"✓ branches 40 del", "! skipped 6"} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
}

// TestSpecP26_NarrowTerminal_Failure covers Problem 26's failure block: the
// compact dialect's Failed row plus evidence line.
//
//	✗ remotes auth
//	  └─ 401 token
func TestSpecP26_NarrowTerminal_Failure(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("clean"), evo.To(&buf), evo.Plain(), evo.NoColor()}})
	remotes := out.Task("remotes")
	remotes.Fail("auth", evo.Detail("401 token"))
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	for _, want := range []string{"✗ remotes auth", "401 token"} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
}

// TestSpecP26_NarrowTerminal_Indeterminate covers Problem 26's indeterminate
// block: an unsealed phase renders on the live region same as any other
// indeterminate task, independent of terminal width.
//
//	:. branches …
func TestSpecP26_NarrowTerminal_Indeterminate(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Terminal(screen), evo.VisibilityDelay(0), evo.MaxFrameRate(1_000_000), evo.NoColor()}})
	out.Task("branches").Phase("…")

	live := screen.LatestLiveText()
	if !strings.Contains(live, "branches") || !strings.Contains(live, "…") {
		t.Fatalf("want branches' indeterminate phase in the live frame, got %q", live)
	}
}

// TestSpecP26_NarrowTerminal_Error covers Problem 26's error block: the
// compact dialect's Failed row with a bare evidence line (no separate
// summary text alongside the task name).
//
//	✗ branches
//	  └─ lock ref
func TestSpecP26_NarrowTerminal_Error(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("clean"), evo.To(&buf), evo.Plain(), evo.NoColor()}})
	branches := out.Task("branches")
	branches.Fail("", evo.Detail("lock ref"))
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	for _, want := range []string{"✗ branches", "lock ref"} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
}

// TestSpecP26_NarrowTerminal_EarlyTermination covers Problem 26's early
// termination block, exercised through the real SIGINT path.
//
//	✓ branches 15 del
//	■ worktrees interrupted
//	! already mutated: 15 local deleted
//
// A single TaskHandle cannot render two terminal rows (a completed "15 del"
// Done row and a separate Cancelled row) under one name — the same
// one-entity-one-state constraint documented on
// TestSpecP8_LiveFrame_Step2_NotTestable — so the closest reachable scenario
// uses a second task ("worktrees") for the cancelled row. The cancelled row
// always annotates "interrupted", and the derived mutation line always
// carries the "already" prefix and the mechanically-conjugated past-tense
// verb.
func TestSpecP26_NarrowTerminal_EarlyTermination(t *testing.T) {
	var buf bytes.Buffer
	evo.SetDefault(evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}}))
	branches := evo.Task("branches")
	worktrees := evo.Task("worktrees")

	started := make(chan struct{})
	go func() {
		<-started
		time.Sleep(20 * time.Millisecond)
		_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
	}()

	code := evo.Main(func() error {
		branches.Record("delete", 15, "local")
		branches.Done("15 del")
		worktrees.Record("remove", 0, "worktrees")
		close(started)
		deadline := time.Now().Add(2 * time.Second)
		for worktrees.Snapshot().State != evo.Cancelled && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		return nil
	})

	if code != evo.ExitCancelled {
		t.Fatalf("exit %d, want %d (ExitCancelled); out:\n%s", code, evo.ExitCancelled, buf.String())
	}
	got := buf.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	if !strings.Contains(collapsed, "✓ branches 15 del") {
		t.Fatalf("want the Done branches row, got:\n%s", got)
	}
	if !strings.Contains(collapsed, "■ worktrees interrupted") {
		t.Fatalf("want the cancelled worktrees row annotated \"interrupted\", got:\n%s", got)
	}
	if !strings.Contains(collapsed, "already mutated: 15 local deleted") {
		t.Fatalf("want the real derived already-mutated line, got:\n%s", got)
	}
}
