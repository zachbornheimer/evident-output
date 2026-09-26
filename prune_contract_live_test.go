package evo_test

import (
	"context"
	"io"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// liveCategory is one zq prune category caught mid-classification: its
// work Task is Running on item of done/total, with the items it has
// skipped so far already declared.
type liveCategory struct {
	name, item  string
	done, total int
	skipped     []skippedItem
}

// TestPruneContract_LiveCategoriesRenderContract18Frame pins the exact §18
// LIVE frame (Linear doc 9c10b754-895e-4852-aa25-f8cc5cafb2d0 §18)
// byte-for-byte, including the "zq prune  ~/repo" durable Subject header
// and the blank line separating it from the live region: three categories,
// each Running with a determinate bar/count and a current-item activity
// child, their name column aligned ("branches"/"worktrees"/
// "remote-tracking") and their count column aligned (numerator
// right-justified, denominator left-justified, to the widest sibling:
// 120/459, 70/294, 1/4 share one "/" column). No "- skipped N" tally shows
// for any of them — §18's own worked LIVE frame shows none while a
// category's own Task is still Running, whatever Skipped children it
// already has (the count could still grow before it settles).
//
// One documented departure, already established by TestV8_LiveParallelPrune:
// the doc's own illustrative frame shows "— 2s", but spec §24 fixes the
// elapsed-suffix threshold at 5 seconds of actual Running time
// (internal/render/live.go's elapsedAfter) — this advances the clock 5s,
// not 2s, and pins "— 5s".
func TestPruneContract_LiveCategoriesRenderContract18Frame(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	clock := testkit.NewClock()
	out := evo.Init(evo.Config{
		Isolated: true, Clock: clock, Terminal: screen, Stdout: io.Discard, Stderr: io.Discard,
		VisibilityDelay: evo.DelayForTest(0), Color: evo.ColorNever, MaxFrameRate: 1_000_000,
		Title: "zq", Subject: "zq prune  ~/repo",
	})
	t.Cleanup(func() { _ = out.Close() })

	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	checkedOut, protected := evo.Reason("checked out"), evo.Reason("protected")
	tracked := evo.Reason("tracked")
	categories := out.Group("categories")
	var repaint []func()
	for _, c := range []liveCategory{
		{"branches", "feat/style-contract", 120, 459, []skippedItem{{"feat/wt-a", checkedOut}}},
		{"worktrees", "eapp-system-style-contract-heading", 70, 294, []skippedItem{{"main", protected}}},
		{"remote-tracking", "origin/old-style", 1, 4, []skippedItem{{"origin/main", tracked}}},
	} {
		items := categories.Group(c.name)
		work := items.Task(c.name)
		classifying := make(chan struct{})
		work.Define(func(context.Context) error {
			for _, item := range c.skipped {
				items.Task(item.name).Skipped(item.reason)
			}
			work.Doing(c.item)
			work.Progress(c.done, c.total)
			close(classifying)
			<-release
			return nil
		})
		<-classifying
		repaint = append(repaint, func() { work.Progress(c.done, c.total) })
	}
	clock.Advance(5 * time.Second)
	for _, paint := range repaint {
		paint()
	}

	glyph := firstRune(screen.LatestLiveText())
	want := glyph + " branches         [████        ]  120/459  — 5s\n" +
		"  " + glyph + " feat/style-contract\n" +
		glyph + " worktrees        [███         ]   70/294  — 5s\n" +
		"  " + glyph + " eapp-system-style-contract-heading\n" +
		glyph + " remote-tracking  [███         ]    1/4    — 5s\n" +
		"  " + glyph + " origin/old-style"
	if got := screen.LatestLiveText(); got != want {
		t.Fatalf("mismatch:\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
	if got, want := screen.PersistedText(), "zq prune  ~/repo\n\n"; got != want {
		t.Fatalf("subject header mismatch:\n--- want ---\n%q\n--- got ---\n%q", want, got)
	}
}
