package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// This file closes the remaining evo-rec.md "Recommended UI" golden cells for
// Problems 16-21 (narrow-terminal compact layout, skip/keep taxonomy,
// remote-tracking vs remote-delete separation, VisibilityDelay first paint,
// stale-phase heartbeat, and durable Println vs the live region). Problem
// numbering follows spec_golden_test.go's convention: the Nth "## Problem:"
// heading in ~/Desktop/evo-rec.md, top to bottom. Problem 16's step1 and
// indeterminate cells (narrow-terminal live spinner frames) are already
// closed by TestSpecP16_LiveFrame_NarrowTerminal_Mismatch in
// spec_golden_live_test.go and are out of scope here (a fix agent owns
// live/resize frames for this problem). Problem 18's step1/step2/success
// cells are already closed in spec_golden_test.go.
//
// collapsed(buf) collapses run-length whitespace/newlines to single spaces so
// assertions read the same shape as the doc's fenced blocks without caring
// about exact column alignment — the same technique spec_golden_test.go
// uses.
func collapsed(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// ---------------------------------------------------------------------
// Problem 16 — narrow terminals (<40 cols) compact layout.
// ---------------------------------------------------------------------

// TestSpecP16_CompactLayout_Step2 covers Problem 16's step2 block: a
// [changed] section under 40 columns drops leaders and prints bare
// "verb qty object" rows (writeEffects's width<compactLayoutMaxWidth
// branch).
//
//	[changed] clean
//	  deleted 3 local
//	  pruned 2 stale
func TestSpecP16_CompactLayout_Step2(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor(), evo.Width(30)}})
	clean := out.Task("clean")
	clean.Record("delete", 3, "local")
	clean.Record("prune", 2, "stale")
	clean.Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := collapsed(buf.String())
	for _, want := range []string{"[changed] clean", "deleted 3 local", "pruned 2 stale"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, buf.String())
		}
	}
}

// TestSpecP16_CompactLayout_Success covers Problem 16's success block: two
// Done task rows with caller-abbreviated summaries plus a single-reason
// skip taxonomy line.
//
//	✓ branches 14 del
//	✓ worktrees 2 rm
//	! skipped 6  (protected)
//
// writeTaxonomy (plain.go) always appends "(<reason>)", even for a single
// reason — every skip/keep taxonomy row includes its reason(s) in
// parentheses by design; there is no public spelling that omits it.
func TestSpecP16_CompactLayout_Success(t *testing.T) {
	// Not t.Parallel(): evo.SetDefault/evo.Reason mutate process-global state.
	var buf bytes.Buffer
	evo.SetDefault(evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor(), evo.Width(30)}}))
	out := evo.Default()
	branches := out.Task("branches")
	protected := evo.Reason("protected")
	for i := 0; i < 6; i++ {
		branches.Skipped(protected, "feat/"+string(rune('a'+i)))
	}
	branches.Done("14 del")
	worktrees := out.Task("worktrees")
	worktrees.Done("2 rm")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{"✓  branches  14 del", "✓  worktrees  2 rm"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "!  skipped 6  (protected)") {
		t.Fatalf("want the real (parenthesized-reason) taxonomy line, got:\n%s", got)
	}
}

// TestSpecP16_CompactLayout_Failure covers Problem 16's failure block: a
// Fail with Detail renders the task's terse cause on its own row and the
// evidence on a connected line, no duplicate.
//
//	✗ remotes auth
//	  └─ 401 token
func TestSpecP16_CompactLayout_Failure(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor(), evo.Width(30)}})
	remotes := out.Task("remotes")
	remotes.Fail("auth", evo.Detail("401 token"))
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := collapsed(buf.String())
	for _, want := range []string{"✗ remotes auth", "401 token"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, buf.String())
		}
	}
}

// TestSpecP16_CompactLayout_Error covers Problem 16's error block: a bare
// failed task row (no caller-supplied cause text) with the only evidence on
// the connected detail line.
//
//	✗ branches
//	  └─ lock ref
func TestSpecP16_CompactLayout_Error(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor(), evo.Width(30)}})
	branches := out.Task("branches")
	branches.Fail("", evo.Detail("lock ref"))
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := collapsed(buf.String())
	for _, want := range []string{"✗ branches", "lock ref"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, buf.String())
		}
	}
}

// TestSpecP16_CompactLayout_EarlyTermination covers Problem 16's early
// termination block: a committed delete survives a later Cancel on a
// sibling task, and the derived already-mutated line summarizes the ledger.
//
//	✓ branches 3 del
//	■ remotes
//	! already mutated: 3 local deleted
//
// The derived line is always "!  already mutated: <N> <object> <verb>"
// (writeAlreadyMutated / summarizeChangeSection, plain.go:485-539). No
// public option suppresses the "already" prefix or the trailing verb.
func TestSpecP16_CompactLayout_EarlyTermination(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor(), evo.Width(30)}})
	branches := out.Task("branches")
	branches.Record("delete", 3, "local")
	branches.Done("3 del")
	remotes := out.Task("remotes")
	remotes.Cancel("")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := collapsed(buf.String())
	for _, want := range []string{"✓ branches 3 del", "■ remotes"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, buf.String())
		}
	}
	if !strings.Contains(got, "already mutated: 3 local deleted") {
		t.Fatalf("want the real derived already-mutated line, got:\n%s", buf.String())
	}
}

// ---------------------------------------------------------------------
// Problem 17 — skip/keep taxonomy needs a reason breakdown.
// ---------------------------------------------------------------------

// TestSpecP17_Taxonomy_Step1 covers Problem 17's step1 block: a live
// determinate-progress row for a task that will accumulate taxonomy.
//
//	:.  branches  10/40  feat/tmp
func TestSpecP17_Taxonomy_Step1(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	branches := out.Task("branches")
	branches.Progress(10, 40)
	branches.Phase("feat/tmp")

	got := screen.LatestLiveText()
	for _, want := range []string{"branches", "10/40", "feat/tmp"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in live frame:\n%s", want, got)
		}
	}
}

// TestSpecP17_Taxonomy_Step2 covers Problem 17's step2 block: a Done task
// with a multi-reason skip partition and a single-reason keep partition,
// each derived (never caller-assembled) from accumulated records.
//
//	✓  branches  14 deleted
//	!  skipped 6  (4 protected, 2 dirty)
//	!  kept 3     (unpushed)
func TestSpecP17_Taxonomy_Step2(t *testing.T) {
	// Not t.Parallel(): evo.SetDefault/evo.Reason mutate process-global state.
	var buf bytes.Buffer
	evo.SetDefault(evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}}))
	out := evo.Default()
	branches := out.Task("branches")
	protected := evo.Reason("protected")
	dirty := evo.Reason("dirty")
	unpushed := evo.Reason("unpushed")
	for i := 0; i < 4; i++ {
		branches.Skipped(protected, "p"+string(rune('a'+i)))
	}
	for i := 0; i < 2; i++ {
		branches.Skipped(dirty, "d"+string(rune('a'+i)))
	}
	for i := 0; i < 3; i++ {
		branches.Kept(unpushed, "k"+string(rune('a'+i)))
	}
	branches.Done("14 deleted")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := collapsed(buf.String())
	for _, want := range []string{
		"✓ branches 14 deleted",
		"! skipped 6 (4 protected, 2 dirty)",
		"! kept 3 (unpushed)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, buf.String())
		}
	}
}

// TestSpecP17_Taxonomy_Success covers Problem 17's success block: the same
// step2 taxonomy plus a next-action row.
//
//	✓  branches  14 deleted
//	!  skipped 6  (4 protected, 2 dirty)
//	!  kept 3     (unpushed)
//	→  repo-retire salvage --dry-run
func TestSpecP17_Taxonomy_Success(t *testing.T) {
	// Not t.Parallel(): evo.SetDefault/evo.Reason mutate process-global state.
	var buf bytes.Buffer
	evo.SetDefault(evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}}))
	out := evo.Default()
	branches := out.Task("branches")
	protected := evo.Reason("protected")
	dirty := evo.Reason("dirty")
	unpushed := evo.Reason("unpushed")
	for i := 0; i < 4; i++ {
		branches.Skipped(protected, "p"+string(rune('a'+i)))
	}
	for i := 0; i < 2; i++ {
		branches.Skipped(dirty, "d"+string(rune('a'+i)))
	}
	for i := 0; i < 3; i++ {
		branches.Kept(unpushed, "k"+string(rune('a'+i)))
	}
	branches.Done("14 deleted")
	branches.NextCommand("repo-retire", "salvage", "--dry-run")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := collapsed(buf.String())
	for _, want := range []string{
		"✓ branches 14 deleted",
		"! skipped 6 (4 protected, 2 dirty)",
		"! kept 3 (unpushed)",
		"repo-retire salvage --dry-run",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, buf.String())
		}
	}
}

// TestSpecP17_Taxonomy_Failure covers Problem 17's failure block: a
// second, independently-declared "branches" task (Output.Task get-or-creates
// by name like evo.Task — a genuinely distinct row sharing a display name
// needs an explicit evo.ID) fails mid-run while an earlier "branches" task's
// partial success survives, alongside an unchanged skip/keep taxonomy.
//
//	✓  branches  10 deleted
//	✗  branches  delete failed on feat/x
//	!  skipped 6  (unchanged)
//	!  kept 3     (unpushed, not attempted)
func TestSpecP17_Taxonomy_Failure(t *testing.T) {
	// Not t.Parallel(): evo.SetDefault/evo.Reason mutate process-global state.
	var buf bytes.Buffer
	evo.SetDefault(evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}}))
	out := evo.Default()
	done := out.Task("branches", evo.ID("branches.step1"))
	done.Record("delete", 10, "branches")
	done.Done("10 deleted")

	failed := out.Task("branches", evo.ID("branches.step2"))
	unchanged := evo.Reason("unchanged")
	notAttempted := evo.Reason("unpushed, not attempted")
	for i := 0; i < 6; i++ {
		failed.Skipped(unchanged, "s"+string(rune('a'+i)))
	}
	for i := 0; i < 3; i++ {
		failed.Kept(notAttempted, "k"+string(rune('a'+i)))
	}
	failed.Fail("delete failed on feat/x")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := collapsed(buf.String())
	for _, want := range []string{
		"✓ branches 10 deleted",
		"✗ branches delete failed on feat/x",
		"! skipped 6 (unchanged)",
		"! kept 3 (unpushed, not attempted)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, buf.String())
		}
	}
}

// TestSpecP17_Taxonomy_Indeterminate covers Problem 17's indeterminate
// block: a phase string while keep-reason classification is still running.
//
//	:.  branches  classifying keep reasons…
func TestSpecP17_Taxonomy_Indeterminate(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	out.Task("branches").Phase("classifying keep reasons…")

	got := screen.LatestLiveText()
	if !strings.Contains(got, "branches") || !strings.Contains(got, "classifying keep reasons…") {
		t.Fatalf("want indeterminate classification phase in live frame:\n%s", got)
	}
}

// TestSpecP17_Taxonomy_Error covers Problem 17's error block: a failed
// classification with technical evidence.
//
//	✗  branches  unable to classify dirty worktree
//	   └─ git status --porcelain failed
func TestSpecP17_Taxonomy_Error(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}})
	branches := out.Task("branches")
	branches.Fail("unable to classify dirty worktree", evo.Detail("git status --porcelain failed"))
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := collapsed(buf.String())
	for _, want := range []string{"✗ branches unable to classify dirty worktree", "git status --porcelain failed"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, buf.String())
		}
	}
}

// TestSpecP17_Taxonomy_EarlyTermination covers Problem 17's early
// termination block: a committed partial delete survives a later Cancel
// on a second independently-declared "branches" task.
//
//	✓  branches  10 deleted
//	■  branches  cancelled during keep pass
//	!  already mutated: 10 branches deleted
//
// The derived already-mutated line is always "<N> <object> <verb>"
// (summarizeChangeSection, plain.go:511-539). No public option appends
// caller narration to that mechanically-derived line.
func TestSpecP17_Taxonomy_EarlyTermination(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}})
	done := out.Task("branches", evo.ID("branches.step1"))
	done.Record("delete", 10, "branches")
	done.Done("10 deleted")

	cancelled := out.Task("branches", evo.ID("branches.step2"))
	cancelled.Cancel("cancelled during keep pass")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := collapsed(buf.String())
	for _, want := range []string{"✓ branches 10 deleted", "■ branches cancelled during keep pass"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, buf.String())
		}
	}
	if !strings.Contains(got, "already mutated: 10 branches deleted") {
		t.Fatalf("want the real derived already-mutated line, got:\n%s", buf.String())
	}
}

// ---------------------------------------------------------------------
// Problem 18 — remote-tracking fetch-prune vs remote delete-remote.
// (step1/step2/success already closed in spec_golden_test.go.)
// ---------------------------------------------------------------------

// TestSpecP18_RemoteTracking_Failure covers Problem 18's failure block: a
// failed fetch-prune with technical evidence, plus a durable note (the
// spec's own taught idiom for incidental information — evo-rec.md Problem
// 21's "out.Println / out.Verbose() only") stating remotes was never
// attempted.
//
//	✗  remote-tracking  fetch --prune failed
//	   └─ could not lock packed-refs
//	!  no remotes delete-remote attempted
func TestSpecP18_RemoteTracking_Failure(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}})
	tracking := out.Task("remote-tracking")
	tracking.Fail("fetch --prune failed", evo.Detail("could not lock packed-refs"))
	out.Println("!  no remotes delete-remote attempted")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := collapsed(buf.String())
	for _, want := range []string{
		"✗ remote-tracking fetch --prune failed",
		"could not lock packed-refs",
		"! no remotes delete-remote attempted",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, buf.String())
		}
	}
}

// TestSpecP18_RemoteTracking_Indeterminate covers Problem 18's
// indeterminate block: a live phase row while fetch --prune runs.
//
//	:.  remote-tracking  fetch --prune…
func TestSpecP18_RemoteTracking_Indeterminate(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	out.Task("remote-tracking").Phase("fetch --prune…")

	got := screen.LatestLiveText()
	if !strings.Contains(got, "remote-tracking") || !strings.Contains(got, "fetch --prune…") {
		t.Fatalf("want indeterminate fetch-prune phase in live frame:\n%s", got)
	}
}

// TestSpecP18_RemoteTracking_Error covers Problem 18's error block: a
// network failure during fetch, plus a durable note on origin's untouched
// state.
//
//	✗  remote-tracking  network error during fetch
//	   └─ fatal: unable to access 'https://…'
//	!  stale tracking refs unchanged; origin untouched
func TestSpecP18_RemoteTracking_Error(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}})
	tracking := out.Task("remote-tracking")
	tracking.Fail("network error during fetch", evo.Detail("fatal: unable to access 'https://…'"))
	out.Println("!  stale tracking refs unchanged; origin untouched")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := collapsed(buf.String())
	for _, want := range []string{
		"✗ remote-tracking network error during fetch",
		"fatal: unable to access 'https://…'",
		"! stale tracking refs unchanged; origin untouched",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, buf.String())
		}
	}
}

// TestSpecP18_RemoteTracking_EarlyTermination covers Problem 18's early
// termination block: an indeterminate fetch-prune phase, then a Cancel
// with an empty Changes ledger.
//
//	:.  remote-tracking  fetch --prune…
//	■  remote-tracking  cancelled
//
// An empty Changes ledger suppresses the already-mutated row entirely
// ("!" is attention-only; "none" earns no attention) — the already-
// established behavior pinned by
// TestConclusion_AlreadyMutated_CancelledEmptyLedger in phase_n2_test.go.
func TestSpecP18_RemoteTracking_EarlyTermination(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	tracking := out.Task("remote-tracking")
	tracking.Phase("fetch --prune…")
	before := screen.LatestLiveText()
	if !strings.Contains(before, "remote-tracking") || !strings.Contains(before, "fetch --prune…") {
		t.Fatalf("want indeterminate fetch-prune phase, got:\n%s", before)
	}

	tracking.Cancel("cancelled")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	final := screen.FinalText()
	if !strings.Contains(final, "■") || !strings.Contains(final, "remote-tracking") || !strings.Contains(final, "cancelled") {
		t.Fatalf("want cancelled remote-tracking row in final text, got:\n%s", final)
	}
	if strings.Contains(final, "already mutated") {
		t.Fatalf("expected the empty-ledger suppression (no already-mutated row) but got one; got:\n%s", final)
	}
}

// ---------------------------------------------------------------------
// Problem 19 — VisibilityDelay must gate from first Task activity, not
// process start; heavy init before the first declaration must not blank
// the screen.
// ---------------------------------------------------------------------

// TestSpecP19_FirstPaint_Step1 covers Problem 19's step1 block: the very
// first live frame is a Phase string, painted before any I/O.
//
//	:.  scan  reading config
func TestSpecP19_FirstPaint_Step1(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	out.Task("scan").Phase("reading config")

	got := screen.LatestLiveText()
	if !strings.Contains(got, "scan") || !strings.Contains(got, "reading config") {
		t.Fatalf("want first-paint phase in live frame:\n%s", got)
	}
}

// TestSpecP19_FirstPaint_Step2 covers Problem 19's step2 block: the same
// task's phase advances in place — no separate fake "startup" task.
//
//	:.  scan  discovering…
func TestSpecP19_FirstPaint_Step2(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	scan := out.Task("scan")
	scan.Phase("reading config")
	scan.Phase("discovering…")

	got := screen.LatestLiveText()
	if !strings.Contains(got, "scan") || !strings.Contains(got, "discovering…") {
		t.Fatalf("want advanced phase in live frame:\n%s", got)
	}
	if strings.Count(got, "scan") != 1 {
		t.Fatalf("want exactly one scan task (phase advanced in place, no new task), got:\n%s", got)
	}
}

// TestSpecP19_FirstPaint_Success covers Problem 19's success block: a
// Done summary plus a [changed] section reporting the discovered total.
//
//	✓  scan  128 checked
//	[changed]  scan
//	  ready  40  repos
func TestSpecP19_FirstPaint_Success(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}})
	scan := out.Task("scan")
	scan.RecordLabel("ready", 40, "repos")
	scan.Done("128 checked")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := collapsed(buf.String())
	for _, want := range []string{"✓ scan 128 checked", "[changed] scan", "ready 40 repos"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, buf.String())
		}
	}
}

// TestSpecP19_FirstPaint_Failure covers Problem 19's failure block: an
// invalid config fails scan with the offending line as evidence.
//
//	✗  scan  config invalid
//	   └─ zq.toml:12: unknown key "paralel"
func TestSpecP19_FirstPaint_Failure(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}})
	scan := out.Task("scan")
	scan.Fail("config invalid", evo.Detail(`zq.toml:12: unknown key "paralel"`))
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := collapsed(buf.String())
	for _, want := range []string{"✗ scan config invalid", `zq.toml:12: unknown key "paralel"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, buf.String())
		}
	}
}

// TestSpecP19_FirstPaint_Indeterminate covers Problem 19's indeterminate
// block: the same first-paint phase, still honest well inside
// VisibilityDelay.
//
//	:.  scan  reading config
func TestSpecP19_FirstPaint_Indeterminate(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	out.Task("scan").Phase("reading config")

	got := screen.LatestLiveText()
	if !strings.Contains(got, "scan") || !strings.Contains(got, "reading config") {
		t.Fatalf("want indeterminate first-paint phase in live frame:\n%s", got)
	}
}

// TestSpecP19_FirstPaint_Error covers Problem 19's error block: the config
// file itself cannot be read.
//
//	✗  scan  cannot read config
//	   └─ open ~/.config/zq/zq.toml: permission denied
func TestSpecP19_FirstPaint_Error(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}})
	scan := out.Task("scan")
	scan.Fail("cannot read config", evo.Detail("open ~/.config/zq/zq.toml: permission denied"))
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := collapsed(buf.String())
	for _, want := range []string{"✗ scan cannot read config", "open ~/.config/zq/zq.toml: permission denied"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, buf.String())
		}
	}
}

// TestSpecP19_FirstPaint_EarlyTermination covers Problem 19's early
// termination block: a Cancel during startup with nothing yet mutated.
//
//	■  scan  cancelled during startup
//
// An empty Changes ledger suppresses the already-mutated row entirely — the
// same established behavior as TestConclusion_AlreadyMutated_CancelledEmptyLedger
// in phase_n2_test.go.
func TestSpecP19_FirstPaint_EarlyTermination(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}})
	scan := out.Task("scan")
	scan.Cancel("cancelled during startup")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := collapsed(buf.String())
	if !strings.Contains(got, "■ scan cancelled during startup") {
		t.Fatalf("want cancelled scan row, got:\n%s", buf.String())
	}
	if strings.Contains(got, "already mutated") {
		t.Fatalf("expected the empty-ledger suppression (no already-mutated row) but got one; got:\n%s", buf.String())
	}
}

// ---------------------------------------------------------------------
// Problem 20 — a stale Phase looks identical to hung work; the caller must
// refresh Phase with elapsed context.
// ---------------------------------------------------------------------

// TestSpecP20_Heartbeat_Step1 covers Problem 20's step1 block: a phase
// naming the current object.
//
//	:.  salvage  pushing feat/a
func TestSpecP20_Heartbeat_Step1(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	out.Task("salvage").Phase("pushing feat/a")

	got := screen.LatestLiveText()
	if !strings.Contains(got, "salvage") || !strings.Contains(got, "pushing feat/a") {
		t.Fatalf("want phase in live frame:\n%s", got)
	}
}

// TestSpecP20_Heartbeat_Step2 covers Problem 20's step2 block: the phase
// text is refreshed in place with elapsed context so a stale-looking
// spinner is distinguishable from real evidence.
//
//	:.  salvage  pushing feat/a — 45s, waiting on remote
func TestSpecP20_Heartbeat_Step2(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	salvage := out.Task("salvage")
	salvage.Phase("pushing feat/a")
	salvage.Phase("pushing feat/a — 45s, waiting on remote")

	got := screen.LatestLiveText()
	if !strings.Contains(got, "salvage") || !strings.Contains(got, "pushing feat/a — 45s, waiting on remote") {
		t.Fatalf("want refreshed elapsed-context phase in live frame:\n%s", got)
	}
}

// TestSpecP20_Heartbeat_Success covers Problem 20's success block.
//
//	✓  salvage  3 pushed
func TestSpecP20_Heartbeat_Success(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}})
	out.Task("salvage").Done("3 pushed")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := collapsed(buf.String())
	if !strings.Contains(got, "✓ salvage 3 pushed") {
		t.Fatalf("want %q in:\n%s", "✓ salvage 3 pushed", buf.String())
	}
}

// TestSpecP20_Heartbeat_Failure covers Problem 20's failure block.
//
//	✗  salvage  remote timed out after 120s
//	   └─ write: broken pipe
func TestSpecP20_Heartbeat_Failure(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}})
	salvage := out.Task("salvage")
	salvage.Fail("remote timed out after 120s", evo.Detail("write: broken pipe"))
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := collapsed(buf.String())
	for _, want := range []string{"✗ salvage remote timed out after 120s", "write: broken pipe"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, buf.String())
		}
	}
}

// TestSpecP20_Heartbeat_Indeterminate covers Problem 20's indeterminate
// block: the elapsed-context refresh continuing further into the run.
//
//	:.  salvage  pushing feat/a — 90s
func TestSpecP20_Heartbeat_Indeterminate(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	salvage := out.Task("salvage")
	salvage.Phase("pushing feat/a")
	salvage.Phase("pushing feat/a — 90s")

	got := screen.LatestLiveText()
	if !strings.Contains(got, "salvage") || !strings.Contains(got, "pushing feat/a — 90s") {
		t.Fatalf("want refreshed elapsed-context phase in live frame:\n%s", got)
	}
}

// TestSpecP20_Heartbeat_Error covers Problem 20's error block: a
// connection reset plus a durable note that remote state is unknown.
//
//	✗  salvage  connection reset at 91s
//	   └─ read tcp: connection reset by peer
//	!  feat/a state on remote unknown — verify before retry
func TestSpecP20_Heartbeat_Error(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}})
	salvage := out.Task("salvage")
	salvage.Fail("connection reset at 91s", evo.Detail("read tcp: connection reset by peer"))
	out.Println("!  feat/a state on remote unknown — verify before retry")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := collapsed(buf.String())
	for _, want := range []string{
		"✗ salvage connection reset at 91s",
		"read tcp: connection reset by peer",
		"! feat/a state on remote unknown — verify before retry",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, buf.String())
		}
	}
}

// TestSpecP20_Heartbeat_EarlyTermination covers Problem 20's early
// termination block: a Cancel with no committed push confirmed.
//
//	■  salvage  cancelled at 96s
//
// An empty Changes ledger suppresses the already-mutated row entirely — the
// same established behavior as TestConclusion_AlreadyMutated_CancelledEmptyLedger
// in phase_n2_test.go; the mechanism never renders caller narration in its
// place.
func TestSpecP20_Heartbeat_EarlyTermination(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}})
	salvage := out.Task("salvage")
	salvage.Cancel("cancelled at 96s")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := collapsed(buf.String())
	if !strings.Contains(got, "■ salvage cancelled at 96s") {
		t.Fatalf("want cancelled salvage row, got:\n%s", buf.String())
	}
	if strings.Contains(got, "already mutated") {
		t.Fatalf("expected the empty-ledger suppression (no already-mutated row) but got one; got:\n%s", buf.String())
	}
}

// ---------------------------------------------------------------------
// Problem 21 — a durable note and the live region share one terminal and
// must be serialized by the library, not the caller.
// ---------------------------------------------------------------------

// TestSpecP21_DurableNote_Step1 covers Problem 21's step1 block: a live
// determinate-progress row.
//
//	:.  install  4/40  requests
func TestSpecP21_DurableNote_Step1(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	install := out.Task("install")
	install.Progress(4, 40)
	install.Phase("requests")

	got := screen.LatestLiveText()
	for _, want := range []string{"install", "4/40", "requests"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in live frame:\n%s", want, got)
		}
	}
}

// TestSpecP21_DurableNote_Step2 covers Problem 21's step2 block: a durable
// note lands above the live region intact, and the bar continues below —
// proven by the recorded operation order (durable write, then live
// redraw), not merely by both texts appearing somewhere.
//
//	using cached wheel index
//	:.  install  5/40  urllib3
func TestSpecP21_DurableNote_Step2(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	install := out.Task("install")
	install.Progress(4, 40)
	out.Println("using cached wheel index")
	install.Progress(5, 40)
	install.Phase("urllib3")

	found := false
	sealedAfterNote := false
	for i, op := range screen.Operations() {
		if op.Kind != "durable" || !strings.Contains(op.Text, "using cached wheel index") {
			continue
		}
		found = true
		for _, later := range screen.Operations()[i+1:] {
			if later.Kind == "live" && strings.Contains(later.Text, "install") && strings.Contains(later.Text, "5/40") && strings.Contains(later.Text, "urllib3") {
				sealedAfterNote = true
				break
			}
		}
		break
	}
	if !found {
		t.Fatalf("want a durable operation carrying %q, got ops: %+v", "using cached wheel index", screen.Operations())
	}
	if !sealedAfterNote {
		t.Fatalf("want a live redraw after the durable note showing install 5/40 urllib3, got ops: %+v", screen.Operations())
	}
}

// TestSpecP21_DurableNote_Success covers Problem 21's success block: the
// durable note precedes the Done summary in the final rendered stream.
//
//	using cached wheel index
//	✓  install  40/40
func TestSpecP21_DurableNote_Success(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}})
	out.Println("using cached wheel index")
	out.Task("install").Done("40/40")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	noteAt := strings.Index(got, "using cached wheel index")
	doneAt := strings.Index(got, "install")
	if noteAt < 0 || doneAt < 0 {
		t.Fatalf("want both the durable note and the Done row, got:\n%s", got)
	}
	if noteAt > doneAt {
		t.Fatalf("want the durable note to precede the Done row, got:\n%s", got)
	}
	if !strings.Contains(collapsed(got), "✓ install 40/40") {
		t.Fatalf("want %q in:\n%s", "✓ install 40/40", got)
	}
}

// TestSpecP21_DurableNote_Failure covers Problem 21's failure block.
//
//	✗  install  mirror unreachable
//	   └─ dial tcp: i/o timeout
func TestSpecP21_DurableNote_Failure(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}})
	install := out.Task("install")
	install.Fail("mirror unreachable", evo.Detail("dial tcp: i/o timeout"))
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := collapsed(buf.String())
	for _, want := range []string{"✗ install mirror unreachable", "dial tcp: i/o timeout"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, buf.String())
		}
	}
}

// TestSpecP21_DurableNote_Indeterminate covers Problem 21's indeterminate
// block.
//
//	:.  install  resolving…
func TestSpecP21_DurableNote_Indeterminate(t *testing.T) {
	t.Parallel()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := newLiveScreenOutput(screen)
	t.Cleanup(func() { _ = out.Close() })

	out.Task("install").Phase("resolving…")

	got := screen.LatestLiveText()
	if !strings.Contains(got, "install") || !strings.Contains(got, "resolving…") {
		t.Fatalf("want indeterminate resolving phase in live frame:\n%s", got)
	}
}

// TestSpecP21_DurableNote_Error covers Problem 21's error block: the
// dialect's own self-check that a torn frame was avoided.
//
//	✗  install  torn frame avoided — durable write serialized
//	   └─ (caller used out.Println; no fmt in live window)
func TestSpecP21_DurableNote_Error(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}})
	install := out.Task("install")
	install.Fail("torn frame avoided — durable write serialized", evo.Detail("(caller used out.Println; no fmt in live window)"))
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := collapsed(buf.String())
	for _, want := range []string{
		"✗ install torn frame avoided — durable write serialized",
		"(caller used out.Println; no fmt in live window)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, buf.String())
		}
	}
}

// TestSpecP21_DurableNote_EarlyTermination covers Problem 21's early
// termination block: a durable note precedes a Cancel mid-install, with a
// committed partial install count.
//
//	using cached wheel index
//	■  install  cancelled at 5/40
//	!  already mutated: 5 packages in .venv installed
//
// The derived already-mutated line is always "<N> <object> <verb>"
// (summarizeChangeSection, plain.go:511-539) — the trailing conjugated verb
// is always appended.
func TestSpecP21_DurableNote_EarlyTermination(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}})
	out.Println("using cached wheel index")
	install := out.Task("install")
	install.Progress(5, 40)
	install.Record("install", 5, "packages in .venv")
	install.Cancel("cancelled at 5/40")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := collapsed(buf.String())
	for _, want := range []string{"using cached wheel index", "■ install cancelled at 5/40"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, buf.String())
		}
	}
	if !strings.Contains(got, "already mutated: 5 packages in .venv installed") {
		t.Fatalf("want the real derived already-mutated line, got:\n%s", buf.String())
	}
}
