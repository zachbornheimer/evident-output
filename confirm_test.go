package evo

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/internal/record"

	"github.com/zachbornheimer/evident-output/v2/testkit"
)

// TestConfirm_Yes_ResolvesOKAndReturnsTrue is red-first against the "confirm
// gate" default (evo-rec.md): "y" answers OK the gate and return true.
func TestConfirm_Yes_ResolvesOKAndReturnsTrue(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := initRun(Config{Facades: Facades{Stdout: io.Discard, Stderr: io.Discard, Terminal: screen, Stdin: strings.NewReader("y\n")}}, tuning{color: colorNever})

	if ok := out.Confirm("delete origin/production-hotfix?"); !ok {
		t.Fatal("Confirm(\"y\") = false, want true")
	}

	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	if out.conclusion().exitCode != exitOK {
		t.Fatalf("exit = %d, want ExitOK", out.conclusion().exitCode)
	}
	snap := out.snapshot()
	if len(snap.Tasks) != 1 || snap.Tasks[0].State != succeeded {
		t.Fatalf("item state = %+v, want OK", snap.Tasks)
	}
}

// TestConfirm_No_ResolvesBlockedDeclinedAndExitsOne proves a human "n" is a
// decline (Blocked, exit 1) — never a Failed row, never exit 2.
func TestConfirm_No_ResolvesBlockedDeclinedAndExitsOne(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := initRun(Config{Facades: Facades{Stdout: io.Discard, Stderr: io.Discard, Terminal: screen, Stdin: strings.NewReader("n\n")}}, tuning{color: colorNever})

	if ok := out.Confirm("delete origin/production-hotfix?"); ok {
		t.Fatal("Confirm(\"n\") = true, want false")
	}

	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	if out.conclusion().headline() != headlineRefused {
		t.Fatalf("state = %v, want StateBlocked", out.conclusion().headline())
	}
	if out.conclusion().exitCode != exitRefused {
		t.Fatalf("exit = %d, want ExitBlocked (1)", out.conclusion().exitCode)
	}
	snap := out.snapshot()
	if len(snap.Tasks) != 1 || snap.Tasks[0].State != refused {
		t.Fatalf("item state = %+v, want Blocked", snap.Tasks)
	}
	if len(snap.Tasks[0].Problems) == 0 || snap.Tasks[0].Problems[0].Summary != "declined" {
		t.Fatalf("problems = %+v, want summary %q", snap.Tasks[0].Problems, "declined")
	}
}

// TestConfirm_EmptyAnswer_Declines proves an empty line (bare Enter) declines
// exactly like an explicit "n" — never blocks, never succeeds.
func TestConfirm_EmptyAnswer_Declines(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := initRun(Config{Facades: Facades{Stdout: io.Discard, Stderr: io.Discard, Terminal: screen, Stdin: strings.NewReader("\n")}}, tuning{color: colorNever})

	if ok := out.Confirm("proceed?"); ok {
		t.Fatal("Confirm(\"\") = true, want false")
	}
	snap := out.snapshot()
	if snap.Tasks[0].State != refused {
		t.Fatalf("item state = %v, want Blocked", snap.Tasks[0].State)
	}
}

// TestConfirm_ZeroByteEOF_BlocksByPolicyNotDecline is red-first for
// evo-rec.md "Confirm EOF = policy block, not decline": a zero-byte EOF on
// stdin (e.g. redirected from /dev/null, or piped from a closed process) is
// a policy block, distinct from a human explicitly typing anything else. Its
// summary is also distinct from the no-TTY policy-block wording (release-gate
// round 4 finding 6): the reader was told nothing arrived, not that a
// deliberate policy refused the prompt.
// policyBlockActions is the remedy the policy block carries. ZYS-1182: it
// rides on the block's Problem (Problems[].Actions), not on the Task.
func policyBlockActions(t *testing.T, item taskSnapshot) []record.Action {
	t.Helper()
	if len(item.Problems) == 0 {
		t.Fatalf("task has no Problem to carry the policy hint: %+v", item)
	}
	if len(item.Actions) != 0 {
		t.Fatalf("task-level actions = %+v, want the hint only on the Problem", item.Actions)
	}
	return item.Problems[0].Actions
}

func TestConfirm_ZeroByteEOF_BlocksByPolicyNotDecline(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := initRun(Config{Facades: Facades{Stdout: io.Discard, Stderr: io.Discard, Terminal: screen, Stdin: strings.NewReader("")}}, tuning{color: colorNever})

	if ok := out.Confirm("proceed?"); ok {
		t.Fatal("Confirm(EOF) = true, want false")
	}

	snap := out.snapshot()
	item := snap.Tasks[0]
	if item.State != refused {
		t.Fatalf("item state = %v, want Blocked", item.State)
	}
	if len(item.Problems) == 0 || item.Problems[0].Summary != "no answer — stdin closed" {
		t.Fatalf("problems = %+v, want summary %q (not %q or %q)", item.Problems, "no answer — stdin closed", "blocked by policy", "declined")
	}
	if actions := policyBlockActions(t, item); len(actions) == 0 || !strings.Contains(actions[0].Label, "--yes") {
		t.Fatalf("actions = %+v, want a --yes hint", actions)
	}
}

// TestConfirm_NonInteractive_BlocksByPolicyWithoutReadingStdin proves a
// non-interactive run without --yes never blocks waiting on stdin: the gate
// resolves Blocked "blocked by policy" with a Next hint, immediately.
func TestConfirm_NonInteractive_BlocksByPolicyWithoutReadingStdin(t *testing.T) {
	out := initRun(Config{Output: Plain, Facades: Facades{Stderr: io.Discard, Stdin: testkit.UnreadableStdin(t)}})

	if ok := out.Confirm("delete origin/production-hotfix?"); ok {
		t.Fatal("Confirm on non-interactive without AssumeYes = true, want false")
	}

	snap := out.snapshot()
	item := snap.Tasks[0]
	if item.State != refused {
		t.Fatalf("item state = %v, want Blocked", item.State)
	}
	if len(item.Problems) == 0 || item.Problems[0].Summary != "blocked by policy" {
		t.Fatalf("problems = %+v, want summary %q", item.Problems, "blocked by policy")
	}
	if actions := policyBlockActions(t, item); len(actions) == 0 || !strings.Contains(actions[0].Label, "--yes") {
		t.Fatalf("actions = %+v, want a --yes hint", actions)
	}
}

// TestConfirm_NonInteractive_DefaultPolicyHint_IsYesFlag is red-first for the
// hardcoded "--yes" hint: without PolicyHint, the policy block still points
// at --yes (today's behavior, unchanged by the strict-widening option).
func TestConfirm_NonInteractive_DefaultPolicyHint_IsYesFlag(t *testing.T) {
	out := initRun(Config{Output: Plain, Facades: Facades{Stderr: io.Discard, Stdin: testkit.UnreadableStdin(t)}})

	out.Confirm("delete origin/production-hotfix?")

	item := out.snapshot().Tasks[0]
	if actions := policyBlockActions(t, item); len(actions) == 0 || !strings.Contains(actions[0].Label, "--yes") {
		t.Fatalf("actions = %+v, want default --yes hint", actions)
	}
}

// TestConfirm_NonInteractive_EmptyFlagKeepsDefaultHint pins the one-string
// signature: a zero flag names nothing, so the refusal keeps pointing at --yes.
func TestConfirm_NonInteractive_EmptyFlagKeepsDefaultHint(t *testing.T) {
	out := initRun(Config{Output: Plain, Facades: Facades{Stderr: io.Discard, Stdin: testkit.UnreadableStdin(t)}})

	out.Confirm("delete origin/production-hotfix?", NonInteractive(""))

	item := out.snapshot().Tasks[0]
	if actions := policyBlockActions(t, item); len(actions) == 0 || !strings.Contains(actions[0].Label, "--yes") {
		t.Fatalf("actions = %+v, want default --yes hint", actions)
	}
}

// TestConfirm_QuiescesLiveRegion_NoFramesBetweenPromptAndAnswer proves the
// live region is cleared before the prompt and produces no further live
// frames while Confirm waits on the answer (evo-rec.md: "no spinner while
// waiting on a human").
func TestConfirm_QuiescesLiveRegion_NoFramesBetweenPromptAndAnswer(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := initRun(Config{Facades: Facades{Stdout: io.Discard, Stderr: io.Discard, Terminal: screen, Stdin: strings.NewReader("y\n")}}, tuning{visibilityDelay: DelayForTest(0)})

	task := out.Task("branches")
	task.Doing("scanning")

	if ok := out.Confirm("delete origin/production-hotfix?", Destructive()); !ok {
		t.Fatal("Confirm(\"y\") = false, want true")
	}

	// The gate must resolve (durable OK/Blocked row) before any live frame is
	// allowed to redraw again — no spinner frame may land between the prompt
	// and the answer being recorded, even while a sibling Task keeps running.
	ops := screen.Operations()
	promptIdx, resolveIdx := -1, -1
	for i, op := range ops {
		if op.Kind == "durable" && strings.Contains(op.Text, "[y/N]") {
			promptIdx = i
		}
		if op.Kind == "durable" && strings.Contains(op.Text, "delete origin/production-hotfix?") && !strings.Contains(op.Text, "[y/N]") {
			resolveIdx = i
			break
		}
	}
	if promptIdx == -1 || resolveIdx == -1 || resolveIdx <= promptIdx {
		t.Fatalf("could not find prompt/resolve durable ops in order: %+v", ops)
	}
	for _, op := range ops[promptIdx+1 : resolveIdx] {
		if op.Kind == "live" {
			t.Fatalf("live frame landed between prompt and resolved answer: %+v", ops)
		}
	}
	if !strings.Contains(ops[promptIdx].Text, "(destructive)") {
		t.Fatalf("prompt line missing (destructive) tag: %q", ops[promptIdx].Text)
	}
	if !strings.Contains(ops[promptIdx].Text, "\n›") {
		t.Fatalf("prompt line missing the › input line (P11): %q", ops[promptIdx].Text)
	}
	succeed(task, "14 deleted")
}

// TestConfirm_Blocked_RendersBlockedGlyph proves the plain projection uses
// the distinct Blocked glyph (⊘), not the Failed glyph (✗) — the two
// terminal states must be visually distinguishable per evo-rec.md's tightened
// glyph vocabulary.
func TestConfirm_Blocked_RendersBlockedGlyph(t *testing.T) {
	var buf strings.Builder
	out := initRun(Config{Facades: Facades{Stderr: &buf, Stdin: strings.NewReader("n\n")}})

	out.Confirm("delete origin/production-hotfix?")
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}

	rendered := buf.String()
	if !strings.Contains(rendered, "⊘") {
		t.Fatalf("rendered output missing ⊘ (Blocked) glyph:\n%s", rendered)
	}
	if strings.Contains(rendered, "✗") {
		t.Fatalf("rendered output used ✗ (Failed) glyph for a decline:\n%s", rendered)
	}
}

// TestConfirm_Declined_ResolvesBlockedWithExitOne golden-proves the
// doc's example: a declined confirmation is Blocked, never Failed, and the
// process-level exit code is 1 (ExitBlocked), not 2 (ExitFailed).
func TestConfirm_Declined_ResolvesBlockedWithExitOne(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})

	// Plain/non-interactive mode without AssumeYes blocks by policy rather
	// than reading stdin — the same declined-by-policy path a real TTY
	// decline resolves through.
	if confirmed := out.Confirm("delete origin/production-hotfix?"); confirmed {
		t.Fatal("Confirm() = true, want false")
	}

	if err := out.finish(); err != nil {
		t.Log(err)
	}
	conc := out.conclusion()
	if conc.headline() != headlineRefused {
		t.Fatalf("state = %v, want StateBlocked (a declined confirm is Blocked, never Failed)", conc.headline())
	}
	if conc.exitCode != exitRefused {
		t.Fatalf("exit code = %d, want %d (ExitBlocked)", conc.exitCode, exitRefused)
	}
}
