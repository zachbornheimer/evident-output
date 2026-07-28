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
	out := evo.NewWithOptions(
		evo.Title("librarian"),
		evo.To(&buf),
		evo.Plain(),
		evo.NoColor(),
	)
	t.Cleanup(func() { _ = out.Close() })

	out.Changes("librarian").Record("placed", 1, "file")
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
	out := evo.NewWithOptions(
		evo.Title("librarian"),
		evo.To(&buf),
		evo.Plain(),
		evo.NoColor(),
	)
	t.Cleanup(func() { _ = out.Close() })

	out.Plan("librarian").Move("a", "b")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if strings.Count(buf.String(), "[planned]") != 1 {
		t.Fatalf("want one [planned]:\n%s", buf.String())
	}
}

func TestCoalesce_ChangedPlusFailure_KeepsConclusion(t *testing.T) {
	var buf bytes.Buffer
	out := evo.NewWithOptions(
		evo.Title("librarian"),
		evo.To(&buf),
		evo.Plain(),
		evo.NoColor(),
	)
	t.Cleanup(func() { _ = out.Close() })

	out.Changes("librarian").Record("placed", 7, "files")
	out.Item("placement", evo.ID("run.placement")).FailedBy(evo.Problem{
		Subject: "arr/x",
		Summary: "not writable",
	})
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
	out := evo.NewWithOptions(
		evo.Title("tool"),
		evo.To(&buf),
		evo.Plain(),
		evo.NoColor(),
	)
	t.Cleanup(func() { _ = out.Close() })

	out.Changes("files").Added(1, "file")
	out.Changes("manifest").Updated(1, "entry")
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
	out := evo.NewWithOptions(
		evo.Title("tool"),
		evo.To(&buf),
		evo.Plain(),
		evo.NoColor(),
	)
	t.Cleanup(func() { _ = out.Close() })

	out.Changes("other-subject").Added(1, "x")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if strings.Count(buf.String(), "[changed]") < 2 {
		t.Fatalf("mismatch should keep conclusion:\n%s", buf.String())
	}
}

func TestCoalesce_NextCommand_KeepsConclusion(t *testing.T) {
	var buf bytes.Buffer
	out := evo.NewWithOptions(
		evo.Title("tool"),
		evo.To(&buf),
		evo.Plain(),
		evo.NoColor(),
	)
	t.Cleanup(func() { _ = out.Close() })

	out.Changes("tool").Added(1, "x")
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
	out := evo.NewWithOptions(
		evo.Title("tool"),
		evo.To(io.Discard),
	)
	t.Cleanup(func() { _ = out.Close() })
	out.Changes("tool").Added(1, "x")
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
