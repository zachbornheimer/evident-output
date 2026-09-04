package evo_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func TestCoalesce_SingleMatchingChanges_SuppressesTrailingConclusion(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{
		evo.Title("librarian"),
		evo.To(&buf),
		evo.Plain(),
		evo.NoColor(),
	}})
	t.Cleanup(func() { _ = out.Close() })

	out.Task("librarian").Record("placed", 1, "file")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	if strings.Count(got, "[changed]") != 1 {
		t.Fatalf("want one [changed] band, got:\n%s", got)
	}
	if !strings.Contains(got, "placed") {
		t.Fatalf("missing body:\n%s", got)
	}
	// Structured model still has conclusion.
	if out.Conclusion().State != evo.StateChanged {
		t.Fatalf("conclusion state = %v", out.Conclusion().State)
	}
}

func TestCoalesce_SingleMatchingPlan_SuppressesTrailingConclusion(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{
		evo.Title("librarian"),
		evo.To(&buf),
		evo.Plain(),
		evo.NoColor(),
		evo.DryRun(),
	}})
	t.Cleanup(func() { _ = out.Close() })

	out.Task("librarian").RecordName("move", "a → b")
	if err := out.Finish(); err != nil {
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
	out := evo.Init(evo.Config{
		Isolated: true,
		DryRun:   true,
		Subject:  "repo  /demo",
		Options:  []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()},
	})
	t.Cleanup(func() { _ = out.Close() })

	branches := out.Task("branches")
	_ = branches.Delete("local tip", nil, evo.Affected(2))
	branches.Done()

	worktrees := out.Task("worktrees")
	_ = worktrees.Remove("worktree", nil, evo.Affected(1))
	worktrees.Done()

	if err := out.Finish(); err != nil {
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
	out := evo.Init(evo.Config{
		Isolated: true,
		DryRun:   true,
		Subject:  "repo  /demo",
		Options:  []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()},
	})
	t.Cleanup(func() { _ = out.Close() })

	branches := out.Task("branches")
	branches.Warn("kept 13")
	_ = branches.Delete("local tip", nil, evo.Affected(2))
	branches.Done()

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	if !strings.Contains(got, "[planned · warned]") {
		t.Fatalf("warned dry-run must keep its trailing band:\n%s", got)
	}
}

func TestCoalesce_ChangedPlusFailure_KeepsConclusion(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{
		evo.Title("librarian"),
		evo.To(&buf),
		evo.Plain(),
		evo.NoColor(),
	}})
	t.Cleanup(func() { _ = out.Close() })

	out.Task("librarian").Record("placed", 7, "files")
	out.Task("placement", evo.ID("run.placement")).Fail("not writable", evo.On("arr/x"))
	if err := out.Finish(); err != nil {
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
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{
		evo.Title("tool"),
		evo.To(&buf),
		evo.Plain(),
		evo.NoColor(),
	}})
	t.Cleanup(func() { _ = out.Close() })

	_ = out.Task("files").Add("file", nil, evo.Affected(1))
	_ = out.Task("manifest").Update("entry", nil, evo.Affected(1))
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	// Two section headers + trailing conclusion.
	if strings.Count(buf.String(), "[changed]") < 3 {
		t.Fatalf("want multi-section + conclusion:\n%s", buf.String())
	}
}

func TestCoalesce_SubjectMismatch_KeepsConclusion(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{
		evo.Title("tool"),
		evo.To(&buf),
		evo.Plain(),
		evo.NoColor(),
	}})
	t.Cleanup(func() { _ = out.Close() })

	_ = out.Task("other-subject").Add("x", nil, evo.Affected(1))
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if strings.Count(buf.String(), "[changed]") < 2 {
		t.Fatalf("mismatch should keep conclusion:\n%s", buf.String())
	}
}

func TestCoalesce_NextCommand_KeepsConclusion(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{
		evo.Title("tool"),
		evo.To(&buf),
		evo.Plain(),
		evo.NoColor(),
	}})
	t.Cleanup(func() { _ = out.Close() })

	_ = out.Task("tool").Add("x", nil, evo.Affected(1))
	out.NextCommand("git", "status")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	// Conclusion band carries next action — must remain (or action visible).
	got := buf.String()
	if !strings.Contains(got, "git") {
		t.Fatalf("want next command visible:\n%s", got)
	}
}

func TestCoalesce_JSONStillHasConclusion(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{
		evo.Title("tool"),
		evo.To(io.Discard),
	}})
	t.Cleanup(func() { _ = out.Close() })
	_ = out.Task("tool").Add("x", nil, evo.Affected(1))
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	// Projection may suppress human footer; model retains conclusion + changes.
	if out.Conclusion().State != evo.StateChanged {
		t.Fatalf("conclusion state = %v", out.Conclusion().State)
	}
	snap := out.Snapshot()
	if snap.Conclusion == nil || len(snap.Changes) != 1 {
		t.Fatalf("snapshot must keep Conclusion and Changes: %#v", snap)
	}
}

func TestCoalesce_TitleWithoutSemanticReport_OmitsHumanConclusion(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{
		evo.Title("zq"),
		evo.To(&buf),
		evo.Plain(),
		evo.NoColor(),
	}})
	t.Cleanup(func() { _ = out.Close() })

	out.Println("zq v0.2.14")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	if strings.Contains(buf.String(), "[ready]") || strings.Contains(buf.String(), "[unchanged]") {
		t.Fatalf("title alone must not fabricate a human conclusion:\n%s", buf.String())
	}
	if out.Conclusion().State != evo.StateReady {
		t.Fatalf("title-only conclusion = %q, want ready (StateUnchanged was deleted, P1)", out.Conclusion().State)
	}
}

func TestCoalesce_SingleMatchingItem_OmitsRepeatedConclusion(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{
		evo.Title("database"),
		evo.To(&buf),
		evo.Plain(),
		evo.NoColor(),
	}})
	t.Cleanup(func() { _ = out.Close() })

	out.Task("database").Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	if strings.Contains(buf.String(), "[ready]") {
		t.Fatalf("matching item already carries the conclusion:\n%s", buf.String())
	}
	if strings.Count(buf.String(), "database") != 1 {
		t.Fatalf("matching subject must render once:\n%s", buf.String())
	}
}

func TestCoalesce_SingleItemForBroaderSubject_KeepsConclusion(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{
		evo.Title("release v1.4"),
		evo.To(&buf),
		evo.Plain(),
		evo.NoColor(),
	}})
	t.Cleanup(func() { _ = out.Close() })

	out.Task("release signature").Block("signature is missing")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(buf.String(), "[blocked]  release v1.4") {
		t.Fatalf("broader subject conclusion adds information:\n%s", buf.String())
	}
}

func TestCoalesce_MultipleItems_KeepsAggregateConclusion(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{
		evo.Title("repository"),
		evo.To(&buf),
		evo.Plain(),
		evo.NoColor(),
	}})
	t.Cleanup(func() { _ = out.Close() })

	out.Task("working tree").Done()
	out.Task("branches").Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(buf.String(), "[ready]  repository") {
		t.Fatalf("multiple results need an aggregate conclusion:\n%s", buf.String())
	}
}
