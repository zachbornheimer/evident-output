package evo

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

// TestReason_GetOrCreateMergesDuplicateNamesOnDefaultInstance is the red-first
// case for "duplicate strings merge into one bucket": two Reason calls
// with the same name on the default instance must be the identical value,
// and a different name must not collide with it.
func TestReason_GetOrCreateMergesDuplicateNamesOnDefaultInstance(t *testing.T) {
	setDefault(initRun(Config{title: "t", color: colorNever}))

	a := reason("protected")
	b := reason("protected")
	if a != b {
		t.Fatalf("Reason(name) called twice must merge into one bucket: %+v vs %+v", a, b)
	}

	c := reason("dirty")
	if c == a {
		t.Fatal("a different reason name must not merge with an existing bucket")
	}
}

// TestTaskHandle_SkippedInlineReasonMergesByName,
// TestTaskHandle_SkippedPartitionSumsRendersCountsByReason,
// TestTaskHandle_KeptSingleReasonCollapsesToBareName, and
// TestTaskHandle_SkippedVerboseEmitsTruncatedNameList pinned Each's own
// collection-level taxonomy rollup (collectEachTaxonomy summed
// Skipped/Kept across every fromEach sibling into one "! skipped N (...)"
// line on the collection's row). 1.0 removed Each outright (§3.1: its
// get-or-create reliance is unsound) — a plain Group child now renders its
// own Skipped/Kept line individually (see TestTaskHandle_KeptSingleReason
// and TestTaskHandle_SkippedCauseRendersOneBoundedEvidenceLine below for
// the still-live per-task taxonomy path). Restoring an aggregated view for
// large homogeneous groups is renderer work for a later increment (§4:
// "aggregation is renderer-owned and automatic") — removed rather than
// pinning stale behavior. The per-task Skipped/Kept path itself is still
// live and covered below (TestTaskHandle_SkippedNonVerboseOmitsNameList
// and the Skipped-cause tests).

// TestTaskHandle_SkippedNonVerboseOmitsNameList pins the counterpart: without
// Verbose, only the count/partition line renders, never the raw name list.
func TestTaskHandle_SkippedNonVerboseOmitsNameList(t *testing.T) {
	var buf bytes.Buffer
	setDefault(initRun(Config{stdout: &buf, color: colorNever, plain: true}))

	protected := reason("protected")
	defaultRun().Task("branches").skipped(protected)

	if err := defaultRun().finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if strings.Contains(got, "protected: main") {
		t.Fatalf("non-verbose output must not include the per-reason name list:\n%s", got)
	}
}

// TestSequence_ChildRendersKeptTaxonomyLine is the red-first case for the
// repo-retire adoption gap: writeCollectionChild never called writeTaxonomy,
// so a Sequence/DisplayGroup child's Kept/Skipped records silently vanished from
// rendered output even though the standalone Task path rendered them.
// A collection child is a task; it must render the same "! kept N (...)"
// line a standalone task does. Each named child records its own reason
// count individually — Each's collection-level rollup across many
// same-shaped children was a distinct, separately-owned feature (removed
// in 1.0, §3.1) that this test never needed for its own regression guard.
func TestSequence_ChildRendersKeptTaxonomyLine(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, color: colorNever, plain: true})

	unpushed := reason("unpushed")
	group := out.Sequence("branches")
	group.Task("feat/a").skipped(unpushed)
	group.Task("feat/b").skipped(unpushed)

	if err := out.finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	got := buf.String()
	if strings.Count(got, "skipped 1 (unpushed)") != 2 {
		t.Fatalf("each collection child must render its own Skipped taxonomy line, got:\n%s", got)
	}
}

// TestSequence_ChildVerboseRendersTruncatedNameList pinned the Verbose
// truncated NAME LIST for a collection — "protected: a, b, c … +1 more" —
// summed by reason across four distinct fromEach sibling tasks. Skipped/Kept
// resolve their Task (see recordTaxonomy/finish above), so that shape has
// no one-task substitute: it was Each's own collection-level rollup
// (collectEachTaxonomy), removed outright in 1.0 (§3.1: get-or-create
// reliance is unsound). TestTaskHandle_SkippedCauseVerboseListsEveryCause
// below still covers the still-live per-task Verbose cause list.

// TestReason_ForSkipUsedViaKeptRecordsMisuseAndStillCounts is the red-first
// case for the ForSkip constraint: recording it through Kept is misuse, and
// production (non-Strict) still counts the record rather than dropping truth.
func TestReason_ForSkipUsedViaSkippedCountsWithoutMisuse(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, color: colorNever, plain: true})
	setDefault(out)
	skipOnly := reasonConstrained("unpushed", ForSkipForTest())

	branches := out.Task("branches")
	branches.skipped(skipOnly)

	if err := out.err(); err != nil {
		t.Fatalf("ForSkip used via Skipped is valid, got misuse %v", err)
	}
	succeed(branches)
	_ = out.finish()
	if !strings.Contains(buf.String(), "skipped 1 (unpushed)") {
		t.Fatalf("want the skipped tally, got:\n%s", buf.String())
	}
}

// TestReason_OnTaskWrongTaskPanicsUnderStrict is the red-first case for the
// OnTask constraint under Strict: a reason scoped to one task, recorded from
// a different task, panics instead of silently degrading.
func TestReason_OnTaskWrongTaskPanicsUnderStrict(t *testing.T) {
	out := initRun(Config{title: "t", color: colorNever, strict: true})
	setDefault(out)
	onlyBranches := reasonConstrained("dirty", OnTaskForTest("branches"))
	worktrees := out.Task("worktrees")

	// No t.Cleanup(out.Close): Strict re-panics on Finish for the
	// intentionally-left-unresolved task, which would escape as a second
	// panic after the assertion below already passed.
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Error("want panic under Strict for an OnTask constraint violation")
			}
		}()
		worktrees.skipped(onlyBranches)
	}()
}

// TestTaskHandle_SkippedDoesNotResolveTask inverts the pre-dialect
// "Skipped does not resolve" assumption: Skipped/Kept on an atomic Task
// IS the resolve (Group.Each is how many names accumulate).
func TestTaskHandle_SkippedDoesNotResolveTask(t *testing.T) {
	out := initRun(Config{title: "t", color: colorNever})
	setDefault(out)
	t.Cleanup(func() { _ = out.close() })

	branches := out.Task("branches")
	branches.skipped(reason("protected"))

	if state := branches.snapshot().State; state != excluded {
		t.Fatalf("Skipped must resolve the task, state = %v, want Skipped", state)
	}
}

// TestTaskHandle_SkippedCauseRendersOneBoundedEvidenceLine is the red-first
// case for the trailing errs on Skipped/Kept (item 2): the aggregation key
// (reason, name) is untouched by errs, and the causes render as one bounded
// └─ line under the count row — first cause plus "(+N more)" — never one
// line per record.
func TestTaskHandle_SkippedCauseRendersOneBoundedEvidenceLine(t *testing.T) {
	var buf bytes.Buffer
	setDefault(initRun(Config{stdout: &buf, color: colorNever, plain: true}))

	protected := reason("protected")
	branches := defaultRun().Task("branches")
	branches.skippedWithErrs(protected, "main", errors.New("required review"))
	branches.skippedWithErrs(protected, "staging", errors.New("required review"))
	succeed(branches)

	if err := defaultRun().finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "skipped 2 (protected)") {
		t.Fatalf("want headline count line, got:\n%s", got)
	}
	if !strings.Contains(got, "└─ required review (+1 more)") {
		t.Fatalf("want one bounded evidence line (first cause + N more), got:\n%s", got)
	}
}

// TestTaskHandle_SkippedCauseVerboseListsEveryCause pins the Verbose
// counterpart: every cause is listed, not just the first.
func TestTaskHandle_SkippedCauseVerboseListsEveryCause(t *testing.T) {
	var buf bytes.Buffer
	setDefault(initRun(Config{
		stdout: &buf, stderr: &buf, verbosity: verbosityVerbose,
		plain: true, color: colorNever,
	}))

	protected := reason("protected")
	branches := defaultRun().Task("branches")
	branches.skippedWithErrs(protected, "main", errors.New("cause one"))
	branches.skippedWithErrs(protected, "staging", errors.New("cause two"))
	succeed(branches)

	if err := defaultRun().finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "cause one") || !strings.Contains(got, "cause two") {
		t.Fatalf("want every cause listed under Verbose, got:\n%s", got)
	}
	if strings.Contains(got, "(+1 more)") {
		t.Fatalf("Verbose must list every cause, not the bounded summary, got:\n%s", got)
	}
}

// TestTaskHandle_SkippedNoCauseOmitsEvidenceLine pins the zero-errs
// backward-compatible path: no errs, no evidence line.
func TestTaskHandle_SkippedNoCauseOmitsEvidenceLine(t *testing.T) {
	var buf bytes.Buffer
	setDefault(initRun(Config{stdout: &buf, color: colorNever, plain: true}))

	branches := defaultRun().Task("branches")
	branches.skipped(reason("protected"))

	if err := defaultRun().finish(); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); strings.Contains(got, "└─") {
		t.Fatalf("no errs must render no evidence line, got:\n%s", got)
	}
}

// TestTaskSnapshot_ExposesSkippedAndKeptTaxonomy pins the structural
// exposure requirement: Skipped/Kept live in TaskSnapshot (disposition side
// of the model), not the mutation ledger (Plan/Changes).
func TestTaskSnapshot_ExposesSkippedAndKeptTaxonomy(t *testing.T) {
	out := initRun(Config{title: "t", color: colorNever})
	setDefault(out)
	t.Cleanup(func() { _ = out.close() })

	reason := reason("protected")
	skipped := out.Task("main")
	skipped.skipped(reason)
	kept := out.Task("feat/a")
	kept.skipped(reason)

	skipSnap := skipped.snapshot()
	if len(skipSnap.Skipped) != 1 || skipSnap.Skipped[0].Reason != "protected" || skipSnap.Skipped[0].Name != "main" {
		t.Fatalf("Skipped taxonomy not exposed on snapshot: %+v", skipSnap.Skipped)
	}
	keepSnap := kept.snapshot()
	if len(keepSnap.Skipped) != 1 || keepSnap.Skipped[0].Reason != "protected" || keepSnap.Skipped[0].Name != "feat/a" {
		t.Fatalf("Skipped taxonomy not exposed on snapshot: %+v", keepSnap.Skipped)
	}
}

// TestTaskHandle_SkippedTallyUsesSkipDetailGlyphNotWarning pins contract §41
// ("Warning | ! | [!]") and §20 ("Use a plain, widely-rendered `-` for an
// already-satisfied/skipped detail"): a Skipped tally is skip detail, not a
// warning, so it renders "-" and never feeds the "· warned" band — unlike a
// Kept tally, which §26/§27 render as "! kept N (...)".
func TestTaskHandle_SkippedTallyUsesSkipDetailGlyphNotWarning(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{isolated: true, stdout: &buf, color: colorNever, plain: true})
	t.Cleanup(func() { _ = out.close() })

	scan := out.Task("branches")
	scan.Define(func(context.Context) error {
		scan.skipped(reason("protected"))
		return nil
	})
	_ = scan.wait()
	succeed(out.Task("worktrees"), "3 checked")
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "- skipped 1 (protected)") || strings.Contains(got, "! skipped") {
		t.Fatalf("a Skipped tally renders the skip-detail dash, never the warning bang:\n%s", got)
	}
	if out.conclusion().warned || strings.Contains(got, "warned") {
		t.Fatalf("a Skipped tally must not feed the warned band:\n%s", got)
	}
}

// TestGroup_KeptChildrenAggregateUnderGroupRow pins contract §25 ("Rendering
// every child is not a correctness requirement; retaining every child in
// the model is") for per-item disposition children: a Group whose children
// only resolved Kept renders its own row plus one "! kept N (...)" tally,
// indented under it (§26/§27), never one row per item.
func TestGroup_KeptChildrenAggregateUnderGroupRow(t *testing.T) {
	for _, summary := range []string{"6 checked", ""} {
		var buf bytes.Buffer
		out := initRun(Config{isolated: true, stdout: &buf, color: colorNever, plain: true})
		t.Cleanup(func() { _ = out.close() })

		unpushed, protected := reason("unpushed"), reason("protected")
		branches := out.Group("branches")
		if summary != "" {
			branches.Summary(summary)
		}
		branches.Task("feat/a").skipped(unpushed)
		branches.Task("main").skipped(protected)
		branches.Task("feat/b").skipped(unpushed)
		if err := out.finish(); err != nil {
			t.Fatal(err)
		}

		row := "✓ branches\n"
		if summary != "" {
			row = "✓ branches  " + summary + "\n"
		}
		want := row + "  - skipped 3 (2 unpushed, 1 protected)\n" + "\n[excluded]\n"
		if got := buf.String(); got != want {
			t.Fatalf("summary %q mismatch:\n--- want ---\n%s\n--- got ---\n%s", summary, want, got)
		}
	}
}
