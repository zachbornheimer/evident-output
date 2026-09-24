package goldens_test

import (
	"bytes"
	"context"
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
// (Config.DryRun + evo.Effect — see "Guess-driven defaults" #1).
//
//	[planned]  branches
//	  delete  1  feat/old-billing
func TestSpecP2_LocalRemoteSeparation_Step1(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Title: "retire", Stdout: &buf, Plain: true, Color: evo.ColorNever, DryRun: true})
	commit(out.Task("branches"), evo.EffectSpec{Verb: evo.EffectDelete, Object: "feat/old-billing", Quantity: 1})
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	for _, want := range []string{"[planned] branches", "delete 1 feat/old-billing"} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
}

// TestSpecP2_LocalRemoteSeparation_Step2 covers Problem 2's step2 block: a
// dry-run plan with both a local and a remote-destructive section, kept as
// two separate Plan subjects.
//
//	[planned]  branches
//	  delete  12  local tip
//	[planned]  remotes
//	  delete   3  origin tip
func TestSpecP2_LocalRemoteSeparation_Step2(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Title: "retire", Stdout: &buf, Plain: true, Color: evo.ColorNever, DryRun: true})
	commit(out.Task("branches"), evo.EffectSpec{Verb: evo.EffectDelete, Object: "local tip", Quantity: 12})
	commit(out.Task("remotes"), evo.EffectSpec{Verb: evo.EffectDelete, Object: "origin tip", Quantity: 3})
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	for _, want := range []string{
		"[planned] branches",
		"delete 12 local tip",
		"[planned] remotes",
		"delete 3 origin tip"} {
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
	out := evo.Init(evo.Config{Title: "retire", Stdout: &buf, Plain: true, Color: evo.ColorNever})
	commit(out.Task("branches"), evo.EffectSpec{Verb: evo.EffectDelete, Object: "local tip", Quantity: 12})
	commit(out.Task("remotes"), evo.EffectSpec{Verb: evo.EffectDelete, Object: "origin tip", Quantity: 3})
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	for _, want := range []string{
		"[changed] branches",
		"deleted 12 local tip",
		"[changed] remotes",
		"deleted 3 origin tip"} {
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
	out := evo.Init(evo.Config{Title: "retire", Stdout: &buf, Plain: true, Color: evo.ColorNever})
	commit(out.Task("branches"), evo.EffectSpec{Verb: evo.EffectDelete, Object: "local tip", Quantity: 12})
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
		"protected branch rule on origin"} {
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
	out := evo.Init(evo.Config{Isolated: true, Title: "python", Stdout: &buf, Plain: true, Color: evo.ColorNever})
	setup := out.Sequence("python")
	setup.Task("scan").Define(func(ctx context.Context) error { return nil })
	setup.Task("venv").Define(func(ctx context.Context) error { return nil })
	install := setup.Task("install")
	install.Define(func(ctx context.Context) error {
		install.Summary("14 modules")
		return nil
	})
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{"✓ scan", "✓ venv", "✓ install  14 modules"} {
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
	out := evo.Init(evo.Config{Isolated: true, Title: "python", Stdout: &buf, Plain: true, Color: evo.ColorNever})
	setup := out.Sequence("python")
	setup.Task("scan").Define(func(ctx context.Context) error { return nil })
	setup.Task("venv").Define(func(ctx context.Context) error {
		return fmt.Errorf("uv exited 1: No such file or directory")
	})
	setup.Task("install")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{
		"✓ scan",
		"✗ venv     uv exited 1: No such file or directory",
		"- install  not started"} {
		if !strings.Contains(got, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
}

// TestSpecP5_DiscoverySealedTotal_Success covers evo-rec.md Problem 5's full
// success block: the Done summary after an indeterminate-to-determinate
// Progress transition, plus classification Facts reporting the discovered
// counts verbatim — a classification is information, not a mutation
// (ZYS-974), so it never lands in the Changes ledger.
//
//	✓  scan  128 checked
//	  ready    40 repos
func TestSpecP5_DiscoverySealedTotal_Success(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Title: "scan", Stdout: &buf, Plain: true, Color: evo.ColorNever})
	scan := out.Task("scan")
	scan.Progress(128, 128)
	scan.Fact("ready", "40 repos")
	scan.Fact("blocked", "80 repos")
	scan.Fact("error", "8 repos")
	succeed(scan, "128 checked")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	for _, want := range []string{
		"✓ scan 128 checked",
		"ready 40 repos",
		"blocked 80 repos",
		"error 8 repos"} {
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

// TestSpecP5_ClassificationFact_NeverMovesUnderPlanDuringDryRun pins the
// second half of the fix: classifying/observing already happened whether or
// not other mutations on this run are a dry run, so a classification Fact
// renders under its Task and never creates a [planned] section, even when
// DryRun is set.
func TestSpecP5_ClassificationFact_NeverMovesUnderPlanDuringDryRun(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Title: "scan", Stdout: &buf, Plain: true, Color: evo.ColorNever, DryRun: true})
	scan := out.Task("scan")
	scan.Fact("ready", "40 repos")
	succeed(scan, "128 checked")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(strings.Join(strings.Fields(got), " "), "ready 40 repos") {
		t.Fatalf("want the classification Fact rendered even under DryRun, got:\n%s", got)
	}
	// Checked against the structured snapshot, not a substring of the durable
	// text: the run's own trailing Conclusion trailer legitimately reads
	// "[planned]  scan" (DryRun's headline state) even when no Plan *section*
	// exists, so a plain string search on "[planned]  scan" collides with it.
	if snap := out.Snapshot(); len(snap.Plans) != 0 {
		t.Fatalf("a Fact must never create a Plan section, got %+v", snap.Plans)
	}
}

// TestSpecP5_DiscoverySealedTotal_NeverShrinks pins the accompanying
// invariant from the "Progress invariants" section: once a total is sealed
// via Progress, ErrProgressRegression fires on a later smaller total rather
// than silently reprinting a smaller denominator.
func TestSpecP5_DiscoverySealedTotal_NeverShrinks(t *testing.T) {
	t.Parallel()
	out := evo.Init(evo.Config{Title: "scan", Color: evo.ColorNever})
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
	out := evo.Init(evo.Config{Title: "build", Stdout: &buf, Plain: true, Color: evo.ColorNever})
	generate := out.Task("generate")
	generate.Bytes(8_000_000, 8_000_000)
	succeed(generate, "8.0 MB")
	test := out.Task("test")
	test.Progress(12, 12)
	succeed(test, "12/12  ok")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{"✓ generate  8.0 MB", "✓ test      12/12  ok"} {
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
	out := evo.Init(evo.Config{Title: "clean", Stdout: &buf, Plain: true, Color: evo.ColorNever})
	specs := make([]evo.EffectSpec, 500)
	for i := range specs {
		specs[i] = evo.EffectSpec{Verb: evo.EffectDelete, Object: fmt.Sprintf("feat/branch-%d", i), Quantity: 1}
	}
	commit(out.Task("branches").Summary("500 deleted"), specs...)
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "✓ branches  500 deleted") {
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
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Title: "retire", Stdout: &buf, Color: evo.ColorNever, Plain: true})
	t.Cleanup(func() { _ = out.Close() })
	remotes := out.Task("remotes")
	commitThen(remotes, evo.EffectSpec{Verb: evo.EffectDelete, Object: "origin/feat/a", Quantity: 1}, func() {
		remotes.Fail("authentication failed", evo.Detail("remote: Invalid username or token"))
	})
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	for _, want := range []string{
		"[changed] remotes",
		"deleted 1 origin/feat/a",
		"✗ remotes authentication failed",
		"remote: Invalid username or token"} {
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
	out := evo.Init(evo.Config{Title: "clean", Stdout: &buf, Plain: true, Color: evo.ColorNever})
	succeed(out.Task("clean"))
	out.Println("nothing to clean")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{"✓ clean", "nothing to clean"} {
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
	out := evo.Init(evo.Config{Isolated: true, Title: "salvage", Stdout: &buf, Plain: true, Color: evo.ColorNever, DryRun: true})
	called := false
	salvage := out.Task("salvage")
	salvage.Define(func(ctx context.Context) error {
		return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectPush, Object: "feat/a → retire/feat/a", Quantity: 3}, func(context.Context) error {
			called = true
			return nil
		})
	})
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("dry-run mutation callback must not run")
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
// block: a dry-run plan for one stale remote-tracking ref removal, using the
// documented Effect spelling (EffectRemove: a local tracking ref is removed,
// never deleted on the remote).
//
//	[planned]  remote-tracking
//	  remove  1  origin/feat/gone
func TestSpecP18_RemoteTrackingVsRemoteDelete_Step1(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Title: "clean", Stdout: &buf, Plain: true, Color: evo.ColorNever, DryRun: true})
	commit(out.Task("remote-tracking"), evo.EffectSpec{Verb: evo.EffectRemove, Object: "origin/feat/gone", Quantity: 1})
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	for _, want := range []string{"[planned] remote-tracking", "remove 1 origin/feat/gone"} {
		if !strings.Contains(collapsed, want) {
			t.Fatalf("want %q in:\n%s", want, got)
		}
	}
}

// TestSpecP18_RemoteTrackingVsRemoteDelete_Step2 covers Problem 18's step2
// block: remote-tracking (remove) and remotes (delete) are separate Plan
// subjects, never merged into one — and a remotes Task with nothing to
// delete calls no Effect, so it has no section at all rather than a
// fabricated "0 (none)" row.
//
//	[planned]  remote-tracking
//	  remove  12  stale origin/*
func TestSpecP18_RemoteTrackingVsRemoteDelete_Step2(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Title: "clean", Stdout: &buf, Plain: true, Color: evo.ColorNever, DryRun: true})
	commit(out.Task("remote-tracking"), evo.EffectSpec{Verb: evo.EffectRemove, Object: "stale origin/*", Quantity: 12})
	commit(out.Task("remotes"))
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	if !strings.Contains(collapsed, "[planned] remote-tracking") || !strings.Contains(collapsed, "remove 12 stale origin/*") {
		t.Fatalf("want the remote-tracking plan section, got:\n%s", got)
	}
	if snap := out.Snapshot(); len(snap.Plans) != 1 {
		t.Fatalf("want only the remote-tracking Plan section, got %+v", snap.Plans)
	}
}

// TestSpecP18_RemoteTrackingVsRemoteDelete_Success covers evo-rec.md Problem
// 18 (fetch --prune stale remote-tracking refs must never share a subject
// with a real `git push --delete` remote branch delete) success block:
// distinct Plan/Changes subjects, distinct verbs.
//
//	[changed]  remote-tracking
//	  removed  12  stale origin/*
func TestSpecP18_RemoteTrackingVsRemoteDelete_Success(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Title: "clean", Stdout: &buf, Plain: true, Color: evo.ColorNever})
	commit(out.Task("remote-tracking"), evo.EffectSpec{Verb: evo.EffectRemove, Object: "stale origin/*", Quantity: 12})
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	collapsed := strings.Join(strings.Fields(got), " ")
	for _, want := range []string{"[changed] remote-tracking", "removed 12 stale origin/*"} {
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
//	- skipped 1 (protected)
//	- skipped 1 (dirty)
func TestSpecP25_ASCIIGlyphFallback_Success(t *testing.T) {
	// Not t.Parallel(): evo.SetDefault/evo.Reason mutate process-global state,
	// same as the existing default-instance tests in taxonomy_test.go.
	var buf bytes.Buffer
	evo.SetDefault(evo.Init(evo.Config{Isolated: true, Title: "clean", Stdout: &buf, Plain: true, Color: evo.ColorNever, Glyphs: evo.GlyphsASCII}))
	out := evo.Default()
	protected := evo.Reason("protected")
	dirty := evo.Reason("dirty")
	g := out.Group("branches")
	g.Summary("14 deleted")
	g.Task("protected-0").Skipped(protected)
	g.Task("dirty-0").Skipped(dirty)
	commit(g.Task("deleted").Summary("14 deleted"), evo.EffectSpec{Verb: evo.EffectDelete, Object: "branch", Quantity: 14})
	commit(out.Task("worktrees").Summary("2 removed"), evo.EffectSpec{Verb: evo.EffectRemove, Object: "worktree", Quantity: 2})
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{"[ok] branches  14 deleted", "[ok] worktrees  2 removed", "- skipped 1 (protected)", "- skipped 1 (dirty)"} {
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
		Color:  evo.ColorNever})
	scan := out.Task("scan")
	succeed(scan, "128 checked")
	_, err := payload.Write([]byte(`{"ready":40,"blocked":80,"error":8}`))
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
	if !strings.Contains(presentation.String(), "✓ scan  128 checked") {
		t.Fatalf("presentation stream missing the task row:\n%s", presentation.String())
	}
}
