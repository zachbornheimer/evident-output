package evo

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/internal/change"
)

func TestCoalesce_SingleMatchingChanges_SuppressesTrailingConclusion(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{Name: "librarian", Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})
	t.Cleanup(func() { _ = out.close() })

	out.Task("librarian").Define(engineEffectOf(change.EffectAdd, "file", 1))
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
	out := initRun(Config{Name: "librarian", DryRun: true, Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})
	t.Cleanup(func() { _ = out.close() })

	out.Task("librarian").Define(effectOf(Update, "file", 1))
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	if strings.Count(buf.String(), "[planned]") != 1 {
		t.Fatalf("want one [planned]:\n%s", buf.String())
	}
}

func TestCoalesce_ChangedPlusFailure_KeepsConclusion(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{Name: "librarian", Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})
	t.Cleanup(func() { _ = out.close() })

	out.Task("librarian").Define(engineEffectOf(change.EffectAdd, "file", 7))
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
	out := initRun(Config{Name: "tool", Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})
	t.Cleanup(func() { _ = out.close() })

	out.Task("files").Define(engineEffectOf(change.EffectAdd, "file", 1))
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
	out := initRun(Config{Name: "tool", Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})
	t.Cleanup(func() { _ = out.close() })

	out.Task("other-subject").Define(engineEffectOf(change.EffectAdd, "x", 1))
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	if strings.Count(buf.String(), "[changed]") < 2 {
		t.Fatalf("mismatch should keep conclusion:\n%s", buf.String())
	}
}

func TestCoalesce_NextCommand_KeepsConclusion(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{Name: "tool", Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})
	t.Cleanup(func() { _ = out.close() })

	// The Task stays unresolved until the Problem is recorded: a Problem on
	// a settled Task is misuse, so the callback waits on a gate this test
	// opens after Problem instead of racing the scheduler.
	problemRecorded := make(chan struct{})
	effect := engineEffectOf(change.EffectAdd, "x", 1)
	tool := out.Task("tool")
	tool.Define(func(ctx context.Context) error {
		<-problemRecorded
		return effect(ctx)
	})
	tool.Problem("working tree state not checked", nextCommand("git", "status"))
	close(problemRecorded)
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
	out := initRun(Config{Name: "tool", Facades: Facades{Stderr: io.Discard}})
	t.Cleanup(func() { _ = out.close() })
	out.Task("tool").Define(engineEffectOf(change.EffectAdd, "x", 1))
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
	out := initRun(Config{Name: "zq", Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})
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
	out := initRun(Config{Name: "database", Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})
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
	out := initRun(Config{Name: "release v1.4", Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})
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
	out := initRun(Config{Name: "repository", Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})
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
