package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestReason_GetOrCreateMergesDuplicateNamesOnDefaultInstance is the red-first
// case for "duplicate strings merge into one bucket": two evo.Reason calls
// with the same name on the default instance must be the identical value,
// and a different name must not collide with it.
func TestReason_GetOrCreateMergesDuplicateNamesOnDefaultInstance(t *testing.T) {
	evo.SetDefault(evo.NewWithOptions(evo.Title("t"), evo.NoColor()))

	a := evo.Reason("protected")
	b := evo.Reason("protected")
	if a != b {
		t.Fatalf("evo.Reason(name) called twice must merge into one bucket: %+v vs %+v", a, b)
	}

	c := evo.Reason("dirty")
	if c == a {
		t.Fatal("a different reason name must not merge with an existing bucket")
	}
}

// TestTaskHandle_SkippedInlineReasonMergesByName is the "inline creation is
// always legal" case: constructing evo.Reason at each call site (no lifted
// var) must still merge into one taxonomy bucket by name.
func TestTaskHandle_SkippedInlineReasonMergesByName(t *testing.T) {
	var buf bytes.Buffer
	evo.SetDefault(evo.NewWithOptions(evo.To(&buf), evo.Plain(), evo.NoColor()))

	branches := evo.Task("branches")
	branches.Skipped(evo.Reason("protected"), "main")
	branches.Skipped(evo.Reason("protected"), "staging")
	branches.Done()

	if err := evo.Default().Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "skipped 2  (protected)") {
		t.Fatalf("inline evo.Reason calls must merge into one bucket, got:\n%s", got)
	}
}

// TestTaskHandle_SkippedPartitionSumsRendersCountsByReason is the red-first
// case for TAX-001: the reason partition is derived from the accumulated
// records, so parts mechanically sum to the headline count.
func TestTaskHandle_SkippedPartitionSumsRendersCountsByReason(t *testing.T) {
	var buf bytes.Buffer
	evo.SetDefault(evo.NewWithOptions(evo.To(&buf), evo.Plain(), evo.NoColor()))

	protected := evo.Reason("protected")
	dirty := evo.Reason("dirty")
	branches := evo.Task("branches")
	branches.Skipped(protected, "main")
	branches.Skipped(protected, "staging")
	branches.Skipped(dirty, "tip")
	branches.Done()

	if err := evo.Default().Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "skipped 3  (2 protected, 1 dirty)") {
		t.Fatalf("want derived partition line, got:\n%s", got)
	}
}

// TestTaskHandle_KeptSingleReasonCollapsesToBareName exercises the second
// disposition verb: same machinery as Skipped, and a single reason bucket
// collapses to its bare name since the count already says N.
func TestTaskHandle_KeptSingleReasonCollapsesToBareName(t *testing.T) {
	var buf bytes.Buffer
	evo.SetDefault(evo.NewWithOptions(evo.To(&buf), evo.Plain(), evo.NoColor()))

	unpushed := evo.Reason("unpushed")
	branches := evo.Task("branches")
	branches.Kept(unpushed, "feat/a")
	branches.Kept(unpushed, "feat/b")
	branches.Kept(unpushed, "feat/c")
	branches.Done()

	if err := evo.Default().Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "kept 3  (unpushed)") {
		t.Fatalf("want single-reason collapse, got:\n%s", got)
	}
}

// TestTaskHandle_SkippedVerboseEmitsTruncatedNameList is the red-first case
// for Verbose taxonomy detail: normal mode shows only counts; Verbose adds
// the bounded (TruncateNames) name list per reason.
func TestTaskHandle_SkippedVerboseEmitsTruncatedNameList(t *testing.T) {
	var buf bytes.Buffer
	evo.SetDefault(evo.New(evo.Config{
		Stdout: &buf, Stderr: &buf, Verbosity: evo.VerbosityVerbose,
		ForcePlain: true, Color: evo.ColorNever,
	}))

	protected := evo.Reason("protected")
	branches := evo.Task("branches")
	for _, name := range []string{"a", "b", "c", "d"} {
		branches.Skipped(protected, name)
	}
	branches.Done()

	if err := evo.Default().Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "skipped 4  (protected)") {
		t.Fatalf("want headline count line, got:\n%s", got)
	}
	if !strings.Contains(got, "protected: a, b, c, +1") {
		t.Fatalf("want Verbose truncated name list, got:\n%s", got)
	}
}

// TestTaskHandle_SkippedNonVerboseOmitsNameList pins the counterpart: without
// Verbose, only the count/partition line renders, never the raw name list.
func TestTaskHandle_SkippedNonVerboseOmitsNameList(t *testing.T) {
	var buf bytes.Buffer
	evo.SetDefault(evo.NewWithOptions(evo.To(&buf), evo.Plain(), evo.NoColor()))

	protected := evo.Reason("protected")
	branches := evo.Task("branches")
	branches.Skipped(protected, "main")
	branches.Done()

	if err := evo.Default().Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if strings.Contains(got, "protected: main") {
		t.Fatalf("non-verbose output must not include the per-reason name list:\n%s", got)
	}
}

// TestGroup_ChildRendersKeptTaxonomyLine is the red-first case for the
// repo-retire adoption gap: writeCollectionChild never called writeTaxonomy,
// so a Group/Tasks child's Kept/Skipped records silently vanished from
// rendered output even though the standalone evo.Task path rendered them.
// A collection child is a task; it must render the same "!  kept N  (...)"
// line a standalone task does.
func TestGroup_ChildRendersKeptTaxonomyLine(t *testing.T) {
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.To(&buf), evo.Plain(), evo.NoColor())

	unpushed := evo.Reason("unpushed")
	group := out.Group("branches")
	child := group.Task("feature-branches")
	child.Kept(unpushed, "feat/a")
	child.Kept(unpushed, "feat/b")
	child.Done()

	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "kept 2  (unpushed)") {
		t.Fatalf("collection child must render its Kept taxonomy line, got:\n%s", got)
	}
}

// TestGroup_ChildVerboseRendersTruncatedNameList pins the Verbose detail line
// for a collection child, mirroring TestTaskHandle_SkippedVerboseEmitsTruncatedNameList
// for the standalone path.
func TestGroup_ChildVerboseRendersTruncatedNameList(t *testing.T) {
	var buf bytes.Buffer
	out := evo.New(evo.Config{
		Stdout: &buf, Stderr: &buf, Verbosity: evo.VerbosityVerbose,
		ForcePlain: true, Color: evo.ColorNever,
	})

	protected := evo.Reason("protected")
	group := out.Group("branches")
	child := group.Task("stale-branches")
	for _, name := range []string{"a", "b", "c", "d"} {
		child.Skipped(protected, name)
	}
	child.Done()

	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "skipped 4  (protected)") {
		t.Fatalf("want headline count line for collection child, got:\n%s", got)
	}
	if !strings.Contains(got, "protected: a, b, c, +1") {
		t.Fatalf("want Verbose truncated name list for collection child, got:\n%s", got)
	}
}

// TestReason_ForSkipUsedViaKeptRecordsMisuseAndStillCounts is the red-first
// case for the ForSkip constraint: recording it through Kept is misuse, and
// production (non-Strict) still counts the record rather than dropping truth.
func TestReason_ForSkipUsedViaKeptRecordsMisuseAndStillCounts(t *testing.T) {
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.To(&buf), evo.Plain(), evo.NoColor())
	evo.SetDefault(out)
	skipOnly := evo.Reason("unpushed", evo.ForSkip())

	branches := out.Task("branches")
	branches.Kept(skipOnly, "feat/a")

	if out.Err() == nil {
		t.Fatal("want recorded misuse for a ForSkip reason recorded via Kept")
	}
	branches.Done()
	// Finish returns the recorded misuse (see ErrAlreadyResolved-style
	// contracts elsewhere); the assertion here is that the record still
	// rendered, not that Finish reports a clean run.
	_ = out.Finish()
	if !strings.Contains(buf.String(), "kept 1  (unpushed)") {
		t.Fatalf("misuse must still count the record, got:\n%s", buf.String())
	}
}

// TestReason_OnTaskWrongTaskPanicsUnderStrict is the red-first case for the
// OnTask constraint under Strict: a reason scoped to one task, recorded from
// a different task, panics instead of silently degrading.
func TestReason_OnTaskWrongTaskPanicsUnderStrict(t *testing.T) {
	out := evo.NewWithOptions(evo.Title("t"), evo.Strict(), evo.NoColor())
	evo.SetDefault(out)
	onlyBranches := evo.Reason("dirty", evo.OnTask("branches"))
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
		worktrees.Skipped(onlyBranches, "x")
	}()
}

// TestTaskHandle_SkippedDoesNotResolveTask pins "usable pre-resolution and
// does not resolve the task": after Skipped, the task is still Running, not
// terminal, so the caller can keep classifying before calling Done.
func TestTaskHandle_SkippedDoesNotResolveTask(t *testing.T) {
	out := evo.NewWithOptions(evo.Title("t"), evo.NoColor())
	evo.SetDefault(out)
	t.Cleanup(func() { _ = out.Close() })

	branches := out.Task("branches")
	branches.Skipped(evo.Reason("protected"), "main")

	if state := branches.Snapshot().State; state != evo.Running {
		t.Fatalf("Skipped must not resolve the task, state = %v", state)
	}
}

// TestTaskSnapshot_ExposesSkippedAndKeptTaxonomy pins the structural
// exposure requirement: Skipped/Kept live in TaskSnapshot (disposition side
// of the model), not the mutation ledger (Plan/Changes).
func TestTaskSnapshot_ExposesSkippedAndKeptTaxonomy(t *testing.T) {
	out := evo.NewWithOptions(evo.Title("t"), evo.NoColor())
	evo.SetDefault(out)
	t.Cleanup(func() { _ = out.Close() })

	reason := evo.Reason("protected")
	branches := out.Task("branches")
	branches.Skipped(reason, "main")
	branches.Kept(reason, "feat/a")

	snap := branches.Snapshot()
	if len(snap.Skipped) != 1 || snap.Skipped[0].Reason != "protected" || snap.Skipped[0].Name != "main" {
		t.Fatalf("Skipped taxonomy not exposed on snapshot: %+v", snap.Skipped)
	}
	if len(snap.Kept) != 1 || snap.Kept[0].Reason != "protected" || snap.Kept[0].Name != "feat/a" {
		t.Fatalf("Kept taxonomy not exposed on snapshot: %+v", snap.Kept)
	}
}
