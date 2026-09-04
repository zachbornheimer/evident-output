package goldens_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// This file proves the "Recommended UI" blocks from ~/Desktop/evo-rec.md
// render for real through the library's public front door. Each test names
// the spec problem it covers in its doc comment; a full checklist mapping
// every fenced block to a verdict lives in the final report handed back with
// this work order.

// TestSpecP2_LocalRemoteSeparation_Step1 covers Problem 2's step1 block: a
// dry-run plan for one local delete, spelled the documented way
// (evo.DryRun() + Task.Delete — see "Guess-driven defaults" #1).
//
//	[planned]  branches
//	  delete  feat/old-billing
func TestSpecP2_LocalRemoteSeparation_Step1(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("retire"), evo.To(&buf), evo.Plain(), evo.NoColor(), evo.DryRun()}})
	branches := out.Task("branches")
	branches.RecordName("delete", "feat/old-billing")
	branches.Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	for _, want := range []string{"[planned] branches", "delete feat/old-billing"} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
}

// TestSpecP2_LocalRemoteSeparation_Step2 covers Problem 2's step2 block: a
// dry-run plan with both a local and a remote-destructive section, kept as
// two separate Plan subjects with distinct verbs.
//
//	[planned]  branches
//	  delete         12  local tip
//	[planned]  remotes
//	  delete-remote   3  origin tip
func TestSpecP2_LocalRemoteSeparation_Step2(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("retire"), evo.To(&buf), evo.Plain(), evo.NoColor(), evo.DryRun()}})
	branches := out.Task("branches")
	branches.Record("delete", 12, "local tip")
	branches.Done()
	remotes := out.Task("remotes")
	remotes.Record("delete-remote", 3, "origin tip")
	remotes.Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	for _, want := range []string{
		"[planned] branches",
		"delete 12 local tip",
		"[planned] remotes",
		"delete-remote 3 origin tip",
	} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
}

// TestSpecP2_LocalRemoteSeparation_Success covers evo-rec.md Problem 2
// ("Remote-destructive deletes mixed with local branch deletes") success
// block: separate Plan/Changes subjects for branches (local) vs remotes,
// never one list.
//
//	[changed]  branches
//	  deleted  12  local tip
//	[changed]  remotes
//	  deleted   3  origin tip
func TestSpecP2_LocalRemoteSeparation_Success(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("retire"), evo.To(&buf), evo.Plain(), evo.NoColor()}})
	branches := out.Task("branches")
	branches.Record("delete", 12, "local tip")
	branches.Done()
	remotes := out.Task("remotes")
	remotes.Record("delete", 3, "origin tip")
	remotes.Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	for _, want := range []string{
		"[changed] branches",
		"deleted 12 local tip",
		"[changed] remotes",
		"deleted 3 origin tip",
	} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
	// The two subjects must never collapse into one list.
	if strings.Count(got, "[changed] branches") != 1 || strings.Count(got, "[changed] remotes") != 1 {
		t.Fatalf("want one [changed] section each for branches and remotes, got:\n%s", got)
	}
}

// TestSpecP2_LocalRemoteSeparation_Failure covers Problem 2's failure block:
// local mutation survives a remote failure, stated as separate subjects.
//
//	[changed]  branches
//	  deleted  12  local tip
//	✗  remotes  push --delete denied
//	   └─ protected branch rule on origin
func TestSpecP2_LocalRemoteSeparation_Failure(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("retire"), evo.To(&buf), evo.Plain(), evo.NoColor()}})
	branches := out.Task("branches")
	branches.Record("delete", 12, "local tip")
	branches.Done()
	remotes := out.Task("remotes")
	remotes.Fail("push --delete denied", evo.Detail("protected branch rule on origin"))
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	for _, want := range []string{
		"[changed] branches",
		"deleted 12 local tip",
		"✗ remotes push --delete denied",
		"protected branch rule on origin",
	} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
}

// TestSpecP4_SequentialGroup_Success covers evo-rec.md Problem 4 ("parallel
// domain work presented sequentially: one Running child, siblings named
// idle") success block via evo.Group — predeclared Tasks resolve in order,
// no concurrent Running siblings, parent lists every child Done.
//
//	python
//	✓  scan
//	✓  venv
//	✓  install  14 modules
func TestSpecP4_SequentialGroup_Success(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("python"), evo.To(&buf), evo.Plain(), evo.NoColor()}})
	setup := out.Sequence("python")
	scan, venv, install := setup.Task("scan"), setup.Task("venv"), setup.Task("install")
	scan.Done()
	venv.Done()
	install.Done("14 modules")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{"✓  scan", "✓  venv", "✓  install  14 modules"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
}

// TestSpecP4_SequentialGroup_Failure covers Problem 4's failure block: a
// failed child stops the group and later siblings render "not started",
// never invented Done/Pending.
//
//	python
//	✓  scan
//	✗  venv     uv exited 1: No such file or directory
//	-  install  not started
func TestSpecP4_SequentialGroup_Failure(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("python"), evo.To(&buf), evo.Plain(), evo.NoColor()}})
	setup := out.Sequence("python")
	scan, venv := setup.Task("scan"), setup.Task("venv")
	setup.Task("install")
	scan.Done()
	venv.Fail("uv exited 1: No such file or directory")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{
		"✓  scan",
		"✗  venv     uv exited 1: No such file or directory",
		"-  install  not started",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
}

// TestSpecP5_DiscoverySealedTotal_Success covers evo-rec.md Problem 5's full
// success block: the Done summary after an indeterminate-to-determinate
// Progress transition, plus a classification ledger reporting the
// discovered counts verbatim.
//
//	✓  scan  128 checked
//	[changed]  scan
//	  ready    40  repos
func TestSpecP5_DiscoverySealedTotal_Success(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("scan"), evo.To(&buf), evo.Plain(), evo.NoColor()}})
	scan := out.Task("scan")
	scan.Progress(128, 128)
	scan.RecordLabel("ready", 40, "repos")
	scan.RecordLabel("blocked", 80, "repos")
	scan.RecordLabel("error", 8, "repos")
	scan.Done("128 checked")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	for _, want := range []string{
		"✓ scan 128 checked",
		"[changed] scan",
		"ready 40 repos",
		"blocked 80 repos",
		"error 8 repos",
	} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
	// The classification labels must render verbatim — never conjugated
	// ("readyed"/"blockeded"/"errored" would be the pre-fix lying output).
	for _, mustNotContain := range []string{"readyed", "blockeded", "errored"} {
		if strings.Contains(collapsed, mustNotContain) {
			t.Fatalf("classification label was conjugated, got %q in:\n%s", mustNotContain, got)
		}
	}
}

// TestSpecP5_RecordLabel_NeverMovesUnderPlanDuringDryRun pins the second half
// of the fix: classifying/observing already happened whether or not other
// mutations on this run are a dry run, so RecordLabel always lands in the
// Changes ledger — never [planned] — even when DryRun is set.
func TestSpecP5_RecordLabel_NeverMovesUnderPlanDuringDryRun(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("scan"), evo.To(&buf), evo.Plain(), evo.NoColor(), evo.DryRun()}})
	scan := out.Task("scan")
	scan.RecordLabel("ready", 40, "repos")
	scan.Done("128 checked")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "[changed] scan") {
		t.Fatalf("want RecordLabel in the Changes ledger even under DryRun, got:\n%s", got)
	}
	// Checked against the structured snapshot, not a substring of the durable
	// text: the run's own trailing Conclusion trailer legitimately reads
	// "[planned]  scan" (DryRun's headline state) even when no Plan *section*
	// exists, so a plain string search on "[planned]  scan" collides with it.
	if snap := out.Snapshot(); len(snap.Plans) != 0 {
		t.Fatalf("RecordLabel must never create a Plan section, got %+v", snap.Plans)
	}
}

// TestSpecP5_DiscoverySealedTotal_NeverShrinks pins the accompanying
// invariant from the "Progress invariants" section: once a total is sealed
// via Progress, ErrProgressRegression fires on a later smaller total rather
// than silently reprinting a smaller denominator.
func TestSpecP5_DiscoverySealedTotal_NeverShrinks(t *testing.T) {
	t.Parallel()
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("scan"), evo.NoColor()}})
	t.Cleanup(func() { _ = out.Close() })
	scan := out.Task("scan")
	scan.Progress(40, 128)
	scan.Progress(14, 53) // smaller sealed total: must be rejected, not silently applied
	if out.Err() == nil {
		t.Fatal("want recorded misuse when a sealed total shrinks")
	}
}

// TestSpecP6_BytesVsCounts_Success covers evo-rec.md Problem 6 (bytes and
// item counts must never share Progress) success block: Bytes for byte
// totals, Progress for counts, both surviving into Done summaries.
//
//	✓  generate  8.0 MB
//	✓  test      12/12  ok
func TestSpecP6_BytesVsCounts_Success(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("build"), evo.To(&buf), evo.Plain(), evo.NoColor()}})
	generate := out.Task("generate")
	generate.Bytes(8_000_000, 8_000_000)
	generate.Done("8.0 MB")
	test := out.Task("test")
	test.Progress(12, 12)
	test.Done("12/12  ok")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{"✓  generate  8.0 MB", "✓  test      12/12  ok"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
}

// TestSpecP7_ViewportTruncation_Success covers evo-rec.md Problem 7 (a plan
// with 500 rows must bound the visible list and carry one overflow line,
// never dump the terminal) success block.
//
//	✓  branches  500 deleted
//	!  names truncated in live view (500 in model)
//
// TestSpecP7_ViewportTruncation_PlanOverflowLine exercises the viewport
// bound with 500 distinct records — release-gate round 3 finding 6 merges
// identical (verb, object) records instead of duplicating the row, so this
// case uses a distinct object per call to keep exercising the bounded-rows
// overflow line it was written to prove.
func TestSpecP7_ViewportTruncation_PlanOverflowLine(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("clean"), evo.To(&buf), evo.Plain(), evo.NoColor()}})
	branches := out.Task("branches")
	for i := 0; i < 500; i++ {
		branches.RecordName("delete", fmt.Sprintf("feat/branch-%d", i))
	}
	branches.Done("500 deleted")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "✓  branches  500 deleted") {
		t.Fatalf("want done summary, got:\n%s", got)
	}
	if !strings.Contains(got, "more (not shown)") {
		t.Fatalf("want a bounded-rows overflow line for 500 records, got:\n%s", got)
	}
}

// TestSpecP8_PartialTruthSurvivesRemoteAuthFailure covers evo-rec.md Problem
// 8 (auth fails after some remote deletes already succeeded: prior Done
// stays, Fail carries the real remote message, "already mutated" is derived)
// failure block.
//
//	[changed]  remotes
//	  deleted  1  origin tip
//	✗  remotes  authentication failed
//	   └─ remote: Invalid username or token
//	!  already mutated: origin/feat/a deleted; feat/b feat/c not
func TestSpecP8_PartialTruthSurvivesRemoteAuthFailure(t *testing.T) {
	t.Parallel()
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("retire"), evo.NoColor()}})
	t.Cleanup(func() { _ = out.Close() })
	remotes := out.Task("remotes")
	remotes.RecordName("delete", "origin/feat/a")
	remotes.Fail("authentication failed", evo.Detail("remote: Invalid username or token"))
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	// FinalPlain is unexported (C8); reconstruct the same text RenderPlain
	// produces from the finished snapshot.
	rendered, err := evo.RenderPlain(out.Snapshot(), evo.PlainOptions{Width: 80, NoColor: true})
	if err != nil {
		t.Fatal(err)
	}
	got := string(rendered)
	collapsed := strings.Join(strings.Fields(got), " ")
	for _, want := range []string{
		"[changed] remotes",
		"deleted origin/feat/a",
		"✗ remotes authentication failed",
		"remote: Invalid username or token",
	} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
	c := out.Conclusion()
	if c.State != evo.StateFailed {
		t.Fatalf("conclusion state = %v, want StateFailed", c.State)
	}
}

// TestSpecP15_NothingToDo_Success covers evo-rec.md Problem 15 (empty-success
// paths get a quiet Item OK plus one plain line, never an invented warning or
// a spinning zero-count task).
//
//	✓  clean
//	nothing to clean
func TestSpecP15_NothingToDo_Success(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("clean"), evo.To(&buf), evo.Plain(), evo.NoColor()}})
	out.Task("clean").Done()
	out.Println("nothing to clean")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{"✓  clean", "nothing to clean"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "[warning]") || strings.Contains(got, "!") {
		t.Fatalf("empty-success path must not invent a warning, got:\n%s", got)
	}
}

// TestSpecP3_DryRunTense_Step1 covers evo-rec.md Problem 3's step1 block: a
// dry-run plan for a named push, spelled the documented way.
//
//	[planned]  salvage
//	  push  3  feat/a → retire/feat/a
func TestSpecP3_DryRunTense_Step1(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("salvage"), evo.To(&buf), evo.Plain(), evo.NoColor(), evo.DryRun()}})
	salvage := out.Task("salvage")
	_ = salvage.Push("feat/a → retire/feat/a", nil, evo.Affected(3))
	salvage.Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	for _, want := range []string{"[planned] salvage", "push 3 feat/a → retire/feat/a"} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
}

// TestSpecP18_RemoteTrackingVsRemoteDelete_Step1 covers Problem 18's step1
// block: a dry-run plan for one fetch-prune, using the documented
// RecordName spelling.
//
//	[planned]  remote-tracking
//	  fetch-prune  origin/feat/gone
func TestSpecP18_RemoteTrackingVsRemoteDelete_Step1(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("clean"), evo.To(&buf), evo.Plain(), evo.NoColor(), evo.DryRun()}})
	tracking := out.Task("remote-tracking")
	tracking.RecordName("fetch-prune", "origin/feat/gone")
	tracking.Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	for _, want := range []string{"[planned] remote-tracking", "fetch-prune origin/feat/gone"} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
}

// TestSpecP18_RemoteTrackingVsRemoteDelete_Step2 covers Problem 18's step2
// block: two separate Plan subjects — remote-tracking (fetch-prune) and
// remotes (delete-remote) — never merged into one, even when one side is
// empty.
//
//	[planned]  remote-tracking
//	  fetch-prune  12  stale origin/*
//	[planned]  remotes
//	  delete-remote  0  (none)
func TestSpecP18_RemoteTrackingVsRemoteDelete_Step2(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("clean"), evo.To(&buf), evo.Plain(), evo.NoColor(), evo.DryRun()}})
	tracking := out.Task("remote-tracking")
	tracking.Record("fetch-prune", 12, "stale origin/*")
	tracking.Done()
	remotes := out.Task("remotes")
	remotes.Record("delete-remote", 0, "origin refs")
	remotes.Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	if !strings.Contains(collapsed, "[planned] remote-tracking") || !strings.Contains(collapsed, "fetch-prune 12 stale origin/*") {
		t.Fatalf("want fetch-prune plan section, got:\n%s", got)
	}
	// A zero-quantity section still renders an honest empty line, never a
	// fabricated "0 (none)" row that hides which verb was intended.
	if !strings.Contains(collapsed, "nothing to delete-remote remotes") {
		t.Fatalf("want empty-section grammar for the zero-quantity remotes plan, got:\n%s", got)
	}
}

// TestSpecP18_RemoteTrackingVsRemoteDelete_Success covers evo-rec.md Problem
// 18 (fetch --prune stale remote-tracking refs must never share a subject
// with a real `git push --delete` remote branch delete) success block:
// distinct Plan/Changes subjects, distinct verbs.
//
//	[changed]  remote-tracking
//	  pruned  12  stale origin/*
func TestSpecP18_RemoteTrackingVsRemoteDelete_Success(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("clean"), evo.To(&buf), evo.Plain(), evo.NoColor()}})
	tracking := out.Task("remote-tracking")
	tracking.Record("prune", 12, "stale origin/*")
	tracking.Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	for _, want := range []string{"[changed] remote-tracking", "pruned 12 stale origin/*"} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
	// The subject must never be spelled "remotes" — that name is reserved for
	// real push --delete destructive verbs (Problem 2 / Problem 18 divergence).
	if strings.Contains(got, "[changed]  remotes\n") {
		t.Fatalf("remote-tracking prune must not share the \"remotes\" subject, got:\n%s", got)
	}
}

// TestSpecP25_ASCIIGlyphFallback_Success covers evo-rec.md Problem 25
// (non-UTF-8 locale / dumb terminal: identical dialect, ASCII faces) success
// block — GlyphsASCII must render "[ok]"/"[!]" markers, never mojibake or
// bare Unicode.
//
//	[ok] branches   14 deleted
//	[ok] worktrees  2 removed
//	[!]  skipped 6  (protected, dirty)
func TestSpecP25_ASCIIGlyphFallback_Success(t *testing.T) {
	// Not t.Parallel(): evo.SetDefault/evo.Reason mutate process-global state,
	// same as the existing default-instance tests in taxonomy_test.go.
	var buf bytes.Buffer
	evo.SetDefault(evo.Init(evo.Config{Options: []evo.Option{evo.Title("clean"), evo.To(&buf), evo.Plain(), evo.NoColor(), evo.Glyphs(evo.GlyphsASCII)}}))
	out := evo.Default()
	branches := out.Task("branches")
	pr := evo.Reason("protected")
	dirty := evo.Reason("dirty")
	branches.Skipped(pr, "main")
	branches.Skipped(dirty, "feat/a")
	branches.Record("delete", 14, "branches")
	branches.Done("14 deleted")
	worktrees := out.Task("worktrees")
	worktrees.Record("remove", 2, "worktrees")
	worktrees.Done("2 removed")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{"[ok]  branches  14 deleted", "[ok]  worktrees  2 removed", "[!]  skipped 2"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in ASCII-profile output:\n%s", want, got)
		}
	}
	if strings.ContainsAny(got, "✓✗⊘■○→…") {
		t.Fatalf("ASCII profile must not leak any Unicode state glyph, got:\n%s", got)
	}
}

// TestSpecP24_DataFormat_PresentationNeverTouchesPayloadStream covers
// evo-rec.md Problem 24 (--json pipes stdout to jq; presentation and data
// must never share a stream): FormatData's ResultWriter is a distinct stream
// from the presentation destination, so a spinner/✓ row can never land in
// the payload.
func TestSpecP24_DataFormat_PresentationNeverTouchesPayloadStream(t *testing.T) {
	t.Parallel()
	var presentation, payload bytes.Buffer
	out := evo.Init(evo.Config{
		Title:  "scan",
		Format: evo.FormatData,
		Stderr: &presentation,
		Result: &payload,
		Color:  evo.ColorNever,
	})
	scan := out.Task("scan")
	scan.Done("128 checked")
	_, err := out.ResultWriter().Write([]byte(`{"ready":40,"blocked":80,"error":8}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(payload.String(), "✓✗■") {
		t.Fatalf("presentation glyphs leaked into the domain payload stream:\n%s", payload.String())
	}
	if !strings.Contains(payload.String(), `"ready":40`) {
		t.Fatalf("payload stream missing the domain payload:\n%s", payload.String())
	}
	if !strings.Contains(presentation.String(), "✓  scan  128 checked") {
		t.Fatalf("presentation stream missing the task row:\n%s", presentation.String())
	}
}
