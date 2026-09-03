package evo_test

import (
	"io"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// TestConfirm_Yes_ResolvesOKAndReturnsTrue is red-first against the "confirm
// gate" default (evo-rec.md): "y" answers OK the gate and return true.
func TestConfirm_Yes_ResolvesOKAndReturnsTrue(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard), evo.Stdin(strings.NewReader("y\n"))}})

	if ok := out.Confirm("delete origin/production-hotfix?"); !ok {
		t.Fatal("Confirm(\"y\") = false, want true")
	}

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if out.Conclusion().ExitCode != evo.ExitOK {
		t.Fatalf("exit = %d, want ExitOK", out.Conclusion().ExitCode)
	}
	snap := out.Snapshot()
	if len(snap.Tasks) != 1 || snap.Tasks[0].State != evo.Done {
		t.Fatalf("item state = %+v, want OK", snap.Tasks)
	}
}

// TestConfirm_No_ResolvesBlockedDeclinedAndExitsOne proves a human "n" is a
// decline (Blocked, exit 1) — never a Failed row, never exit 2.
func TestConfirm_No_ResolvesBlockedDeclinedAndExitsOne(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard), evo.Stdin(strings.NewReader("n\n"))}})

	if ok := out.Confirm("delete origin/production-hotfix?"); ok {
		t.Fatal("Confirm(\"n\") = true, want false")
	}

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if out.Conclusion().State != evo.StateBlocked {
		t.Fatalf("state = %v, want StateBlocked", out.Conclusion().State)
	}
	if out.Conclusion().ExitCode != evo.ExitBlocked {
		t.Fatalf("exit = %d, want ExitBlocked (1)", out.Conclusion().ExitCode)
	}
	snap := out.Snapshot()
	if len(snap.Tasks) != 1 || snap.Tasks[0].State != evo.Blocked {
		t.Fatalf("item state = %+v, want Blocked", snap.Tasks)
	}
	if len(snap.Tasks[0].Problems) == 0 || snap.Tasks[0].Problems[0].Summary != "declined" {
		t.Fatalf("problems = %+v, want summary %q", snap.Tasks[0].Problems, "declined")
	}
}

// TestConfirm_EmptyAnswer_Declines proves an empty line (bare Enter) declines
// exactly like an explicit "n" — never blocks, never succeeds.
func TestConfirm_EmptyAnswer_Declines(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard), evo.Stdin(strings.NewReader("\n"))}})

	if ok := out.Confirm("proceed?"); ok {
		t.Fatal("Confirm(\"\") = true, want false")
	}
	snap := out.Snapshot()
	if snap.Tasks[0].State != evo.Blocked {
		t.Fatalf("item state = %v, want Blocked", snap.Tasks[0].State)
	}
}

// TestConfirm_ZeroByteEOF_BlocksByPolicyNotDecline is red-first for
// evo-rec.md "Confirm EOF = policy block, not decline": a zero-byte EOF on
// stdin (e.g. redirected from /dev/null, or piped from a closed process) is
// a policy block, distinct from a human explicitly typing anything else.
func TestConfirm_ZeroByteEOF_BlocksByPolicyNotDecline(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard), evo.Stdin(strings.NewReader(""))}})

	if ok := out.Confirm("proceed?"); ok {
		t.Fatal("Confirm(EOF) = true, want false")
	}

	snap := out.Snapshot()
	item := snap.Tasks[0]
	if item.State != evo.Blocked {
		t.Fatalf("item state = %v, want Blocked", item.State)
	}
	if len(item.Problems) == 0 || item.Problems[0].Summary != "blocked by policy" {
		t.Fatalf("problems = %+v, want summary %q (not %q)", item.Problems, "blocked by policy", "declined")
	}
	if len(item.Actions) == 0 || !strings.Contains(item.Actions[0].Label, "--yes") {
		t.Fatalf("actions = %+v, want a --yes hint", item.Actions)
	}
}

// TestConfirm_AssumeYes_SkipsPromptAndReadsNothing proves --yes never touches
// stdin and resolves OK "assumed --yes" without reading a line.
func TestConfirm_AssumeYes_SkipsPromptAndReadsNothing(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard), evo.Stdin(&panicReader{t: t})}})

	if ok := out.Confirm("delete origin/production-hotfix?", evo.AssumeYes(true)); !ok {
		t.Fatal("Confirm with AssumeYes(true) = false, want true")
	}
	snap := out.Snapshot()
	if snap.Tasks[0].State != evo.Done {
		t.Fatalf("item state = %v, want OK", snap.Tasks[0].State)
	}
	if snap.Tasks[0].Summary != "assumed --yes" {
		t.Fatalf("summary = %q, want %q", snap.Tasks[0].Summary, "assumed --yes")
	}
}

// TestConfirm_NonInteractive_BlocksByPolicyWithoutReadingStdin proves a
// non-interactive run without --yes never blocks waiting on stdin: the gate
// resolves Blocked "blocked by policy" with a Next hint, immediately.
func TestConfirm_NonInteractive_BlocksByPolicyWithoutReadingStdin(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{
		evo.To(io.Discard),
		evo.NonInteractive(),
		evo.Stdin(&panicReader{t: t}),
	}})

	if ok := out.Confirm("delete origin/production-hotfix?"); ok {
		t.Fatal("Confirm on non-interactive without AssumeYes = true, want false")
	}

	snap := out.Snapshot()
	item := snap.Tasks[0]
	if item.State != evo.Blocked {
		t.Fatalf("item state = %v, want Blocked", item.State)
	}
	if len(item.Problems) == 0 || item.Problems[0].Summary != "blocked by policy" {
		t.Fatalf("problems = %+v, want summary %q", item.Problems, "blocked by policy")
	}
	if len(item.Actions) == 0 || !strings.Contains(item.Actions[0].Label, "--yes") {
		t.Fatalf("actions = %+v, want a --yes hint", item.Actions)
	}
}

// TestConfirm_NonInteractive_DefaultPolicyHint_IsYesFlag is red-first for the
// hardcoded "--yes" hint: without PolicyHint, the policy block still points
// at --yes (today's behavior, unchanged by the strict-widening option).
func TestConfirm_NonInteractive_DefaultPolicyHint_IsYesFlag(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{
		evo.To(io.Discard),
		evo.NonInteractive(),
		evo.Stdin(&panicReader{t: t}),
	}})

	out.Confirm("delete origin/production-hotfix?")

	item := out.Snapshot().Tasks[0]
	if len(item.Actions) == 0 || !strings.Contains(item.Actions[0].Label, "--yes") {
		t.Fatalf("actions = %+v, want default --yes hint", item.Actions)
	}
}

// TestConfirm_NonInteractive_PolicyHint_OverridesDefaultYesHint is red-first
// for evo.PolicyHint: a caller whose confirm flag isn't --yes (e.g. zq
// clean-repo's --apply) must see its own flag in the policy-block hint, not
// the hardcoded "--yes" text.
func TestConfirm_NonInteractive_PolicyHint_OverridesDefaultYesHint(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{
		evo.To(io.Discard),
		evo.NonInteractive(),
		evo.Stdin(&panicReader{t: t}),
	}})

	out.Confirm("clean the repo?", evo.PolicyHint("zq", "clean-repo", "--apply"))

	item := out.Snapshot().Tasks[0]
	if len(item.Actions) == 0 || item.Actions[0].Command == nil {
		t.Fatalf("actions = %+v, want a command action", item.Actions)
	}
	got := item.Actions[0].Command
	if got.Executable != "zq" || strings.Join(got.Args, " ") != "clean-repo --apply" {
		t.Fatalf("hint command = %+v, want zq clean-repo --apply", got)
	}
	if strings.Contains(item.Actions[0].Label, "--yes") {
		t.Fatalf("actions = %+v, want no --yes hint once PolicyHint is set", item.Actions)
	}
}

// TestConfirm_QuiescesLiveRegion_NoFramesBetweenPromptAndAnswer proves the
// live region is cleared before the prompt and produces no further live
// frames while Confirm waits on the answer (evo-rec.md: "no spinner while
// waiting on a human").
func TestConfirm_QuiescesLiveRegion_NoFramesBetweenPromptAndAnswer(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := evo.Init(evo.Config{Options: []evo.Option{
		evo.Terminal(screen),
		evo.VisibilityDelay(0),
		evo.Stdin(strings.NewReader("y\n")),
	}})

	task := out.Task("branches")
	task.Phase("scanning")

	if ok := out.Confirm("delete origin/production-hotfix?", evo.Destructive()); !ok {
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
	task.Done("14 deleted")
}

// TestConfirm_Blocked_RendersBlockedGlyph proves the plain projection uses
// the distinct Blocked glyph (⊘), not the Failed glyph (✗) — the two
// terminal states must be visually distinguishable per evo-rec.md's tightened
// glyph vocabulary.
func TestConfirm_Blocked_RendersBlockedGlyph(t *testing.T) {
	var buf strings.Builder
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Stdin(strings.NewReader("n\n"))}})

	out.Confirm("delete origin/production-hotfix?")
	if err := out.Finish(); err != nil {
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

// panicReader fails the test if Confirm ever reads from it — used to prove a
// code path (AssumeYes, non-interactive policy block) never touches stdin.
type panicReader struct{ t *testing.T }

func (r *panicReader) Read(_ []byte) (int, error) {
	r.t.Helper()
	r.t.Fatal("Confirm read from stdin when it should not have")
	return 0, io.EOF
}
