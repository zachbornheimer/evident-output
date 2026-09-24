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

// keptItem is one item a prune category keeps, and why.
type keptItem struct {
	name   string
	reason evo.TaxonomyReason
}

// pruneCategory is one zq prune category in the contract-correct per-item
// shape: a Group named for the category holding the category's own Task
// (same name: it classifies, summarizes, and owns the Effect, so the
// ledger subject is the category — docs/reference.md "own Task") plus one
// child Task per kept item that resolves Kept (the item is the Task).
type pruneCategory struct {
	name, summary string
	effect        *evo.EffectSpec
	onDisk        string // routine Fact, verbose-only (§13, §21); "" for none
	kept          []keptItem
}

// declare submits the category under parent and returns its own Task.
// Every annotation happens inside Define, before the Task resolves.
func (c pruneCategory) declare(parent *evo.GroupHandle) *evo.TaskHandle {
	items := parent.Group(c.name)
	work := items.Task(c.name)
	work.Define(func(ctx context.Context) error {
		for _, item := range c.kept {
			items.Task(item.name).Kept(item.reason)
		}
		if c.onDisk != "" {
			work.Fact("on disk", c.onDisk)
		}
		work.Summary(c.summary)
		if c.effect == nil {
			return nil
		}
		return evo.Effect(ctx, *c.effect, func(context.Context) error { return nil })
	})
	return work
}

// renderPruneContract18 runs zq prune's dry-run under zq's own Config
// (Title "zq", a Subject header) at verbosity.
func renderPruneContract18(t *testing.T, verbosity evo.Verbosity) string {
	t.Helper()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true, DryRun: true, Color: evo.ColorNever, Plain: true, Verbosity: verbosity,
		Title: "zq", Subject: "zq prune  ~/repo", Stdout: &buf,
	})
	t.Cleanup(func() { _ = out.Close() })

	categories := out.Group("categories")
	checkedOut, protected := evo.Reason("checked out"), evo.Reason("protected")
	dirty, unpushed := evo.Reason("dirty"), evo.Reason("unpushed")
	branches := pruneCategory{
		name: "branches", summary: "188 checked",
		effect: &evo.EffectSpec{Verb: evo.EffectDelete, Object: "local tip", Quantity: 87},
		kept:   []keptItem{{"feat/wt-a", checkedOut}, {"feat/wt-b", checkedOut}, {"main", protected}},
	}.declare(categories)
	worktrees := pruneCategory{
		name: "worktrees", summary: "168 checked", onDisk: "508.8 MB",
		effect: &evo.EffectSpec{Verb: evo.EffectRemove, Object: "worktree", Quantity: 95},
		kept:   []keptItem{{"../wt-a", dirty}, {"../wt-b", dirty}, {"../wt-c", unpushed}},
	}.declare(categories)
	remotes := pruneCategory{name: "remote-tracking", summary: "nothing to clean"}.declare(categories)
	for _, category := range []*evo.TaskHandle{branches, worktrees, remotes} {
		if err := category.Wait(); err != nil {
			t.Fatal(err)
		}
	}
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// TestPruneContract_KeptUnderGroupedCategoriesRendersContract18 holds zq
// prune's contract-correct per-item shape (pruneCategory) to the contract
// §18 dry-run bytes TestV8_DryRunPlanOnly pins for the Warn-authored form.
// Each category Group's kept children aggregate into one tally under the
// category's row (§25: "aggregation is a renderer concern"; §26/§27:
// "  ! kept N (...)"), the tally feeds "[planned · warned]", and the band
// stays bare because the dry-run Subject header already named the run.
func TestPruneContract_KeptUnderGroupedCategoriesRendersContract18(t *testing.T) {
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
	if got := renderPruneContract18(t, evo.VerbosityNormal); got != want {
		t.Fatalf("mismatch:\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

// TestPruneContract_KeptTallyVerboseListsRealItemNames is the --verbose
// half: the aggregated tally lists each kept child's own name under its
// reason, and the routine "on disk" Fact appears.
func TestPruneContract_KeptTallyVerboseListsRealItemNames(t *testing.T) {
	got := renderPruneContract18(t, evo.VerbosityVerbose)
	for _, want := range []string{
		"✓ branches         188 checked\n  ! kept 3 (2 checked out, 1 protected)\n",
		"checked out: feat/wt-a, feat/wt-b\n",
		"protected: main\n",
		"dirty: ../wt-a, ../wt-b\n",
		"unpushed: ../wt-c\n",
		"on disk  508.8 MB\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("verbose output lacks %q:\n%s", want, got)
		}
	}
}

// TestPruneContract_KeptChildrenStayInMachineOutput proves the aggregation
// is human-only: JSON keeps every kept child Task.
func TestPruneContract_KeptChildrenStayInMachineOutput(t *testing.T) {
	var buf bytes.Buffer
	out := newPlainOutput(&buf, true)
	t.Cleanup(func() { _ = out.Close() })
	work := pruneCategory{
		name: "branches", summary: "2 checked",
		kept: []keptItem{{"feat/a", evo.Reason("unpushed")}, {"main", evo.Reason("protected")}},
	}.declare(out.Group("categories"))
	if err := work.Wait(); err != nil {
		t.Fatal(err)
	}
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "feat/a") {
		t.Fatalf("human output must aggregate kept children into the tally:\n%s", buf.String())
	}
	if doc := machineDocument(t, out); !strings.Contains(doc, `"feat/a"`) || !strings.Contains(doc, `"main"`) {
		t.Fatalf("machine output keeps every kept child:\n%s", doc)
	}
}
