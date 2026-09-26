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

// TestPruneContract_LiveCategoriesRenderGroupedShape holds the live phase
// of zq prune's grouped categories (the same out.Group("categories") →
// per-category Group → same-named work Task + Skipped item Tasks shape as
// TestPruneContract_SkippedUnderGroupedCategoriesRendersContract18) to this
// package's own recorded render: each category is a root row with its own
// bar, count, timer and current item. The summary-less "categories" Group
// owns no information of its own, so it paints no header — never a
// "categories  0/0 complete" row counting Tasks it does not directly hold.
//
// Every category here (including remote-tracking) has Skipped children,
// but contract §18 is explicit that no "- skipped N" tally shows while a
// category's own Task is still Running (the count could still grow before
// it settles) — so this frame, captured mid-run, shows none, matching
// TestPruneContract_LiveCategoriesRenderContract18Frame's own live shape.
// This fixture's counts still legitimately differ from §18's own worked
// numbers (all three categories share one skip reason set here, and
// remote-tracking has items §18's worked example does not), so it is named
// for what it pins — this package's live rendering of the grouped-category
// shape — rather than claim §18's own byte-for-byte numbers.
func TestPruneContract_LiveCategoriesRenderGroupedShape(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	clock := testkit.NewClock()
	out := evo.Init(evo.Config{
		Isolated: true, Clock: clock, Terminal: screen, Stdout: io.Discard, Stderr: io.Discard,
		VisibilityDelay: evo.DelayForTest(0), Color: evo.ColorNever, MaxFrameRate: 1_000_000,
	})
	t.Cleanup(func() { _ = out.Close() })

	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	checkedOut, protected := evo.Reason("checked out"), evo.Reason("protected")
	dirty, tracked := evo.Reason("dirty"), evo.Reason("tracked")
	categories := out.Group("categories")
	var repaint []func()
	for _, c := range []liveCategory{
		{"branches", "feat/style-contract", 120, 459, []skippedItem{{"feat/wt-a", checkedOut}, {"main", protected}}},
		{"worktrees", "eapp-system-style-contract-heading", 70, 294, []skippedItem{{"../wt-a", dirty}, {"../wt-b", dirty}}},
		{"remote-tracking", "origin/old-style", 1, 4, []skippedItem{{"origin/main", tracked}, {"origin/dev", tracked}}},
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
	clock.Advance(8 * time.Second)
	for _, paint := range repaint {
		paint()
	}

	glyph := firstRune(screen.LatestLiveText())
	// The count column is §18-aligned across these three siblings: the
	// numerator right-justified and the denominator left-justified to the
	// widest sibling, so every "/" lands in the same column.
	want := glyph + " branches         [████        ]  120/459  — 8s\n" +
		"  " + glyph + " feat/style-contract\n" +
		glyph + " worktrees        [███         ]   70/294  — 8s\n" +
		"  " + glyph + " eapp-system-style-contract-heading\n" +
		glyph + " remote-tracking  [███         ]    1/4    — 8s\n" +
		"  " + glyph + " origin/old-style"
	if got := screen.LatestLiveText(); got != want {
		t.Fatalf("mismatch:\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

// TestPruneContract_LiveCategoriesRenderContract18Frame pins the exact §18
// LIVE frame (Linear doc 9c10b754-895e-4852-aa25-f8cc5cafb2d0 §18)
// byte-for-byte: three categories, each Running with a determinate
// bar/count and a current-item activity child, their name column aligned
// ("branches"/"worktrees"/"remote-tracking") and their count column aligned
// (numerator right-justified, denominator left-justified, to the widest
// sibling: 120/459, 70/294, 1/4 share one "/" column). No "- skipped N"
// tally shows for any of them — contract §18 shows none while a category's
// own Task is still Running, whatever Skipped children it already has
// (TestPruneContract_LiveCategoriesRenderGroupedShape's own rationale).
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
}
