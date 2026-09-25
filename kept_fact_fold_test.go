package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// keptWithFactsRun is zq prune's in-use shape: one Task per kept item,
// each with a Fact saying why (row := group.Task(name); row.Fact("why",
// d); row.Kept(r)).
func keptWithFactsRun(t *testing.T, v evo.Verbosity) string {
	t.Helper()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, StateDir: t.TempDir(), Stdout: &buf, Title: "prune", Color: evo.ColorNever, Plain: true, Verbosity: v})
	g := out.Group("worktrees")
	for _, n := range []string{"feat1", "a-much-longer-worktree-name", "main"} {
		row := g.Task(n)
		if n != "main" {
			row.Fact("why", "current checkout")
		}
		row.Kept(evo.Reason("in use"))
	}
	_ = out.Finish()
	_ = out.Close()
	return buf.String()
}

// TestKeptItemFacts_KeepTheFold pins E-100: a Fact on a kept child broke
// the kept fold under verbose, so each child rendered its own "✓ name
// why …" row and "! kept 1 (…)". The fold holds; verbose lists each item
// once with its Facts at one column, whatever the name width.
func TestKeptItemFacts_KeepTheFold(t *testing.T) {
	for _, v := range []evo.Verbosity{evo.VerbosityNormal, evo.VerbosityVerbose} {
		got := keptWithFactsRun(t, v)
		if strings.Count(got, "! kept") != 1 || !strings.Contains(got, "! kept 3 (in use)") {
			t.Errorf("verbosity %d: want one \"! kept 3 (in use)\" tally:\n%s", v, got)
		}
		if strings.Contains(got, "✓ feat1") || strings.Contains(got, "✓ main") {
			t.Errorf("verbosity %d: a kept item got its own ✓ row:\n%s", v, got)
		}
	}
	got := keptWithFactsRun(t, evo.VerbosityVerbose)
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
