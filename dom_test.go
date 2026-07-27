package evo_test

import (
	"io"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

func TestDOM006_ItemOKWithoutStart(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	item := out.Item("working tree")
	item.OK()
	if item.Snapshot().State != evo.OK {
		t.Fatalf("state = %q", item.Snapshot().State)
	}
}

func TestDOM039_ChangesPlusFailure(t *testing.T) {
	out := evo.For("deps", evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	out.Changes("deps").Added(1, "package")
	out.Item("install").Fail("disk full")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	c := out.Conclusion()
	if c.State != evo.StateFailed {
		t.Fatalf("state = %q, want failed", c.State)
	}
	if !c.Changed {
		t.Fatal("expected Changed=true with changes present")
	}
}

func TestDOM046_CallerMutatesProblemSlice(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	problems := []evo.Problem{{Subject: "a", Summary: "s", Count: 1}}
	item := out.Item("branches")
	item.BlockedBy(problems...)
	problems[0].Summary = "mutated"
	if item.Snapshot().Problems[0].Summary != "s" {
		t.Fatal("defensive copy failed")
	}
}

func TestDOM043_FinishTwice(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	out.Item("x").OK()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
}

func TestAPI001_MinimalItemExample(t *testing.T) {
	out := evo.For("repo")
	defer out.Close()
	out.Item("working tree").OK()
	out.Item("branches").Block("local-only")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	testkit.RequireConclusion(t, out, evo.StateBlocked)
}

func TestConclusion_PlanOnlyIsPlanned(t *testing.T) {
	out := evo.For("acct", evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	out.Plan("delete").Delete(1, "thing")
	_ = out.Finish()
	testkit.RequireConclusion(t, out, evo.StatePlanned)
	testkit.RequireClean(t, out)
}

func TestDOM010_WarnedByAndFailedBy(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	w := out.Item("w")
	w.WarnedBy(evo.Problem{Summary: "soft"})
	if w.Snapshot().State != evo.Warning {
		t.Fatal(w.Snapshot().State)
	}
	f := out.Item("f")
	f.FailedBy(evo.Problem{Summary: "hard"})
	if f.Snapshot().State != evo.Failed {
		t.Fatal(f.Snapshot().State)
	}
}

func TestDOM012_AnnotationAfterResolve(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	it := out.Item("x")
	it.Block("b").Because("why").NextCommand("fix", "it")
	if it.Snapshot().Because != "why" {
		t.Fatal(it.Snapshot().Because)
	}
	if len(it.Snapshot().Actions) != 1 {
		t.Fatal("expected action")
	}
}

func TestDOM019_Advance(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("files")
	task.Progress(0, 3)
	task.Advance(1)
	task.Advance(1)
	if task.Snapshot().Progress.Completed != 2 {
		t.Fatalf("got %d", task.Snapshot().Progress.Completed)
	}
}

func TestDOM033_UnresolvedItemAtFinish(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	out.Item("hanging")
	err := out.Finish()
	if err == nil {
		t.Fatal("expected unresolved item error")
	}
}

func TestDOM044_CloseTwice(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	out.Item("x").OK()
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestDOM045_EmptyOutput(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	c := out.Conclusion()
	if c.State != evo.StateUnchanged {
		t.Fatalf("state=%q", c.State)
	}
}
