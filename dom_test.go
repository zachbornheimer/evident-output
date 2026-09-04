package evo_test

import (
	"io"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

func TestDOM006_TaskDoneWithoutPhase(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })
	item := out.Task("working tree")
	item.Done()
	if item.Snapshot().State != evo.Done {
		t.Fatalf("state = %q", item.Snapshot().State)
	}
}

func TestDOM039_ChangesPlusFailure(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Title("deps"), evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })
	_ = out.Task("deps").Add("package", nil, evo.Affected(1))
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
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
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
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Title("repo")}})
	defer func() { _ = out.Close() }()
	out.Task("working tree").Done()
	out.Task("branches").Block("local-only")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	testkit.RequireConclusion(t, out, evo.StateBlocked)
}

func TestConclusion_PlanOnlyIsPlanned(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Title("acct"), evo.To(io.Discard), evo.DryRun()}})
	t.Cleanup(func() { _ = out.Close() })
	_ = out.Task("delete").Delete("thing", nil, evo.Affected(1))
	_ = out.Finish()
	testkit.RequireConclusion(t, out, evo.StatePlanned)
	testkit.RequireClean(t, out)
}

// TestDOM010_WarnAndFailWithStructuredSummary is updated for P2: Warn
// annotates a task instead of resolving it (evo.Warning as a terminal
// EntityState is deleted). A warned task stays non-terminal — Pending here,
// since nothing else touched it — and its warning lands on the Warnings
// field; a later Done still resolves it normally.
func TestDOM010_WarnAndFailWithStructuredSummary(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })
	w := out.Task("w")
	w.Warn("soft")
	if got := w.Snapshot().State; got != evo.Pending {
		t.Fatalf("state = %q, want Pending: Warn must not resolve the task", got)
	}
	if warnings := w.Snapshot().Warnings; len(warnings) != 1 || warnings[0].Summary != "soft" {
		t.Fatalf("warnings = %+v, want one warning %q", warnings, "soft")
	}
	w.Done()
	if got := w.Snapshot().State; got != evo.Done {
		t.Fatalf("state = %q, want Done after Warn then Done", got)
	}
	f := out.Task("f")
	f.Fail("hard")
	if f.Snapshot().State != evo.Failed {
		t.Fatal(f.Snapshot().State)
	}
}

func TestDOM012_NextActionAfterResolve(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })
	it := out.Task("x")
	it.Block("b")
	it.NextCommand("fix", "it")
	if len(it.Snapshot().Actions) != 1 {
		t.Fatal("expected action")
	}
}

// TestDOM033_UnresolvedItemAtFinish pins release-gate round 4 finding 3: a
// never-touched task with no problems, on a clean finish, reads as an honest
// Partial outcome (Conclusion.Partial), never misuse — Finish returns nil.
func TestDOM033_UnresolvedItemAtFinish(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })
	out.Task("hanging")
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish() = %v, want nil (clean finish, no amnesty-defeating problems)", err)
	}
	if !out.Conclusion().Partial {
		t.Fatal("want Conclusion.Partial = true for the unresolved hanging task")
	}
}

func TestDOM044_CloseTwice(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
	out.Task("x").Done()
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestDOM045_EmptyOutput(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	c := out.Conclusion()
	if c.State != evo.StateReady {
		t.Fatalf("state=%q, want StateReady (StateUnchanged was deleted, P1)", c.State)
	}
}
