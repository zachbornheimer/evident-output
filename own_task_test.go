package evo_test

import (
	"bytes"
	"context"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// warnedBand is the trailing band a run with a Kept tally closes on.
const warnedBand = "\n[ready · warned]  prune\n"

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
		items.Task("main").Kept(evo.Reason("protected"))
		items.Task("feat/a").Kept(evo.Reason("unpushed"))
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
	want := "✓ branches  12 checked\n  ! kept 2 (1 protected, 1 unpushed)\n" + warnedBand
	if got := renderCategory(t, "branches"); got != want {
		t.Fatalf("mismatch:\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

// TestOwnTask_DifferentlyNamedTaskNeverStandsInForItsGroup is the other
// half: a child with any other name cannot say which subject it is about,
// so the Group keeps its header and the child keeps its own row.
func TestOwnTask_DifferentlyNamedTaskNeverStandsInForItsGroup(t *testing.T) {
	want := "✓ branches\n  ! kept 2 (1 protected, 1 unpushed)\n   ✓ classify  12 checked\n" + warnedBand
	if got := renderCategory(t, "classify"); got != want {
		t.Fatalf("mismatch:\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

// renderSkippedFetch renders zq prune's --skip-fetch run: branches does
// its work, remote-tracking resolves Skipped with nothing else to say.
// ownGroups declares each category as a Group plus its own Task;
// otherwise the categories are peer Tasks under one Group.
func renderSkippedFetch(t *testing.T, ownGroups bool) string {
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
	task("remote-tracking").Skipped(evo.Reason("--skip-fetch"))
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
	if got := renderSkippedFetch(t, true); got != want {
		t.Fatalf("mismatch:\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

// TestOwnTask_LoneSkippedPeerKeepsItsRow: one Skipped Task beside other
// work is no aggregate, so it keeps its named row and the header-less
// Group does not grow a header to carry a nameless tally.
func TestOwnTask_LoneSkippedPeerKeepsItsRow(t *testing.T) {
	want := "✓ branches         3 checked\n○ remote-tracking\n  - skipped 1 (--skip-fetch)\n\n[ready]  prune\n"
	if got := renderSkippedFetch(t, false); got != want {
		t.Fatalf("mismatch:\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}
