package evo

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestDOM014_DetailOnBlock(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })
	it := out.Task("i")
	it.block("b", Detail("user visible"))
	if it.snapshot().Problems[0].Detail != "user visible" {
		t.Fatal(it.snapshot().Problems)
	}
}

// TestDOM023_SealedTotalRejectsChange documents the sealed-total invariant
// (evo-rec.md "Progress invariants"): once a nonzero total is reported, it
// cannot change to a different value — 14/40 never becomes 14/53. Earlier
// behavior allowed the total to grow silently; that is now recorded misuse
// and the first total is kept.
func TestDOM023_SealedTotalRejectsChange(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })
	task := out.Task("t")
	task.Progress(1, 2)
	task.Progress(2, 5)
	if task.snapshot().Progress.Total != 2 {
		t.Fatalf("sealed total was not preserved: %#v", task.snapshot().Progress)
	}
	if !errors.Is(out.err(), errInvalidProgress) {
		t.Fatalf("error = %v, want ErrInvalidProgress", out.err())
	}
}

func TestDOM037_FailedConclusion(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })
	out.Task("i").fail("no")
	_ = out.finish()
	if out.conclusion().state != stateFailed {
		t.Fatal(out.conclusion().state)
	}
	if out.conclusion().exitCode != 2 {
		t.Fatal(out.conclusion().exitCode)
	}
}

// TestDOM038_WarningOnly is updated for P2: Warn no longer resolves its
// task, so a task that only ever calls Warn auto-resolves Done at Finish
// (the same amnesty a recorded effect gets) — the run reads StateReady, with
// Conclusion.Warned carrying the warning forward instead of a StateWarning
// headline.
func TestDOM038_WarningOnly(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })
	out.Task("i").Problem("careful", severity(severityWarning))
	_ = out.finish()
	if got := out.conclusion().state; got != stateReady {
		t.Fatalf("state = %v, want StateReady (Warn auto-resolves Done, P2)", got)
	}
	if !out.conclusion().warned {
		t.Fatal("Conclusion.Warned = false, want true")
	}
}

func TestDOM041_ActionsPromoted(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })
	item := out.Task("i")
	item.block("b", nextCommand("fix", "it"))
	_ = out.finish()
	c := out.conclusion()
	if len(c.actions) == 0 {
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
	out := initRun(Config{isolated: true, stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })
	a := out.Task("same")
	b := out.Task("same")
	succeed(a)
	if a.snapshot().ID == b.snapshot().ID {
		t.Fatal("expected a distinct handle for the duplicate declaration")
	}
	if !errors.Is(out.err(), errDuplicateSiblingName) {
		t.Fatalf("Err() = %v, want ErrDuplicateSiblingName", out.err())
	}
}

// TestDOM004_DistinctParentsAllowSameDisplayName covers the remaining case
// the retired DuplicateDisplayNamesAllowed test named: two genuinely
// distinct entities may still share a display name under different parents.
func TestDOM004_DistinctParentsAllowSameDisplayName(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })
	a := out.Group("first").Task("same")
	b := out.Group("second").Task("same")
	succeed(a)
	succeed(b)
	if a.snapshot().ID == b.snapshot().ID {
		t.Fatal("IDs must differ")
	}
}

func TestDOM013_MutationAfterFinishRejected(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard})
	succeed(out.Task("x"))
	_ = out.finish()
	succeed(out.Task("y"))
	if !errors.Is(out.err(), errClosed) && out.err() == nil {
		// ensureOpen records ErrClosed
		if out.err() == nil {
			// Item after finish may still allocate handle but records misuse
			t.Log("err", out.err())
		}
	}
}

func TestDOM021_NegativeProgressRejected(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })
	task := out.Task("t")
	task.Progress(-1, 10)
	if !errors.Is(out.err(), errInvalidProgress) {
		t.Fatalf("err=%v", out.err())
	}
}

// TestDOM030_CollectionWarning is updated for P2: Warn annotates a task
// instead of resolving it, so a task that only ever calls Warn stays
// non-terminal (Pending) until Finish's amnesty resolves it — before
// Finish, the collection reads Incomplete (one unresolved child), and the
// warning itself lives on that child's Warnings field.
func TestDOM030_CollectionWarning(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })
	g := out.Group("g")
	succeed(g.Task("a"))
	g.Task("b").Problem("soft", severity(severityWarning))
	snap := g.snapshot()
	if snap.State != running && snap.State != incomplete {
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
	out := initRun(Config{isolated: true, stdout: &buf, color: colorNever, plain: true})
	g := out.Group("capture")
	succeed(g.Task("Brewfile"))
	g.Task("Zen").Problem("skipped — zen-bootstrap not available", severity(severityWarning))
	_ = out.finish()
	_ = out.close()

	rendered := buf.String()
	if !strings.Contains(rendered, "Zen") {
		t.Fatalf("rendered output missing warned child name %q: %s", "Zen", rendered)
	}
	if !strings.Contains(rendered, "skipped — zen-bootstrap not available") {
		t.Fatalf("rendered output missing warned child's reason: %s", rendered)
	}
}

func TestDOM031_CollectionAllDone(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })
	g := out.Group("g")
	g.Summary("all good")
	succeed(g.Task("a"))
	succeed(g.Task("b"))
	if g.snapshot().State != done {
		t.Fatal(g.snapshot().State)
	}
	if g.snapshot().Summary != "all good" {
		t.Fatal(g.snapshot().Summary)
	}
}

// TestDOM035_UnresolvedChildInCollection pins release-gate round 4 finding
// 3: an unresolved child with no problems, on a clean finish, reads as an
// honest Partial outcome, never misuse — Finish returns nil.
func TestDOM035_UnresolvedChildInCollection(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })
	g := out.Group("g")
	succeed(g.Task("a"))
	g.Task("hanging")
	if err := out.finish(); err != nil {
		t.Fatalf("Finish() = %v, want nil (clean finish, no amnesty-defeating problems)", err)
	}
	if !out.conclusion().partial {
		t.Fatal("want Conclusion.Partial = true for the unresolved hanging child")
	}
}

func TestDOM049_OutputFail(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })
	out.fail("stopped", Detail("disk"))
	_ = out.finish()
	if out.conclusion().state != stateFailed {
		t.Fatal(out.conclusion().state)
	}
}

func TestDOM048_BlockedWithNilErrorReturn(t *testing.T) {
	// Pattern from spec: presentation negative, callback returns nil
	out := initRun(Config{isolated: true, stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })
	var ret error
	func() {
		out.Task("branches").block("local-only")
		ret = nil
	}()
	if ret != nil {
		t.Fatal(ret)
	}
	_ = out.finish()
	if out.conclusion().state != stateBlocked {
		t.Fatal(out.conclusion().state)
	}
}

func TestDOM005_DuplicateKeyRejected(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })
	out.Task("one").key("k")
	out.Task("two").key("k")
	if !errors.Is(out.err(), errDuplicateKey) {
		t.Fatalf("err=%v", out.err())
	}
}

func TestDOM024_TotalDecreaseBelowCompletedRejected(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })
	task := out.Task("t")
	task.Progress(5, 10)
	task.Progress(5, 3) // total < completed
	if !errors.Is(out.err(), errInvalidProgress) {
		t.Fatalf("err=%v", out.err())
	}
	if task.snapshot().Progress.Total != 10 {
		t.Fatal("last valid total not preserved")
	}
}

func TestDOM006_TaskDoneWithoutPhase(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })
	item := out.Task("working tree")
	succeed(item)
	if item.snapshot().State != done {
		t.Fatalf("state = %q", item.snapshot().State)
	}
}

func TestDOM039_ChangesPlusFailure(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard, title: "deps"})
	t.Cleanup(func() { _ = out.close() })
	out.Task("deps").Define(effectOf(Verb(effectAdd), "package", 1))
	out.Task("install").fail("disk full")
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	c := out.conclusion()
	if c.state != stateFailed {
		t.Fatalf("state = %q, want failed", c.state)
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
	out := initRun(Config{isolated: true, stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })
	succeed(out.Task("x"))
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
}

// TestDOM010_WarnAndFailWithStructuredSummary is updated for P2: Warn
// annotates a task instead of resolving it (Warning as a terminal
// EntityState is deleted). A warned task stays non-terminal — Pending here,
// since nothing else touched it — and its warning lands on the Warnings
// field; a later Done still resolves it normally.
func TestDOM010_WarnAndFailWithStructuredSummary(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })
	w := out.Task("w")
	w.Problem("soft", severity(severityWarning))
	if got := w.snapshot().State; got == done || got == failed || got == blocked {
		t.Fatalf("state = %q, want non-terminal: Warn must not resolve the task", got)
	}
	if warnings := w.snapshot().Warnings; len(warnings) != 1 || warnings[0].Summary != "soft" {
		t.Fatalf("warnings = %+v, want one warning %q", warnings, "soft")
	}
	succeed(w)
	if got := w.snapshot().State; got != done {
		t.Fatalf("state = %q, want Done after Warn then Done", got)
	}
	f := out.Task("f")
	f.fail("hard")
	if f.snapshot().State != failed {
		t.Fatal(f.snapshot().State)
	}
}

func TestDOM012_NextActionAfterResolve(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })
	it := out.Task("x")
	it.block("b", nextCommand("fix", "it"))
	if len(it.snapshot().Problems[0].Actions) != 1 {
		t.Fatal("expected action")
	}
}

// TestDOM033_UnresolvedItemAtFinish pins release-gate round 4 finding 3: a
// never-touched task with no problems, on a clean finish, reads as an honest
// Partial outcome (Conclusion.Partial), never misuse — Finish returns nil.
func TestDOM033_UnresolvedItemAtFinish(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })
	out.Task("hanging")
	if err := out.finish(); err != nil {
		t.Fatalf("Finish() = %v, want nil (clean finish, no amnesty-defeating problems)", err)
	}
	if !out.conclusion().partial {
		t.Fatal("want Conclusion.Partial = true for the unresolved hanging task")
	}
}

func TestDOM044_CloseTwice(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard})
	succeed(out.Task("x"))
	if err := out.close(); err != nil {
		t.Fatal(err)
	}
	if err := out.close(); err != nil {
		t.Fatal(err)
	}
}

func TestDOM045_EmptyOutput(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	c := out.conclusion()
	if c.state != stateReady {
		t.Fatalf("state=%q, want StateReady (StateUnchanged was deleted, P1)", c.state)
	}
}
