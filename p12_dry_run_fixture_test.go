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
	out := evo.Init(evo.Config{Isolated: true, DryRun: true, Subject: "repo  /Users/zbornheimer/Developer/Software-Automation-Holdings/bpp2.0", Stdout: &buf, Color: evo.ColorNever, Plain: true})

	protected := evo.Reason("protected")
	unpushedBranch := evo.Reason("unpushed")
	branches := out.Group("branches")
	for _, task := range branches.Each([]string{"p1", "p2", "p3", "p4", "p5", "p6", "p7", "p8"}) {
		task.Kept(protected)
	}
	for _, task := range branches.Each([]string{"u1", "u2", "u3", "u4", "u5"}) {
		task.Kept(unpushedBranch)
	}
	out.Task("branches").Delete("local tip", func() error { return nil }, evo.Affected(2))

	dirty := evo.Reason("dirty")
	unpushedWorktree := evo.Reason("unpushed")
	worktrees := out.Group("worktrees")
	for _, task := range worktrees.Each([]string{"w1", "w2", "w3", "w4"}) {
		task.Kept(dirty)
	}
	for _, task := range worktrees.Each([]string{"wu1", "wu2"}) {
		task.Kept(unpushedWorktree)
	}
	out.Task("worktrees").Remove("worktree", func() error { return nil }, evo.Affected(1))

	remoteTracking := out.Task("remote-tracking")
	remoteTracking.Fact("", "1 stale")
	remoteTracking.Delete("stale origin/*", func() error { return nil }, evo.Affected(1))

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	for _, want := range []string{
		"[dry-run] repo  /Users/zbornheimer/Developer/Software-Automation-Holdings/bpp2.0\n\n",
		"kept 13 (8 protected, 5 unpushed)",
		"kept 6 (4 dirty, 2 unpushed)",
		"1 stale",
		"[planned] branches",
		"delete 2 local tips",
		"[planned] worktrees",
		"remove 1 worktree",
		"[planned] remote-tracking",
		"delete 1 stale origin/*",
	} {
		if !bytes.Contains([]byte(got), []byte(want)) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
	if bytes.Contains([]byte(got), []byte("[planned]  repo")) {
		t.Fatalf("must never render a trailing effectless ledger row, got:\n%s", got)
	}
}
