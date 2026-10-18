package evo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/internal/change"
)

// The presentation contracts zq's prune needs from the renderer (Evident
// Output 1.x contract §3, §13, §15, §18): the renderer, not the caller,
// decides which rows deserve a line.

func newPlainOutput(buf *bytes.Buffer, dryRun bool) *Run {
	return initRun(Config{Name: "prune", DryRun: dryRun, Output: Plain, Facades: Facades{Stderr: buf}}, tuning{color: colorNever})
}

func machineDocument(t *testing.T, out *Run) string {
	t.Helper()
	var doc bytes.Buffer
	if err := writeJSON(&doc, Result{Conclusion: out.conclusion()}); err != nil {
		t.Fatal(err)
	}
	return doc.String()
}

func TestPruneContract_GroupWithoutOwnInformationKeepsItsHeaderRow(t *testing.T) {
	var buf bytes.Buffer
	out := newPlainOutput(&buf, false)
	t.Cleanup(func() { _ = out.close() })

	group := out.Group("categories")
	succeed(group.Task("branches"), "deleted 3")
	succeed(group.Task("worktrees"), "removed 1")
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}

	want := "✓ categories\n   ✓ branches   deleted 3\n   ✓ worktrees  removed 1\n"
	if got := buf.String(); !strings.HasPrefix(got, want) {
		t.Fatalf("group row must render with its Tasks nested; got:\n%q\nwant prefix:\n%q", got, want)
	}
	if doc := machineDocument(t, out); !strings.Contains(doc, `"categories"`) {
		t.Fatalf("machine output must keep the group; got:\n%s", doc)
	}
}

func TestPruneContract_GroupWithOwnSummaryKeepsItsHeaderRow(t *testing.T) {
	var buf bytes.Buffer
	out := newPlainOutput(&buf, false)
	t.Cleanup(func() { _ = out.close() })

	group := out.Group("categories").Summary("all clean")
	succeed(group.Task("branches"), "deleted 3")
	succeed(group.Task("worktrees"), "removed 1")
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}

	if got := buf.String(); !strings.Contains(got, "categories") || !strings.Contains(got, "all clean") {
		t.Fatalf("a Group with its own Summary keeps its row; got:\n%s", got)
	}
}

func TestPruneContract_ZeroInformationNoWorkRowIsHiddenFromHumanOutputOnly(t *testing.T) {
	var buf bytes.Buffer
	out := newPlainOutput(&buf, false)
	t.Cleanup(func() { _ = out.close() })

	group := out.Group("categories")
	satisfied(group.Task("worktrees"))
	succeed(group.Task("branches"), "deleted 3")
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	if strings.Contains(got, "worktrees") || !strings.Contains(got, "branches") {
		t.Fatalf("zero-information row must be suppressed and the informative row kept; got:\n%s", got)
	}
	if doc := machineDocument(t, out); !strings.Contains(doc, `"worktrees"`) {
		t.Fatalf("machine output keeps every row; got:\n%s", doc)
	}
}

func TestPruneContract_JSONLStreamKeepsEveryTaskAndTheGroup(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{Name: "prune", Output: JSONL, Facades: Facades{Stdout: &buf}})
	t.Cleanup(func() { _ = out.close() })

	group := out.Group("categories")
	succeed(group.Task("worktrees"))
	succeed(group.Task("branches"), "deleted 3")
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	linesNaming := map[string]int{}
	for _, line := range lines {
		if !json.Valid([]byte(line)) {
			t.Fatalf("JSONL line is not valid JSON: %q", line)
		}
		for _, name := range []string{"categories", "worktrees", "branches"} {
			if strings.Contains(line, `"`+name+`"`) {
				linesNaming[name]++
			}
		}
	}
	for _, name := range []string{"categories", "worktrees", "branches"} {
		if linesNaming[name] == 0 {
			t.Fatalf("JSONL must not drop %q; got %d lines:\n%s", name, len(lines), buf.String())
		}
	}
}

func TestPruneContract_RootTaskThatSimplyFinishesStaysALandmark(t *testing.T) {
	var buf bytes.Buffer
	out := newPlainOutput(&buf, false)
	t.Cleanup(func() { _ = out.close() })

	succeed(out.Task("compile"))
	succeed(out.Task("branches"), "deleted 3")
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}

	if got := buf.String(); !strings.Contains(got, "✓ compile") {
		t.Fatalf("a root Task is a landmark and keeps its row; got:\n%s", got)
	}
}

func TestPruneContract_AlreadySatisfiedRootRowIsHiddenWhenOtherContentExists(t *testing.T) {
	var buf bytes.Buffer
	out := newPlainOutput(&buf, false)
	t.Cleanup(func() { _ = out.close() })

	satisfied := out.Task("tools")
	satisfied.Verify(func(context.Context) (bool, error) { return true, nil })
	satisfied.Define(func(context.Context) error { return nil })
	if err := satisfied.wait(); err != nil {
		t.Fatal(err)
	}
	succeed(out.Task("branches"), "deleted 3")
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}

	if got := buf.String(); strings.Contains(got, "tools") || !strings.Contains(got, "branches") {
		t.Fatalf("a proven no-op root row is suppressed; got:\n%s", got)
	}
	if doc := machineDocument(t, out); !strings.Contains(doc, `"tools"`) {
		t.Fatalf("machine output keeps every row; got:\n%s", doc)
	}
}

func TestPruneContract_ZeroInformationRowsSurviveWhenNothingElseIsVisible(t *testing.T) {
	var buf bytes.Buffer
	out := newPlainOutput(&buf, false)
	t.Cleanup(func() { _ = out.close() })

	group := out.Group("categories")
	satisfied(group.Task("worktrees"))
	satisfied(group.Task("branches"))
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	if !strings.Contains(got, "worktrees") || !strings.Contains(got, "branches") {
		t.Fatalf("an all-no-op run keeps its rows; got:\n%s", got)
	}
}

func TestPruneContract_NoWorkRowThatRecordedEffectsIsNotZeroInformation(t *testing.T) {
	var buf bytes.Buffer
	out := newPlainOutput(&buf, false)
	t.Cleanup(func() { _ = out.close() })

	satisfied(out.Task("noop"))
	commit(out.Task("branches"), EffectSpec{Verb: Delete, Object: "stale branch", Quantity: 3})
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}

	if got := buf.String(); !strings.Contains(got, "✓ branches") {
		t.Fatalf("a task with effects keeps its row; got:\n%s", got)
	}
}

func TestPruneContract_ZeroInformationRowIsNeverHiddenOnFailure(t *testing.T) {
	var buf bytes.Buffer
	out := newPlainOutput(&buf, false)
	t.Cleanup(func() { _ = out.close() })

	satisfied(out.Task("worktrees"))
	out.Task("branches").fail("cannot delete")
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}

	if got := buf.String(); !strings.Contains(got, "worktrees") {
		t.Fatalf("a failed run keeps every row; got:\n%s", got)
	}
}

func TestPruneContract_LedgerFollowsTaskDeclarationOrderNotCompletionOrder(t *testing.T) {
	for _, dryRun := range []bool{true, false} {
		var buf bytes.Buffer
		out := newPlainOutput(&buf, dryRun)
		t.Cleanup(func() { _ = out.close() })

		branches := out.Task("branches")
		worktrees := out.Task("worktrees")
		remote := out.Task("remote-tracking")
		commit(remote, EffectSpec{Verb: Delete, Object: "stale origin/* branch", Quantity: 4})
		commit(worktrees, change.EffectSpec{Verb: change.EffectRemove, Object: "worktree", Quantity: 2})
		commit(branches, EffectSpec{Verb: Delete, Object: "local tip", Quantity: 40})
		if err := out.finish(); err != nil {
			t.Fatal(err)
		}

		var subjects []string
		for line := range strings.SplitSeq(buf.String(), "\n") {
			for _, tag := range []string{"[planned] ", "[changed] "} {
				if strings.HasPrefix(line, tag) && !strings.HasPrefix(line, tag+" ") {
					subjects = append(subjects, strings.Fields(strings.TrimPrefix(line, tag))[0])
				}
			}
		}
		want := []string{"branches", "worktrees", "remote-tracking"}
		if strings.Join(subjects, ",") != strings.Join(want, ",") {
			t.Fatalf("dryRun=%v: ledger subjects = %v, want declaration order %v; output:\n%s", dryRun, subjects, want, buf.String())
		}
	}
}

// TestPruneContract_LedgerSectionKeepsOnlyItsOwnTaskVisible pins that a
// section belongs to its Task, not its name: beta's no-op "prune" is hidden
// even though alpha's "prune" owns a [changed] section.
func TestPruneContract_LedgerSectionKeepsOnlyItsOwnTaskVisible(t *testing.T) {
	var buf bytes.Buffer
	out := newPlainOutput(&buf, false)
	t.Cleanup(func() { _ = out.close() })

	commit(out.Group("alpha").Task("prune"), EffectSpec{Verb: Delete, Object: "stale branch", Quantity: 3})
	satisfied(out.Group("beta").Task("prune"))
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}

	if got := buf.String(); strings.Count(got, "✓ prune") != 1 {
		t.Fatalf("only the Task owning the section keeps its no-op row; got:\n%s", got)
	}
}

// TestPruneContract_SameNamedRowsStayBareUnderTheirGroups pins that two
// failing "build" rows from two Groups need no qualification: each sits
// nested under its own Group's row, which already says where it is.
func TestPruneContract_SameNamedRowsStayBareUnderTheirGroups(t *testing.T) {
	var buf bytes.Buffer
	out := newPlainOutput(&buf, false)
	t.Cleanup(func() { _ = out.close() })

	for _, g := range []*Group{out.Group("g"), out.Group("gwith header").Summary("all built")} {
		g.Task("build").Define(func(context.Context) error { return errors.New("compile failed") })
		succeed(g.Task("ok"), "linked")
	}
	_ = out.finish()

	got := buf.String()
	for _, row := range []string{"✗ g\n   ✗ build", "✗ gwith header", "   ✓ ok"} {
		if !strings.Contains(got, row) {
			t.Fatalf("missing %q: same-named rows sit bare under their Group; got:\n%s", row, got)
		}
	}
	if strings.Contains(got, " › ") {
		t.Fatalf("rows under a Group row keep bare names; got:\n%s", got)
	}
}
