package evo_test

import (
	"io"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

func TestDOM006_TaskDoneWithoutPhase(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })
	item := out.Task("working tree")
	item.Done()
	if item.Snapshot().State != evo.Done {
		t.Fatalf("state = %q", item.Snapshot().State)
	}
}

func TestDOM039_ChangesPlusFailure(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("deps"), evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })
	out.Changes("deps").Added(1, "package")
	out.Task("install").Fail("disk full")
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

// TestDOM046_CallerMutatesProblemSlice guarded a caller-supplied []Problem
// slice against aliasing (BlockedBy stored the slice by reference). That
// construction path is gone: Block/Fail/Warn build exactly one Problem
// inline (finish's []Problem{p} is always a fresh literal), so the aliasing
// bug this test caught is now structurally impossible rather than merely
// untested.

func TestDOM043_FinishTwice(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })
	out.Task("x").Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
}

func TestAPI001_MinimalItemExample(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("repo")}})
	defer func() { _ = out.Close() }()
	out.Task("working tree").Done()
	out.Task("branches").Block("local-only")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	testkit.RequireConclusion(t, out, evo.StateBlocked)
}

func TestConclusion_PlanOnlyIsPlanned(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("acct"), evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })
	out.Plan("delete").Delete(1, "thing")
	_ = out.Finish()
	testkit.RequireConclusion(t, out, evo.StatePlanned)
	testkit.RequireClean(t, out)
}

func TestDOM010_WarnAndFailWithStructuredSummary(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })
	w := out.Task("w")
	w.Warn("soft")
	if w.Snapshot().State != evo.Warning {
		t.Fatal(w.Snapshot().State)
	}
	f := out.Task("f")
	f.Fail("hard")
	if f.Snapshot().State != evo.Failed {
		t.Fatal(f.Snapshot().State)
	}
}

func TestDOM012_NextActionAfterResolve(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })
	it := out.Task("x")
	it.Block("b")
	it.NextCommand("fix", "it")
	if len(it.Snapshot().Actions) != 1 {
		t.Fatal("expected action")
	}
}

func TestDOM019_Advance(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard)}})
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
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })
	out.Task("hanging")
	err := out.Finish()
	if err == nil {
		t.Fatal("expected unresolved item error")
	}
}

func TestDOM044_CloseTwice(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard)}})
	out.Task("x").Done()
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestDOM045_EmptyOutput(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	c := out.Conclusion()
	if c.State != evo.StateUnchanged {
		t.Fatalf("state=%q", c.State)
	}
}
