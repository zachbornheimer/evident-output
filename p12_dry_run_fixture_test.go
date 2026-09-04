package evo_test

import (
	"bytes"
	"fmt"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestP12_DryRunFixtureShape is red-first against
// .../scratchpad/static-ui/fixture-repo-retire-dryrun.md — the user-provided
// exact rendering of `repo-retire clean --dry-run`. It proves evo's plain
// projection produces byte-identical typography for the shape the fixture
// pins:
//
//	[dry-run] repo  <subject>
//
//	✓ branches          ! kept 13 (8 protected, 5 unpushed)
//	✓ worktrees         ! kept 6 (4 dirty, 2 unpushed)
//	✓ remote-tracking     1 stale
//
//	[planned] branches          delete 2 local tips
//	[planned] worktrees         remove 1 worktree
//	[planned] remote-tracking   delete 1 stale origin/*
//
// The "kept N (...)" annotation is exercised through the real
// TaskHandle.Kept accumulator (8 "protected" + 5 "unpushed" records), not a
// pre-composed Warn string — proving the taxonomy tally itself, not just its
// text, renders inline on the task's own row (fixture core.Problem 1: "task.Kept
// keep-tallies currently render as a separate line" is the bug this closes).
// repo-retire's own call sites (Order H, out of this repo's scope) compose
// the "repo  <path>" subject text and the task/fact calls below — this test
// proves evo renders that shape correctly given those calls. The ledger's
// mutation verb for remote-tracking uses Delete (evo's fixed verb set has no
// "prune" — repo-retire's own call site choice is out of this repo's scope;
// the fixture's acceptance step allows verb/count text to differ, pinning
// shape and typography only).
func TestP12_DryRunFixtureShape(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true,
		DryRun:   true,
		Subject:  "repo  /Users/zbornheimer/Developer/Software-Automation-Holdings/bpp2.0",
		Options: []evo.Option{
			evo.To(&buf), evo.NoColor(), evo.Plain(),
		},
	})

	// The caller declares its whole known set of sibling tasks up front —
	// the pattern that lets progressive/immediate rendering (§17.5) still
	// align the column across rows it hasn't resolved yet (see
	// maxRootTaskNameWidth's doc comment).
	branches := out.Task("branches")
	worktrees := out.Task("worktrees")
	remoteTracking := out.Task("remote-tracking")

	protected := evo.Reason("protected")
	unpushedBranch := evo.Reason("unpushed")
	for i := 0; i < 8; i++ {
		branches.Kept(protected, fmt.Sprintf("protected-%d", i))
	}
	for i := 0; i < 5; i++ {
		branches.Kept(unpushedBranch, fmt.Sprintf("unpushed-%d", i))
	}
	_ = branches.Delete("local tip", nil, evo.Affected(2))
	branches.Done()

	dirty := evo.Reason("dirty")
	unpushedWorktree := evo.Reason("unpushed")
	for i := 0; i < 4; i++ {
		worktrees.Kept(dirty, fmt.Sprintf("dirty-%d", i))
	}
	for i := 0; i < 2; i++ {
		worktrees.Kept(unpushedWorktree, fmt.Sprintf("unpushed-%d", i))
	}
	_ = worktrees.Remove("worktree", nil, evo.Affected(1))
	worktrees.Done()

	remoteTracking.Fact("", "1 stale")
	_ = remoteTracking.Delete("stale origin/*", nil, evo.Affected(1))
	remoteTracking.Done()

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	for _, want := range []string{
		"[dry-run] repo  /Users/zbornheimer/Developer/Software-Automation-Holdings/bpp2.0\n\n",
		// One shared column: every root task's name pads to the widest
		// sibling ("remote-tracking", 15 cells) plus one margin column
		// before its annotation (taskNameColumnMargin) — verified
		// byte-for-byte against the fixture.
		"✓ branches          ! kept 13 (8 protected, 5 unpushed)\n",
		"✓ worktrees         ! kept 6 (4 dirty, 2 unpushed)\n",
		"✓ remote-tracking     1 stale\n",
		"[planned] branches          delete 2 local tips\n",
		"[planned] worktrees         remove 1 worktree\n",
		"[planned] remote-tracking   delete 1 stale origin/*\n",
	} {
		if !bytes.Contains([]byte(got), []byte(want)) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
	if bytes.Contains([]byte(got), []byte("[planned]  repo")) {
		t.Fatalf("must never render a trailing effectless ledger row, got:\n%s", got)
	}
}
