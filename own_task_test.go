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
