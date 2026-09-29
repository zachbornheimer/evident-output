package evo_test

import (
	"bytes"
	"context"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// readyBand is the trailing band a run of Skipped policy exclusions closes on.
const readyBand = "\n[ready]  prune\n"

// renderCategory renders one category Group whose work Task is named
// workName, with two kept items folded into its tally.
func renderCategory(t *testing.T, workName string) string {
	t.Helper()
	var buf bytes.Buffer
	out := newPlainOutput(&buf, false)
	t.Cleanup(func() { _ = out.Close() })
	items := out.Group("branches")
	work := items.Task(workName)
	work.Define(func(context.Context) error {
		items.Task("main").Skipped(evo.Reason("protected"))
		items.Task("feat/a").Skipped(evo.Reason("unpushed"))
		work.Summary("12 checked")
		return nil
	})
	if err := work.Wait(); err != nil {
		t.Fatal(err)
	}
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// TestOwnTask_SameNamedWorkTaskIsTheGroupRow pins the "own Task" rule
// (docs/reference.md): the Task named for its Group is the Group's own
// work, so once the items fold the Group renders as that one row.
func TestOwnTask_SameNamedWorkTaskIsTheGroupRow(t *testing.T) {
	want := "✓ branches  12 checked\n  - skipped 2 (1 protected, 1 unpushed)\n" + readyBand
	if got := renderCategory(t, "branches"); got != want {
		t.Fatalf("mismatch:\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

// TestOwnTask_DifferentlyNamedTaskNeverStandsInForItsGroup is the other
// half: a child with any other name cannot say which subject it is about.
// It never stands in for its Group, and it is a work peer of the kept
// children, which then keep their named rows: nothing says they are items
// of one category (docs/reference.md, "own Task").
func TestOwnTask_DifferentlyNamedTaskNeverStandsInForItsGroup(t *testing.T) {
	want := "✓ classify  12 checked\n○ main\n  - skipped 1 (protected)\n○ feat/a\n  - skipped 1 (unpushed)\n" + readyBand
	if got := renderCategory(t, "classify"); got != want {
		t.Fatalf("mismatch:\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

// renderSkippedFetch renders zq prune's --skip-fetch run: branches does
// its work, remote-tracking resolves Skipped with nothing else to say.
// skipped names the categories that resolve Skipped. ownGroups declares each category as a Group plus its own Task;
// otherwise the categories are peer Tasks under one Group.
func renderSkippedFetch(t *testing.T, ownGroups bool, skipped ...string) string {
	t.Helper()
	var buf bytes.Buffer
	out := newPlainOutput(&buf, false)
	t.Cleanup(func() { _ = out.Close() })
	categories := out.Group("categories")
	task := func(name string) *evo.TaskHandle {
		if ownGroups {
			return categories.Group(name).Task(name)
		}
		return categories.Task(name)
	}
	succeed(task("branches"), "3 checked")
	for _, name := range skipped {
		task(name).Skipped(evo.Reason("--skip-fetch"))
	}
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// TestOwnTask_IsNeverFoldedAsAnItem: a category's own Task that resolved
// Skipped is the category's row, not one of its items, so it keeps its
// own glyph and name.
func TestOwnTask_IsNeverFoldedAsAnItem(t *testing.T) {
	want := "✓ branches         3 checked\n○ remote-tracking\n  - skipped 1 (--skip-fetch)\n\n[ready]  prune\n"
	if got := renderSkippedFetch(t, true, "remote-tracking"); got != want {
		t.Fatalf("mismatch:\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

// TestOwnTask_LoneSkippedPeerKeepsItsRow: one Skipped Task beside other
// work is no aggregate, so it keeps its named row and the header-less
// Group does not grow a header to carry a nameless tally.
func TestOwnTask_LoneSkippedPeerKeepsItsRow(t *testing.T) {
	want := "✓ branches         3 checked\n○ remote-tracking\n  - skipped 1 (--skip-fetch)\n\n[ready]  prune\n"
	if got := renderSkippedFetch(t, false, "remote-tracking"); got != want {
		t.Fatalf("mismatch:\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

// TestOwnTask_SkippedPeersBesideWorkKeepTheirRows: two Skipped categories
// beside a category that did its own work are peers, not items of one
// subject, so neither folds into a nameless tally under a header the
// header-less Group never had.
func TestOwnTask_SkippedPeersBesideWorkKeepTheirRows(t *testing.T) {
	want := "✓ branches         3 checked\n○ remote-tracking\n  - skipped 1 (--skip-fetch)\n○ tags\n  - skipped 1 (--skip-fetch)\n\n[ready]  prune\n"
	if got := renderSkippedFetch(t, false, "remote-tracking", "tags"); got != want {
		t.Fatalf("mismatch:\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

// TestOwnTask_LoneKeptItemFoldsUnderItsGroupRow is zq prune on a small
// repo: main is always kept, so a category often has exactly one kept
// item. Under a Group with its own work Task that item is an item of the
// category, and it folds into the tally like any number of items would.
func TestOwnTask_LoneKeptItemFoldsUnderItsGroupRow(t *testing.T) {
	var buf bytes.Buffer
	out := newPlainOutput(&buf, false)
	t.Cleanup(func() { _ = out.Close() })
	items := out.Group("branches")
	work := items.Task("branches")
	work.Define(func(context.Context) error {
		items.Task("main").Skipped(evo.Reason("protected"))
		work.Summary("2 checked")
		return nil
	})
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	want := "✓ branches  2 checked\n  - skipped 1 (protected)\n" + readyBand
	if got := buf.String(); got != want {
		t.Fatalf("mismatch:\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}
