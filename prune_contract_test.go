package evo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// The presentation contracts zq's prune needs from the renderer (Evident
// Output 1.x contract §3, §13, §15, §18): the renderer, not the caller,
// decides which rows deserve a line.

func newPlainOutput(buf *bytes.Buffer, dryRun bool) *Run {
	return initRun(Config{isolated: true, stdout: buf, title: "prune", color: colorNever, plain: true, DryRun: dryRun})
}

func machineDocument(t *testing.T, out *Run) string {
	t.Helper()
	var doc bytes.Buffer
	if err := writeJSON(&doc, Result{Conclusion: out.conclusion()}); err != nil {
		t.Fatal(err)
	}
	return doc.String()
}

func TestPruneContract_GroupWithoutOwnInformationRendersNoHeaderRow(t *testing.T) {
	var buf bytes.Buffer
	out := newPlainOutput(&buf, false)
	t.Cleanup(func() { _ = out.close() })

	group := out.Group("categories")
	succeed(group.Task("branches"), "deleted 3")
	succeed(group.Task("worktrees"), "removed 1")
	if err := out.finish(); err != nil {
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
	out := initRun(Config{isolated: true, stdout: &buf, title: "prune", format: formatJSONL})
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
		commit(worktrees, EffectSpec{Verb: Verb(effectRemove), Object: "worktree", Quantity: 2})
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

// keptItem is one item a prune category keeps, and why.
type keptItem struct {
	name   string
	reason taxonomyReason
}

// pruneCategory is one zq prune category in the contract-correct per-item
// shape: a Group named for the category holding the category's own Task
// (same name: it classifies, summarizes, and owns the Effect, so the
// ledger subject is the category — mcp/docs/reference.md "own Task") plus one
// child Task per kept item that resolves Kept (the item is the Task).
type pruneCategory struct {
	name, summary string
	effect        *EffectSpec
	onDisk        string // routine Fact, verbose-only (§13, §21); "" for none
	kept          []keptItem
}

// declare submits the category under parent and returns its own Task.
// Kept children are declared up front; annotations happen inside Define.
func (c pruneCategory) declare(parent *Group) *Task {
	items := parent.Group(c.name)
	work := items.Task(c.name)
	for _, item := range c.kept {
		items.Task(item.name).skipped(item.reason)
	}
	work.Define(func(ctx context.Context) error {
		if c.onDisk != "" {
			work.Fact("on disk", c.onDisk)
		}
		work.Summary(c.summary)
		if c.effect == nil {
			return nil
		}
		return Effect(ctx, *c.effect, func(context.Context) error { return nil })
	})
	return work
}

// renderPruneContract18 runs zq prune's dry-run under zq's own Config
// (Title "zq", a Subject header) at verbosity.
func renderPruneContract18(t *testing.T, verbosity verbosity) string {
	t.Helper()
	var buf bytes.Buffer
	out := initRun(Config{
		isolated: true, DryRun: true, color: colorNever, plain: true, verbosity: verbosity,
		title: "zq", subject: "zq prune  ~/repo", stdout: &buf,
	})
	t.Cleanup(func() { _ = out.close() })

	categories := out.Group("categories")
	checkedOut, protected := reason("checked out"), reason("protected")
	dirty, unpushed := reason("dirty"), reason("unpushed")
	branches := pruneCategory{
		name: "branches", summary: "188 checked",
		effect: &EffectSpec{Verb: Delete, Object: "local tip", Quantity: 87},
		kept:   []keptItem{{"feat/wt-a", checkedOut}, {"feat/wt-b", checkedOut}, {"main", protected}},
	}.declare(categories)
	worktrees := pruneCategory{
		name: "worktrees", summary: "168 checked", onDisk: "508.8 MB",
		effect: &EffectSpec{Verb: Verb(effectRemove), Object: "worktree", Quantity: 95},
		kept:   []keptItem{{"../wt-a", dirty}, {"../wt-b", dirty}, {"../wt-c", unpushed}},
	}.declare(categories)
	remotes := pruneCategory{name: "remote-tracking", summary: "nothing to clean"}.declare(categories)
	for _, category := range []*Task{branches, worktrees, remotes} {
		if err := category.wait(); err != nil {
			t.Fatal(err)
		}
	}
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// TestPruneContract_KeptUnderGroupedCategoriesRendersContract18 holds zq
// prune's contract-correct per-item shape (pruneCategory) to the contract
// §18 dry-run bytes TestV8_DryRunPlanOnly pins for the Warn-authored form.
// Each category Group's kept children aggregate into one tally under the
// category's row (§25: "aggregation is a renderer concern"; §26/§27:
// "  ! kept N (...)"), the tally is one skipped fold under the category row, and the trailing
// conclusion band is omitted because the planned rows already named the outcome.
func TestPruneContract_KeptUnderGroupedCategoriesRendersContract18(t *testing.T) {
	want := "[dry-run] zq prune  ~/repo\n" +
		"\n" +
		"✓ branches         188 checked\n" +
		"  - skipped 3 (2 checked out, 1 protected)\n" +
		"✓ worktrees        168 checked\n" +
		"  - skipped 3 (2 dirty, 1 unpushed)\n" +
		"✓ remote-tracking  nothing to clean\n" +
		"\n" +
		"[planned] branches   delete 87 local tips\n" +
		"[planned] worktrees  remove 95 worktrees\n"
	if got := renderPruneContract18(t, verbosityNormal); got != want {
		t.Fatalf("mismatch:\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

// TestPruneContract_KeptTallyVerboseListsRealItemNames is the --verbose
// half: the aggregated tally lists each kept child's own name under its
// reason, and the routine "on disk" Fact appears.
func TestPruneContract_KeptTallyVerboseListsRealItemNames(t *testing.T) {
	got := renderPruneContract18(t, verbosityVerbose)
	for _, want := range []string{
		"✓ branches         188 checked\n  - skipped 3 (2 checked out, 1 protected)\n",
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
	t.Cleanup(func() { _ = out.close() })
	work := pruneCategory{
		name: "branches", summary: "2 checked",
		kept: []keptItem{{"feat/a", reason("unpushed")}, {"main", reason("protected")}},
	}.declare(out.Group("categories"))
	if err := work.wait(); err != nil {
		t.Fatal(err)
	}
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "feat/a") {
		t.Fatalf("human output must aggregate kept children into the tally:\n%s", buf.String())
	}
	if doc := machineDocument(t, out); !strings.Contains(doc, `"feat/a"`) || !strings.Contains(doc, `"main"`) {
		t.Fatalf("machine output keeps every kept child:\n%s", doc)
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

// TestPruneContract_FlattenedSameNamedRowsNameTheirGroup pins that a
// header-less Group's rows never read as identical siblings: two failing
// "build" rows from two Groups each name their container, the same way
// their ledger sections would ("g › build").
func TestPruneContract_FlattenedSameNamedRowsNameTheirGroup(t *testing.T) {
	var buf bytes.Buffer
	out := newPlainOutput(&buf, false)
	t.Cleanup(func() { _ = out.close() })

	for _, g := range []*Group{out.Group("g"), out.Group("gwith header").Summary("all built")} {
		g.Task("build").Define(func(context.Context) error { return errors.New("compile failed") })
		succeed(g.Task("ok"), "linked")
	}
	_ = out.finish()

	got := buf.String()
	for _, row := range []string{"✗ g › build", "✓ g › ok", "✗ gwith header › build", "✓ gwith header › ok"} {
		if !strings.Contains(got, row) {
			t.Fatalf("missing %q: same-named rows must name their Group; got:\n%s", row, got)
		}
	}
}

// TestPruneContract_FlattenedUniqueRowsKeepBareNames: qualification is for
// collisions only; a unique name stays bare.
func TestPruneContract_FlattenedUniqueRowsKeepBareNames(t *testing.T) {
	var buf bytes.Buffer
	out := newPlainOutput(&buf, false)
	t.Cleanup(func() { _ = out.close() })

	succeed(out.Group("a").Task("branches"), "deleted 3")
	succeed(out.Group("b").Task("worktrees"), "removed 1")
	_ = out.finish()

	if got := buf.String(); strings.Contains(got, " › ") {
		t.Fatalf("unique rows keep their bare names; got:\n%s", got)
	}
}
