package evo_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// skippedWithFactsRun is zq prune's in-use shape: one Task per skipped item,
// each with a Fact saying why (row := group.Task(name); row.Fact("why",
// d); row.Skipped(r)). Ordinary call sites moved to Skipped(reason) in 1.1; the fold
// this test pins moved with it.
func skippedWithFactsRun(t *testing.T, v evo.Verbosity) string {
	t.Helper()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, StateDir: t.TempDir(), Stdout: &buf, Title: "prune", Color: evo.ColorNever, Plain: true, Verbosity: v})
	g := out.Group("worktrees")
	for _, n := range []string{"feat1", "a-much-longer-worktree-name", "main"} {
		row := g.Task(n)
		if n != "main" {
			row.Fact("why", "current checkout")
		}
		row.Skipped(evo.Reason("in use"))
	}
	_ = out.Finish()
	_ = out.Close()
	return buf.String()
}

// TestSkippedItemFacts_KeepTheFold pins E-100: a Fact on a skipped child broke
// the fold under verbose, so each child rendered its own "✓ name why …"
// row and "- skipped 1 (…)". The fold holds; verbose lists each item once
// with its Facts at one column, whatever the name width.
func TestSkippedItemFacts_KeepTheFold(t *testing.T) {
	for _, v := range []evo.Verbosity{evo.VerbosityNormal, evo.VerbosityVerbose} {
		got := skippedWithFactsRun(t, v)
		if strings.Count(got, "- skipped") != 1 || !strings.Contains(got, "- skipped 3 (in use)") {
			t.Errorf("verbosity %d: want one \"- skipped 3 (in use)\" tally:\n%s", v, got)
		}
		if strings.Contains(got, "✓ feat1") || strings.Contains(got, "✓ main") {
			t.Errorf("verbosity %d: a skipped item got its own ✓ row:\n%s", v, got)
		}
	}
	got := skippedWithFactsRun(t, evo.VerbosityVerbose)
	var cols []int
	for line := range strings.SplitSeq(got, "\n") {
		if i := strings.Index(line, "why  current checkout"); i >= 0 {
			cols = append(cols, i)
		}
	}
	if len(cols) != 2 || cols[0] != cols[1] {
		t.Errorf("verbose: want both items' Facts at one column, got columns %v:\n%s", cols, got)
	}
	if !strings.Contains(got, "main") {
		t.Errorf("verbose: the item with no Fact is not listed:\n%s", got)
	}
}

// manySkippedOneFactRun is n skipped items where only the first carries a Fact,
// named name (empty for the value-only spelling evo.Fact("", v)).
func manySkippedOneFactRun(t *testing.T, n int, name string) string {
	t.Helper()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, StateDir: t.TempDir(), Stdout: &buf, Title: "prune", Color: evo.ColorNever, Plain: true, Verbosity: evo.VerbosityVerbose})
	g := out.Group("worktrees")
	for i := range n {
		row := g.Task(fmt.Sprintf("wt%04d", i))
		if i == 0 {
			row.Fact(name, "8.0 KB")
		}
		row.Skipped(evo.Reason("in use"))
	}
	_ = out.Finish()
	_ = out.Close()
	return buf.String()
}

// TestSkippedItemFacts_StayBounded pins E-107/E-104: one Fact on one of 1000
// skipped items switched the verbose reason list from the bounded
// "a, b, c … +N more" to one unbounded line per item (1005 lines), and a
// value-only Fact rendered with a stray leading separator. Only the items
// with Facts get their own rows; the rest fold into the bounded list.
func TestSkippedItemFacts_StayBounded(t *testing.T) {
	got := manySkippedOneFactRun(t, 1000, "why")
	if lines := strings.Count(got, "\n"); lines > 12 {
		t.Errorf("verbose skipped list of 1000 items with one Fact is %d lines; want it bounded:\n%.600s", lines, got)
	}
	if !strings.Contains(got, "wt0000  why  8.0 KB") || !strings.Contains(got, "wt0001, wt0002, wt0003 … +996 more") {
		t.Errorf("want the Fact item's row and the bounded remainder:\n%.600s", got)
	}
	valueOnly := manySkippedOneFactRun(t, 3, "")
	if !strings.Contains(valueOnly, "wt0000  8.0 KB") || strings.Contains(valueOnly, "wt0000    8.0 KB") {
		t.Errorf("a value-only Fact carries a stray separator:\n%s", valueOnly)
	}
}
