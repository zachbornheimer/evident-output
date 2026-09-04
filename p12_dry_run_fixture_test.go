package evo_test

import (
	"bytes"
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
//	[planned] remote-tracking   prune 1 stale origin/*
//
// repo-retire's own call sites (Order H, out of this repo's scope) compose
// the "repo  <path>" subject text and the task/fact calls below — this test
// proves evo renders that shape correctly given those calls.
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

	branches.Warn("kept 13 (8 protected, 5 unpushed)")
	_ = branches.Delete("local tip", nil, evo.Affected(2))
	branches.Done()

	worktrees.Warn("kept 6 (4 dirty, 2 unpushed)")
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
		// sibling ("remote-tracking", 15 cells) before its annotation.
		"✓  branches         ! kept 13 (8 protected, 5 unpushed)\n",
		"✓  worktrees        ! kept 6 (4 dirty, 2 unpushed)\n",
		"✓  remote-tracking    1 stale\n",
		"[planned] branches         delete 2 local tips\n",
		"[planned] worktrees        remove 1 worktree\n",
		"[planned] remote-tracking  delete 1 stale origin/*\n",
	} {
		if !bytes.Contains([]byte(got), []byte(want)) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
	if bytes.Contains([]byte(got), []byte("[planned]  repo")) {
		t.Fatalf("must never render a trailing effectless ledger row, got:\n%s", got)
	}
}
