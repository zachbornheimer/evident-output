package evo_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// The presentation contracts zq's prune needs from the renderer (Evident
// Output 1.x contract §3, §13, §15, §18): the renderer, not the caller,
// decides which rows deserve a line.

func newPlainOutput(buf *bytes.Buffer, dryRun bool) *evo.Output {
	return evo.Init(evo.Config{Isolated: true, Stdout: buf, Title: "prune", Color: evo.ColorNever, Plain: true, DryRun: dryRun})
}

func machineDocument(t *testing.T, out *evo.Output) string {
	t.Helper()
	var doc bytes.Buffer
	if err := evo.WriteJSON(&doc, evo.Result{Conclusion: out.Conclusion()}); err != nil {
		t.Fatal(err)
	}
	return doc.String()
}

func TestPruneContract_GroupWithoutOwnInformationRendersNoHeaderRow(t *testing.T) {
	var buf bytes.Buffer
	out := newPlainOutput(&buf, false)
	t.Cleanup(func() { _ = out.Close() })

	group := out.Group("categories")
	succeed(group.Task("branches"), "deleted 3")
	succeed(group.Task("worktrees"), "removed 1")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	want := "✓ branches   deleted 3\n✓ worktrees  removed 1\n"
	if got := buf.String(); !strings.HasPrefix(got, want) || strings.Contains(got, "categories") {
		t.Fatalf("group header must not render; got:\n%q\nwant prefix:\n%q", got, want)
	}
	if doc := machineDocument(t, out); !strings.Contains(doc, `"categories"`) {
		t.Fatalf("machine output must keep the group; got:\n%s", doc)
	}
}

func TestPruneContract_GroupWithOwnSummaryKeepsItsHeaderRow(t *testing.T) {
	var buf bytes.Buffer
	out := newPlainOutput(&buf, false)
	t.Cleanup(func() { _ = out.Close() })

	group := out.Group("categories").Summary("all clean")
	succeed(group.Task("branches"), "deleted 3")
	succeed(group.Task("worktrees"), "removed 1")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	if got := buf.String(); !strings.Contains(got, "categories") || !strings.Contains(got, "all clean") {
		t.Fatalf("a Group with its own Summary keeps its row; got:\n%s", got)
	}
}

func TestPruneContract_ZeroInformationNoWorkRowIsHiddenFromHumanOutputOnly(t *testing.T) {
	var buf bytes.Buffer
	out := newPlainOutput(&buf, false)
	t.Cleanup(func() { _ = out.Close() })

	group := out.Group("categories")
	satisfied(group.Task("worktrees"))
	succeed(group.Task("branches"), "deleted 3")
	if err := out.Finish(); err != nil {
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
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Title: "prune", Format: evo.FormatJSONL})
	t.Cleanup(func() { _ = out.Close() })

	group := out.Group("categories")
	succeed(group.Task("worktrees"))
	succeed(group.Task("branches"), "deleted 3")
	if err := out.Finish(); err != nil {
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
	t.Cleanup(func() { _ = out.Close() })

	succeed(out.Task("compile"))
	succeed(out.Task("branches"), "deleted 3")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	if got := buf.String(); !strings.Contains(got, "✓ compile") {
		t.Fatalf("a root Task is a landmark and keeps its row; got:\n%s", got)
	}
}

func TestPruneContract_AlreadySatisfiedRootRowIsHiddenWhenOtherContentExists(t *testing.T) {
	var buf bytes.Buffer
	out := newPlainOutput(&buf, false)
	t.Cleanup(func() { _ = out.Close() })

	satisfied := out.Task("tools")
	satisfied.Verify(func(context.Context) (bool, error) { return true, nil })
	satisfied.Define(func(context.Context) error { return nil })
	if err := satisfied.Wait(); err != nil {
		t.Fatal(err)
	}
	succeed(out.Task("branches"), "deleted 3")
	if err := out.Finish(); err != nil {
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
	t.Cleanup(func() { _ = out.Close() })

	group := out.Group("categories")
	satisfied(group.Task("worktrees"))
	satisfied(group.Task("branches"))
	if err := out.Finish(); err != nil {
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
	t.Cleanup(func() { _ = out.Close() })

	satisfied(out.Task("noop"))
	commit(out.Task("branches"), evo.EffectSpec{Verb: evo.EffectDelete, Object: "stale branch", Quantity: 3})
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	if got := buf.String(); !strings.Contains(got, "✓ branches") {
		t.Fatalf("a task with effects keeps its row; got:\n%s", got)
	}
}

func TestPruneContract_ZeroInformationRowIsNeverHiddenOnFailure(t *testing.T) {
	var buf bytes.Buffer
	out := newPlainOutput(&buf, false)
	t.Cleanup(func() { _ = out.Close() })

	satisfied(out.Task("worktrees"))
	out.Task("branches").Fail("cannot delete")
	if err := out.Finish(); err != nil {
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
		t.Cleanup(func() { _ = out.Close() })

		branches := out.Task("branches")
		worktrees := out.Task("worktrees")
		remote := out.Task("remote-tracking")
		commit(remote, evo.EffectSpec{Verb: evo.EffectDelete, Object: "stale origin/* branch", Quantity: 4})
		commit(worktrees, evo.EffectSpec{Verb: evo.EffectRemove, Object: "worktree", Quantity: 2})
		commit(branches, evo.EffectSpec{Verb: evo.EffectDelete, Object: "local tip", Quantity: 40})
		if err := out.Finish(); err != nil {
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

// TestPruneContract_KeptUnderGroupedCategoriesRendersContract18 is zq
// prune's real 1.1 shape (a header-less Group of category Tasks, each
// accumulating Kept records inside its Define before its Effect), held to
// the contract §18 dry-run bytes TestV8_DryRunPlanOnly pins for the
// Warn-authored form: a Kept tally nests under its row ("  ! kept N (...)",
// spec §26/§27) exactly like a Warn, and its "!" row feeds the trailing
// "[planned · warned]" band like any other warning-glyph annotation.
func TestPruneContract_KeptUnderGroupedCategoriesRendersContract18(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true, DryRun: true, Color: evo.ColorNever, Plain: true,
		Subject: "zq prune  ~/repo", Stdout: &buf,
	})
	t.Cleanup(func() { _ = out.Close() })

	categories := out.Group("categories")
	branches := categories.Task("branches")
	worktrees := categories.Task("worktrees")
	remotes := categories.Task("remote-tracking")

	keepAll := func(task *evo.TaskHandle, summary string, spec evo.EffectSpec, reasons ...evo.TaxonomyReason) {
		task.Define(func(ctx context.Context) error {
			for _, reason := range reasons {
				task.Kept(reason)
			}
			task.Summary(summary)
			return evo.Effect(ctx, spec, func(context.Context) error { return nil })
		})
	}
	remotes.Define(func(context.Context) error {
		remotes.Summary("nothing to clean")
		return nil
	})
	keepAll(worktrees, "168 checked",
		evo.EffectSpec{Verb: evo.EffectRemove, Object: "worktree", Quantity: 95},
		evo.Reason("dirty"), evo.Reason("dirty"), evo.Reason("unpushed"))
	branches.After(worktrees, remotes)
	keepAll(branches, "188 checked",
		evo.EffectSpec{Verb: evo.EffectDelete, Object: "local tip", Quantity: 87},
		evo.Reason("checked out"), evo.Reason("checked out"), evo.Reason("protected"))
	if err := branches.Wait(); err != nil {
		t.Fatal(err)
	}
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	want := "[dry-run] zq prune  ~/repo\n" +
		"\n" +
		"✓ branches         188 checked\n" +
		"  ! kept 3 (2 checked out, 1 protected)\n" +
		"✓ worktrees        168 checked\n" +
		"  ! kept 3 (2 dirty, 1 unpushed)\n" +
		"✓ remote-tracking  nothing to clean\n" +
		"\n" +
		"[planned] branches   delete 87 local tips\n" +
		"[planned] worktrees  remove 95 worktrees\n" +
		"\n" +
		"[planned · warned]\n"
	if got := buf.String(); got != want {
		t.Fatalf("mismatch:\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}
