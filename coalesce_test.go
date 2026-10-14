package evo

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/change"
)

func TestCoalesce_SingleMatchingChanges_SuppressesTrailingConclusion(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, title: "librarian", color: colorNever, plain: true})
	t.Cleanup(func() { _ = out.close() })

	out.Task("librarian").Define(effectOf(Verb(change.EffectAdd), "file", 1))
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	if strings.Count(got, "[changed]") != 1 {
		t.Fatalf("want one [changed] band, got:\n%s", got)
	}
	if !strings.Contains(got, "added") {
		t.Fatalf("missing body:\n%s", got)
	}
	// Structured model still has conclusion.
	if out.conclusion().headline() != headlineChanged {
		t.Fatalf("conclusion state = %v", out.conclusion().headline())
	}
}

func TestCoalesce_SingleMatchingPlan_SuppressesTrailingConclusion(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, title: "librarian", color: colorNever, plain: true, DryRun: true})
	t.Cleanup(func() { _ = out.close() })

	out.Task("librarian").Define(effectOf(Update, "file", 1))
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	if strings.Count(buf.String(), "[planned]") != 1 {
		t.Fatalf("want one [planned]:\n%s", buf.String())
	}
}

// TestCoalesce_DryRunPlannedWithHeader_SuppressesTrailingConclusion is
// red-first against fixture-repo-retire-dryrun.md's binding reading: "NO
// ledger row for tasks/binaries with no effects (current `[planned]
// repo-retire` trailing row is a bug)". A dry-run run that already rendered
// its Config.Subject header and settled on a pure StatePlanned verdict
// across MULTIPLE effect sections (not just the single-section case
// TestCoalesce_SingleMatchingPlan_SuppressesTrailingConclusion already
// covers) must not also print a trailing "[planned]" band — the header plus
// the per-section ledger rows already told the whole story.
func TestCoalesce_DryRunPlannedWithHeader_SuppressesTrailingConclusion(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{DryRun: true, subject: "repo  /demo", stdout: &buf, color: colorNever, plain: true})
	t.Cleanup(func() { _ = out.close() })

	branches := out.Task("branches")
	branches.Define(effectOf(Delete, "local tip", 2))

	worktrees := out.Task("worktrees")
	worktrees.Define(effectOf(Verb(change.EffectRemove), "worktree", 1))

	if err := out.finish(); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	if strings.Count(got, "[planned]") != 2 {
		t.Fatalf("want exactly the two per-section [planned] rows, no trailing band:\n%s", got)
	}
}

// TestCoalesce_DryRunWarned_KeepsTrailingConclusion proves the suppression
// above does not overreach: a dry-run header plus a warned task still needs
// the trailing band to carry the "· warned" modifier, since the header and
// per-section ledger rows never show it.
func TestCoalesce_DryRunWarned_KeepsTrailingConclusion(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{DryRun: true, subject: "repo  /demo", stdout: &buf, color: colorNever, plain: true})
	t.Cleanup(func() { _ = out.close() })

	branches := out.Task("branches")
	branches.Problem("origin unreachable", severity(severityWarning))
	branches.Define(effectOf(Delete, "local tip", 2))

	if err := out.finish(); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	if !strings.Contains(got, "[planned · warned]") {
		t.Fatalf("warned dry-run must keep its trailing band:\n%s", got)
	}
}

func TestCoalesce_ChangedPlusFailure_KeepsConclusion(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, title: "librarian", color: colorNever, plain: true})
	t.Cleanup(func() { _ = out.close() })

	out.Task("librarian").Define(effectOf(Verb(change.EffectAdd), "file", 7))
	out.Task("placement").fail("not writable", On("arr/x"))
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "[failed]") {
		t.Fatalf("want failed conclusion:\n%s", got)
	}
	if !strings.Contains(got, "[changed]") {
		t.Fatalf("want changes section:\n%s", got)
	}
}

func TestCoalesce_MultipleChanges_KeepsConclusion(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, title: "tool", color: colorNever, plain: true})
	t.Cleanup(func() { _ = out.close() })

	out.Task("files").Define(effectOf(Verb(change.EffectAdd), "file", 1))
	out.Task("manifest").Define(effectOf(Update, "entry", 1))
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	// Two section headers + trailing conclusion.
	if strings.Count(buf.String(), "[changed]") < 3 {
		t.Fatalf("want multi-section + conclusion:\n%s", buf.String())
	}
}

func TestCoalesce_SubjectMismatch_KeepsConclusion(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, title: "tool", color: colorNever, plain: true})
	t.Cleanup(func() { _ = out.close() })

	out.Task("other-subject").Define(effectOf(Verb(change.EffectAdd), "x", 1))
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	if strings.Count(buf.String(), "[changed]") < 2 {
		t.Fatalf("mismatch should keep conclusion:\n%s", buf.String())
	}
}

func TestCoalesce_NextCommand_KeepsConclusion(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, title: "tool", color: colorNever, plain: true})
	t.Cleanup(func() { _ = out.close() })

	tool := out.Task("tool")
	tool.Define(effectOf(Verb(change.EffectAdd), "x", 1))
	tool.Problem("working tree state not checked", severity(severityWarning), nextCommand("git", "status"))
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	// Conclusion band carries next action — must remain (or action visible).
	got := buf.String()
	if !strings.Contains(got, "git") {
		t.Fatalf("want next command visible:\n%s", got)
	}
}

func TestCoalesce_JSONStillHasConclusion(t *testing.T) {
	out := initRun(Config{stdout: io.Discard, title: "tool"})
	t.Cleanup(func() { _ = out.close() })
	out.Task("tool").Define(effectOf(Verb(change.EffectAdd), "x", 1))
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	// Projection may suppress human footer; model retains conclusion + changes.
	if out.conclusion().headline() != headlineChanged {
		t.Fatalf("conclusion state = %v", out.conclusion().headline())
	}
	snap := out.snapshot()
	if snap.Conclusion == nil || len(snap.Changes) != 1 {
		t.Fatalf("snapshot must keep Conclusion and Changes: %#v", snap)
	}
}

func TestCoalesce_TitleWithoutSemanticReport_OmitsHumanConclusion(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, title: "zq", color: colorNever, plain: true})
	t.Cleanup(func() { _ = out.close() })

	out.println("zq v0.2.14")
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}

	if strings.Contains(buf.String(), "[succeeded]") || strings.Contains(buf.String(), "[unchanged]") {
		t.Fatalf("title alone must not fabricate a human conclusion:\n%s", buf.String())
	}
	if out.conclusion().headline() != headlineSucceeded {
		t.Fatalf("title-only conclusion = %q, want ready (StateUnchanged was deleted, P1)", out.conclusion().headline())
	}
}

func TestCoalesce_SingleMatchingItem_OmitsRepeatedConclusion(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, title: "database", color: colorNever, plain: true})
	t.Cleanup(func() { _ = out.close() })

	succeed(out.Task("database"))
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}

	if strings.Contains(buf.String(), "[succeeded]") {
		t.Fatalf("matching item already carries the conclusion:\n%s", buf.String())
	}
	if strings.Count(buf.String(), "database") != 1 {
		t.Fatalf("matching subject must render once:\n%s", buf.String())
	}
}

func TestCoalesce_SingleItemForBroaderSubject_KeepsConclusion(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, title: "release v1.4", color: colorNever, plain: true})
	t.Cleanup(func() { _ = out.close() })

	out.Task("release signature").block("signature is missing")
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(buf.String(), "[refused]  release v1.4") {
		t.Fatalf("broader subject conclusion adds information:\n%s", buf.String())
	}
}

func TestCoalesce_MultipleItems_KeepsAggregateConclusion(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, title: "repository", color: colorNever, plain: true})
	t.Cleanup(func() { _ = out.close() })

	succeed(out.Task("working tree"))
	succeed(out.Task("branches"))
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(buf.String(), "[succeeded]  repository") {
		t.Fatalf("multiple results need an aggregate conclusion:\n%s", buf.String())
	}
}
