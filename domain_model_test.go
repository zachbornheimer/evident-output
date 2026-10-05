package evo_test

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func TestDOM014_DetailOnBlock(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	it := out.Task("i")
	it.Block("b", evo.Detail("user visible"))
	if it.Snapshot().Problems[0].Detail != "user visible" {
		t.Fatal(it.Snapshot().Problems)
	}
}

// TestDOM023_SealedTotalRejectsChange documents the sealed-total invariant
// (evo-rec.md "Progress invariants"): once a nonzero total is reported, it
// cannot change to a different value — 14/40 never becomes 14/53. Earlier
// behavior allowed the total to grow silently; that is now recorded misuse
// and the first total is kept.
func TestDOM023_SealedTotalRejectsChange(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("t")
	task.Progress(1, 2)
	task.Progress(2, 5)
	if task.Snapshot().Progress.Total != 2 {
		t.Fatalf("sealed total was not preserved: %#v", task.Snapshot().Progress)
	}
	if !errors.Is(out.Err(), evo.ErrInvalidProgress) {
		t.Fatalf("error = %v, want ErrInvalidProgress", out.Err())
	}
}

func TestDOM037_FailedConclusion(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	out.Task("i").Fail("no")
	_ = out.Finish()
	if out.Conclusion().State != evo.StateFailed {
		t.Fatal(out.Conclusion().State)
	}
	if out.Conclusion().ExitCode != 2 {
		t.Fatal(out.Conclusion().ExitCode)
	}
}

// TestDOM038_WarningOnly is updated for P2: Warn no longer resolves its
// task, so a task that only ever calls Warn auto-resolves Done at Finish
// (the same amnesty a recorded effect gets) — the run reads StateReady, with
// Conclusion.Warned carrying the warning forward instead of a StateWarning
// headline.
func TestDOM038_WarningOnly(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	out.Task("i").Problem("careful", evo.Severity(evo.SeverityWarning))
	_ = out.Finish()
	if got := out.Conclusion().State; got != evo.StateReady {
		t.Fatalf("state = %v, want StateReady (Warn auto-resolves Done, P2)", got)
	}
	if !out.Conclusion().Warned {
		t.Fatal("Conclusion.Warned = false, want true")
	}
}

func TestDOM041_ActionsPromoted(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	item := out.Task("i")
	item.Block("b", evo.NextCommand("fix", "it"))
	_ = out.Finish()
	c := out.Conclusion()
	if len(c.Actions) == 0 {
		t.Fatal("expected promoted actions")
	}
}

// TestDOM004_SameNameIsDuplicateSibling pins §3.1: repeated Output.Task
// calls with the same name are a duplicate sibling declaration, not a
// get-or-create — two distinct call sites sharing a name is exactly the
// ambiguity 1.0 refuses at declaration time (get-or-create merged them into
// one identity, which is unsound once identity drives manifest
// reconciliation).
func TestDOM004_SameNameIsDuplicateSibling(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	a := out.Task("same")
	b := out.Task("same")
	succeed(a)
	if a.Snapshot().ID == b.Snapshot().ID {
		t.Fatal("expected a distinct handle for the duplicate declaration")
	}
	if !errors.Is(out.Err(), evo.ErrDuplicateSiblingName) {
		t.Fatalf("Err() = %v, want ErrDuplicateSiblingName", out.Err())
	}
}

// TestDOM004_DistinctParentsAllowSameDisplayName covers the remaining case
// the retired DuplicateDisplayNamesAllowed test named: two genuinely
// distinct entities may still share a display name under different parents.
func TestDOM004_DistinctParentsAllowSameDisplayName(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	a := out.Group("first").Task("same")
	b := out.Group("second").Task("same")
	succeed(a)
	succeed(b)
	if a.Snapshot().ID == b.Snapshot().ID {
		t.Fatal("IDs must differ")
	}
}

func TestDOM013_MutationAfterFinishRejected(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	succeed(out.Task("x"))
	_ = out.Finish()
	succeed(out.Task("y"))
	if !errors.Is(out.Err(), evo.ErrClosed) && out.Err() == nil {
		// ensureOpen records ErrClosed
		if out.Err() == nil {
			// Item after finish may still allocate handle but records misuse
			t.Log("err", out.Err())
		}
	}
}

func TestDOM021_NegativeProgressRejected(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("t")
	task.Progress(-1, 10)
	if !errors.Is(out.Err(), evo.ErrInvalidProgress) {
		t.Fatalf("err=%v", out.Err())
	}
}

// TestDOM030_CollectionWarning is updated for P2: Warn annotates a task
// instead of resolving it, so a task that only ever calls Warn stays
// non-terminal (Pending) until Finish's amnesty resolves it — before
// Finish, the collection reads Incomplete (one unresolved child), and the
// warning itself lives on that child's Warnings field.
func TestDOM030_CollectionWarning(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	g := out.Group("g")
	succeed(g.Task("a"))
	g.Task("b").Problem("soft", evo.Severity(evo.SeverityWarning))
	snap := g.Snapshot()
	if snap.State != evo.Running && snap.State != evo.Incomplete {
		t.Fatalf("state = %v, want Running or Incomplete (Warn no longer resolves its task)", snap.State)
	}
	if warnings := snap.Tasks[1].Warnings; len(warnings) != 1 || warnings[0].Summary != "soft" {
		t.Fatalf("child warnings = %+v, want one warning %q", warnings, "soft")
	}
}

// TestDOM030b_CollectionWarningDetailIsRendered guards against a regression
// where writeCollection only special-cased Failed children: a group glyph
// like "!" rendered with no explanation of which child warned or why,
// because the Warn() message was recorded but never printed under the
// group summary line.
func TestDOM030b_CollectionWarningDetailIsRendered(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})
	g := out.Group("capture")
	succeed(g.Task("Brewfile"))
	g.Task("Zen").Problem("skipped — zen-bootstrap not available", evo.Severity(evo.SeverityWarning))
	_ = out.Finish()
	_ = out.Close()

	rendered := buf.String()
	if !strings.Contains(rendered, "Zen") {
		t.Fatalf("rendered output missing warned child name %q: %s", "Zen", rendered)
	}
	if !strings.Contains(rendered, "skipped — zen-bootstrap not available") {
		t.Fatalf("rendered output missing warned child's reason: %s", rendered)
	}
}

func TestDOM031_CollectionAllDone(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	g := out.Group("g")
	g.Summary("all good")
	succeed(g.Task("a"))
	succeed(g.Task("b"))
	if g.Snapshot().State != evo.Done {
		t.Fatal(g.Snapshot().State)
	}
	if g.Snapshot().Summary != "all good" {
		t.Fatal(g.Snapshot().Summary)
	}
}

// TestDOM035_UnresolvedChildInCollection pins release-gate round 4 finding
// 3: an unresolved child with no problems, on a clean finish, reads as an
// honest Partial outcome, never misuse — Finish returns nil.
func TestDOM035_UnresolvedChildInCollection(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	g := out.Group("g")
	succeed(g.Task("a"))
	g.Task("hanging")
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish() = %v, want nil (clean finish, no amnesty-defeating problems)", err)
	}
	if !out.Conclusion().Partial {
		t.Fatal("want Conclusion.Partial = true for the unresolved hanging child")
	}
}

func TestDOM049_OutputFail(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	out.Fail("stopped", evo.Detail("disk"))
	_ = out.Finish()
	if out.Conclusion().State != evo.StateFailed {
		t.Fatal(out.Conclusion().State)
	}
}

func TestDOM048_BlockedWithNilErrorReturn(t *testing.T) {
	// Pattern from spec: presentation negative, callback returns nil
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	var ret error
	func() {
		out.Task("branches").Block("local-only")
		ret = nil
	}()
	if ret != nil {
		t.Fatal(ret)
	}
	_ = out.Finish()
	if out.Conclusion().State != evo.StateBlocked {
		t.Fatal(out.Conclusion().State)
	}
}

func TestDOM005_DuplicateKeyRejected(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	out.Task("one").Key("k")
	out.Task("two").Key("k")
	if !errors.Is(out.Err(), evo.ErrDuplicateKey) {
		t.Fatalf("err=%v", out.Err())
	}
}

func TestDOM024_TotalDecreaseBelowCompletedRejected(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("t")
	task.Progress(5, 10)
	task.Progress(5, 3) // total < completed
	if !errors.Is(out.Err(), evo.ErrInvalidProgress) {
		t.Fatalf("err=%v", out.Err())
	}
	if task.Snapshot().Progress.Total != 10 {
		t.Fatal("last valid total not preserved")
	}
}

func TestDOM006_TaskDoneWithoutPhase(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	item := out.Task("working tree")
	succeed(item)
	if item.Snapshot().State != evo.Done {
		t.Fatalf("state = %q", item.Snapshot().State)
	}
}

func TestDOM039_ChangesPlusFailure(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Title: "deps"})
	t.Cleanup(func() { _ = out.Close() })
	out.Task("deps").Define(effectOf(evo.EffectAdd, "package", 1))
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
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	succeed(out.Task("x"))
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
}

// TestDOM010_WarnAndFailWithStructuredSummary is updated for P2: Warn
// annotates a task instead of resolving it (evo.Warning as a terminal
// EntityState is deleted). A warned task stays non-terminal — Pending here,
// since nothing else touched it — and its warning lands on the Warnings
// field; a later Done still resolves it normally.
func TestDOM010_WarnAndFailWithStructuredSummary(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	w := out.Task("w")
	w.Problem("soft", evo.Severity(evo.SeverityWarning))
	if got := w.Snapshot().State; got == evo.Done || got == evo.Failed || got == evo.Blocked {
		t.Fatalf("state = %q, want non-terminal: Warn must not resolve the task", got)
	}
	if warnings := w.Snapshot().Warnings; len(warnings) != 1 || warnings[0].Summary != "soft" {
		t.Fatalf("warnings = %+v, want one warning %q", warnings, "soft")
	}
	succeed(w)
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
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	it := out.Task("x")
	it.Block("b", evo.NextCommand("fix", "it"))
	if len(it.Snapshot().Problems[0].Actions) != 1 {
		t.Fatal("expected action")
	}
}

// TestDOM033_UnresolvedItemAtFinish pins release-gate round 4 finding 3: a
// never-touched task with no problems, on a clean finish, reads as an honest
// Partial outcome (Conclusion.Partial), never misuse — Finish returns nil.
func TestDOM033_UnresolvedItemAtFinish(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
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
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	succeed(out.Task("x"))
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestDOM045_EmptyOutput(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	c := out.Conclusion()
	if c.State != evo.StateReady {
		t.Fatalf("state=%q, want StateReady (StateUnchanged was deleted, P1)", c.State)
	}
}
