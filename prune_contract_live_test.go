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
// work Task is Running on item of done/total, with the items it has kept
// so far already declared.
type liveCategory struct {
	name, item  string
	done, total int
	kept        []keptItem
}

// TestPruneContract_LiveCategoriesRenderContract18Frame holds the live
// phase of zq prune's grouped categories (the same out.Group("categories")
// → per-category Group → same-named work Task + Kept item Tasks shape as
// TestPruneContract_KeptUnderGroupedCategoriesRendersContract18) to the
// contract §18 live frame: each category is a root row with its own bar,
// count, timer and current item. The summary-less "categories" Group owns
// no information of its own, so it paints no header — never a
// "categories  0/0 complete" row counting Tasks it does not directly hold.
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
	dirty, tracked := evo.Reason("dirty"), evo.Reason("tracked")
	categories := out.Group("categories")
	var repaint []func()
	for _, c := range []liveCategory{
		{"branches", "feat/style-contract", 120, 459, []keptItem{{"feat/wt-a", checkedOut}, {"main", protected}}},
		{"worktrees", "eapp-system-style-contract-heading", 70, 294, []keptItem{{"../wt-a", dirty}, {"../wt-b", dirty}}},
		{"remote-tracking", "origin/old-style", 1, 4, []keptItem{{"origin/main", tracked}, {"origin/dev", tracked}}},
	} {
		items := categories.Group(c.name)
		work := items.Task(c.name)
		classifying := make(chan struct{})
		work.Define(func(context.Context) error {
			for _, kept := range c.kept {
				items.Task(kept.name).Kept(kept.reason)
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
	want := glyph + " branches         [███         ]  120/459 — 8s\n" +
		"   " + glyph + " feat/style-contract\n" +
		"  ! kept 2 (1 checked out, 1 protected)\n" +
		glyph + " worktrees        [██          ]  70/294 — 8s\n" +
		"   " + glyph + " eapp-system-style-contract-heading\n" +
		"  ! kept 2 (dirty)\n" +
		glyph + " remote-tracking  [███         ]  1/4 — 8s\n" +
		"   " + glyph + " origin/old-style\n" +
		"  ! kept 2 (tracked)"
	if got := screen.LatestLiveText(); got != want {
		t.Fatalf("mismatch:\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}
