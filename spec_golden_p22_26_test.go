package evo_test

import (
	"bytes"
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
	out := evo.NewWithOptions(evo.Terminal(screen), evo.VisibilityDelay(0), evo.NoColor(), evo.Stdin(strings.NewReader("y\n")))

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
	out := evo.NewWithOptions(evo.Terminal(screen), evo.VisibilityDelay(0), evo.NoColor(), evo.Stdin(strings.NewReader("y\n")))

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
	out := evo.NewWithOptions(evo.Terminal(screen), evo.VisibilityDelay(0), evo.MaxFrameRate(1_000_000), evo.NoColor(), evo.Stdin(strings.NewReader("y\n")))

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
	out := evo.NewWithOptions(evo.To(&buf), evo.NoColor(), evo.Stdin(strings.NewReader("y\n")))

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

// TestSpecP22_ConfirmGate_Failure_Mismatch covers Problem 22's failure block.
//
//	⊘  confirm remote delete  declined
//	!  nothing mutated
//	# $? = 1 (Blocked)
//
// MISMATCH (executed, not fixed): the decline half renders exactly as
// documented (⊘ Blocked "declined", exit 1). But "!  nothing mutated" has no
// clean public spelling: Println (the caller-narrative idiom Problem 15
// teaches) carries no glyph at all, and the only glyph-bearing narrative
// primitive, Item(...).Warn(""), renders a spurious empty "└─" child line for
// an empty summary — neither reproduces a bare "!  nothing mutated" row.
func TestSpecP22_ConfirmGate_Failure_Mismatch(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.To(&buf), evo.NoColor(), evo.Stdin(strings.NewReader("n\n")))

	if ok := out.Confirm("confirm remote delete", evo.Destructive()); ok {
		t.Fatal("Confirm(\"n\") = true, want false")
	}
	out.Println("nothing mutated")
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
	t.Skip("MISMATCH: spec wants a glyph-bearing \"!  nothing mutated\" line; the closest public spelling (out.Println) renders \"nothing mutated\" with no glyph, and Item(...).Warn(\"\") renders \"!  nothing mutated\" but with a spurious empty evidence line attached — see doc comment")
}

// TestSpecP22_ConfirmGate_Error_Mismatch covers Problem 22's error block.
//
//	⊘  confirm remote delete  no interactive stdin — blocked by policy
//	   └─ pass --yes to confirm non-interactively
//	!  nothing mutated
//	# $? = 1 (Blocked) — policy block, distinct from a human decline and from Failed
//
// MISMATCH (executed, not fixed): the real non-interactive policy block
// renders as two separate durable lines —
//
//	⊘  confirm remote delete
//	   └─ blocked by policy
//	→  pass --yes to confirm non-interactively
//
// — never the spec's single "<question>  no interactive stdin — blocked by
// policy" line (the "no interactive stdin —" cause clause is not part of
// confirmPolicyBlockedSummary in confirm.go; the summary is always the bare
// "blocked by policy"), and the hint renders via the Next-action glyph (→,
// per the Adopted revisions glyph table) rather than the spec's Evidence
// glyph (└─). "!  nothing mutated" is the same unproducible line documented
// on the failure cell above.
func TestSpecP22_ConfirmGate_Error_Mismatch(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.To(&buf), evo.NoColor(), evo.NonInteractive(), evo.Stdin(&panicReader{t: t}))

	if ok := out.Confirm("confirm remote delete"); ok {
		t.Fatal("Confirm on non-interactive without --yes = true, want false")
	}
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "⊘  confirm remote delete") || !strings.Contains(got, "blocked by policy") {
		t.Fatalf("want a policy-blocked row, got:\n%s", got)
	}
	t.Skip("MISMATCH: real summary is bare \"blocked by policy\" (missing the \"no interactive stdin —\" cause clause) rendered on its own line, and the --yes hint uses the Next-action glyph → rather than the spec's Evidence glyph └─; \"!  nothing mutated\" is unproducible as documented on the failure cell — see doc comment")
}

// TestSpecP22_ConfirmGate_EarlyTermination_Mismatch covers Problem 22's early
// termination block, exercised through the real SIGINT path
// (runInterruptible → cancelActive → cancelPendingConfirmLocked), the same
// mechanism run_signal_test.go and confirm_signal_test.go already prove.
//
//	■  confirm remote delete  ^C at prompt
//	!  already mutated: none
//
// MISMATCH (executed, not fixed): the cancelled gate always annotates
// "interrupted" (run.go hard-codes this reason string for every SIGINT/
// SIGTERM cancellation), never "^C at prompt". And "! already mutated: ..."
// is deliberately suppressed entirely when the effect ledger is empty
// (plain.go's writeAlreadyMutated: "an empty ledger earns no attention, so
// the row is suppressed entirely rather than rendered as 'none'") — a
// documented, deliberately-adopted design decision the spec's literal "none"
// spelling predates.
func TestSpecP22_ConfirmGate_EarlyTermination_Mismatch(t *testing.T) {
	var buf bytes.Buffer
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	defer func() { _ = w.Close() }()
	evo.SetDefault(evo.NewWithOptions(evo.To(&buf), evo.NoColor(), evo.Stdin(r)))

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
	if !strings.Contains(got, "■") || !strings.Contains(got, "confirm remote delete") {
		t.Fatalf("want the cancelled gate row, got:\n%s", got)
	}
	t.Skip("MISMATCH: real reason text is \"interrupted\" (run.go hard-codes it for every signal cancellation), never \"^C at prompt\"; \"! already mutated: none\" never renders — an empty effect ledger deliberately suppresses the whole row (plain.go writeAlreadyMutated) rather than printing \"none\" — see doc comment")
}
