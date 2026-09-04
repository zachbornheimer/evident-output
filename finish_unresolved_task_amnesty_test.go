package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestFinish_ReadmeQuickstart_EachLoopAutoResolvesDone is beginner-gate-2
// finding (i): the README quickstart's `evo.Task("install").Each(packages)`
// loop with no following Done. A completed Each loop sealed its absolute
// progress at total/total and told an honest, complete story — exactly the
// same easiest-path amnesty already given to a recorded mutation-verb
// effect — so it must not read as misuse.
func TestFinish_ReadmeQuickstart_EachLoopAutoResolvesDone(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})

	out.Task("working tree").Done()
	out.Task("branches").Block(
		"local-only branch",
		evo.Detail("commit or stash before continuing"),
	)
	out.Task("cleanup").Delete(2, "stale local branch")
	packages := []string{"a", "b", "c"}
	for range out.Task("install").Each(packages) {
		// install(pkg) — no explicit Done afterward, matching the README.
	}

	if err := out.Finish(); err != nil {
		t.Fatalf("Finish() = %v, want nil (a sealed Each loop should auto-resolve Done)", err)
	}
	rendered := buf.String()
	if strings.Contains(rendered, "misuse") {
		t.Fatalf("no misuse expected once the sealed loop auto-resolves:\n%s", rendered)
	}
	if code := out.Conclusion().ExitCode; code != evo.ExitBlocked {
		// "branches" is deliberately Blocked in this quickstart-verbatim
		// scenario; the band must stay sane (Blocked → exit 1), not escalate.
		t.Fatalf("exit code = %d, want %d (ExitBlocked, the quickstart's own Block)", code, evo.ExitBlocked)
	}
}

// TestFinish_TeachingLadder_EachThenReturnNil_NeverCancels is beginner-gate-2
// finding (ii): docs/guides/teaching-ladder.md's standalone example is
// `evo.Task("scan").Each(items); return nil` verbatim — an uninterrupted run
// must conclude OK, never [cancelled]/130, since nothing ever signaled the
// process.
func TestFinish_TeachingLadder_EachThenReturnNil_NeverCancels(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})

	items := []string{"one", "two"}
	for range out.Task("scan").Each(items) {
		// loop body — no explicit Done afterward, matching teaching-ladder.md.
	}

	if err := out.Finish(); err != nil {
		t.Fatalf("Finish() = %v, want nil (a sealed Each loop should auto-resolve Done)", err)
	}
	if code := out.Conclusion().ExitCode; code != evo.ExitOK {
		t.Fatalf("exit code = %d, want %d (ExitOK) — a sealed Each loop must never read as cancelled", code, evo.ExitOK)
	}
	if state := out.Conclusion().State; state == evo.StateCancelled {
		t.Fatalf("state = %v, want anything but StateCancelled on an uninterrupted run", state)
	}
	rendered := buf.String()
	if strings.Contains(rendered, "misuse") {
		t.Fatalf("no misuse expected once the sealed loop auto-resolves:\n%s", rendered)
	}
}

// TestFinish_TaxonomyOnlyTaskWithDeclinedConfirm_BlockedNeverEscalates is
// beginner-gate-2 finding (iii): a task that only recorded taxonomy
// (Skipped/Kept) and was never given a terminal verb told an honest,
// complete story already, same as a recorded effect or a sealed loop. And
// separately: a Confirm gate that resolves Blocked (declined / blocked by
// policy, per Confirm's own contract) must keep the run's exit code at
// ExitBlocked (1) — a leftover bookkeeping misuse must never silently
// escalate that to ExitFailed (2); the documented "Block → exit 1" contract
// wins.
func TestFinish_TaxonomyOnlyTaskWithDeclinedConfirm_BlockedNeverEscalates(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})

	reason := evo.Reason("protected")
	out.Task("prune branches").Skipped(reason, "release/2.0")

	if confirmed := out.Confirm("delete origin/production-hotfix?"); confirmed {
		t.Fatal("Confirm() = true, want false (plain mode without AssumeYes blocks by policy)")
	}

	if err := out.Finish(); err != nil {
		t.Fatalf("Finish() = %v, want nil (taxonomy-only task should auto-resolve Done, no misuse)", err)
	}
	rendered := buf.String()
	if strings.Contains(rendered, "misuse") {
		t.Fatalf("no misuse expected — taxonomy-only task auto-resolves, and Confirm's Blocked is not misuse:\n%s", rendered)
	}
	conc := out.Conclusion()
	if conc.State != evo.StateBlocked {
		t.Fatalf("state = %v, want StateBlocked (the declined Confirm gate)", conc.State)
	}
	if conc.ExitCode != evo.ExitBlocked {
		t.Fatalf("exit code = %d, want %d (ExitBlocked) — a recorded misuse must never escalate Block to Failed", conc.ExitCode, evo.ExitBlocked)
	}
}

// TestRun_BlockedWithLeftoverMisuse_NeverEscalatesToFailed exercises the
// same "Block → exit 1 wins" contract through the full Output.Run lifecycle
// (where Finish's returned misuse error previously forced ExitFailed even
// when the conclusion itself was Blocked): a genuinely blocked task plus an
// unrelated bookkeeping misuse (a double-resolve) must still leave the
// process exit code at ExitBlocked, per the documented "Block → exit 1"
// contract — bookkeeping misuse is not a reason to override the headline
// the run actually printed.
func TestRun_BlockedWithLeftoverMisuse_NeverEscalatesToFailed(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})

	code := out.Run(func(o *evo.Output) error {
		o.Task("branches").Block("local-only branch")
		o.Task("branches").Done() // already resolved — leftover bookkeeping misuse
		return nil
	})

	if code != evo.ExitBlocked {
		t.Fatalf("exit code = %d, want %d (ExitBlocked) — leftover misuse must not escalate the printed Block band to Failed:\n%s", code, evo.ExitBlocked, buf.String())
	}
}
