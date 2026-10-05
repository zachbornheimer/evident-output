package engine

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// assertFrame fails unless the whole plain output equals want: a ledger
// fold is a layout promise, so the exact frame is the assertion.
func assertFrame(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("frame mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestLedgerFoldFrameZqShapeApply(t *testing.T) {
	got, _ := runPlain(t, Config{}, func(o *Output) {
		declareCentralize(o, 2, func(string, int) func(context.Context) error { return centralizeEffect(EffectUpdate, "package") })
	})
	assertFrame(t, got, "✓ npm pkg 0\n✓ npm pkg 1\n✓ composer pkg 0\n✓ composer pkg 1\n\n[changed] centralize packages  updated 4 packages\n\n[changed]\n")
}

func TestLedgerFoldFrameZqShapeDryRun(t *testing.T) {
	got, _ := runPlain(t, Config{DryRun: true}, func(o *Output) {
		declareCentralize(o, 2, func(string, int) func(context.Context) error { return centralizeEffect(EffectUpdate, "package") })
	})
	assertFrame(t, got, "[dry-run] no changes will be made\n\n✓ npm pkg 0\n✓ npm pkg 1\n✓ composer pkg 0\n✓ composer pkg 1\n\n[planned] centralize packages  update 4 packages\n\n[planned]\n")
}

func TestLedgerFoldFrameSequence(t *testing.T) {
	got, _ := runPlain(t, Config{}, func(o *Output) {
		s := o.Sequence("steps")
		for _, name := range []string{"a", "b"} {
			s.Task(name).Define(centralizeEffect(EffectDelete, "cache"))
		}
	})
	assertFrame(t, got, "✓ steps\n   ✓ a\n   ✓ b\n\n[changed] steps  deleted 2 caches\n\n[changed]\n")
}

// TestLedgerFoldCountsCommittedPartOfPartialEffect proves a failed item that
// reports PartialEffect counts its committed part (1 of 2), while it still
// shows as a Failed Task row.
func TestLedgerFoldCountsCommittedPartOfPartialEffect(t *testing.T) {
	got, _ := runPlain(t, Config{}, func(o *Output) {
		g := o.Group("npm")
		for i := range 3 {
			g.Task(string(rune('a' + i))).Define(func(ctx context.Context) error {
				return Effect(ctx, EffectSpec{Verb: EffectUpdate, Object: "package", Quantity: 2}, func(context.Context) error {
					if i == 1 {
						return PartialEffect(1, context.DeadlineExceeded)
					}
					return nil
				})
			})
		}
	})
	assertFrame(t, got, "✓ a\n✗ b  context deadline exceeded\n✓ c\n\n[changed] npm  updated 5 packages\n\n[failed]\n!  already mutated: 5 packages updated\n")
}

// Probe A: one different sibling must not stop the rest folding.
func TestLedgerFoldMixedContainerFoldsItsLikeChildren(t *testing.T) {
	got, _ := runPlain(t, Config{}, func(o *Output) {
		g := o.Group("npm")
		for i := range 5 {
			g.Task(fmt.Sprintf("pkg %d", i)).Define(effectOf(EffectUpdate, "package", 1))
		}
		g.Task("lock").Define(effectOf(EffectDelete, "lockfile", 1))
	})
	want := "[changed] npm   updated 5 packages\n[changed] lock  deleted 1 lockfile\n"
	if !strings.Contains(got, want) {
		t.Errorf("ledger = %q, want it to contain %q", got, want)
	}
}

// Probe C: a wrapper whose categories all do the same thing is one row.
func TestLedgerFoldLiftsToHighestUniformContainer(t *testing.T) {
	got, _ := runPlain(t, Config{}, func(o *Output) {
		w := o.Group("clean")
		for _, category := range []string{"local branches", "remote branches"} {
			g := w.Group(category)
			for i := range 3 {
				g.Task(fmt.Sprintf("%s %d", category, i)).Define(effectOf(EffectDelete, "branch", 1))
			}
		}
	})
	if want := "[changed] clean  deleted 6 branches\n"; !strings.Contains(got, want) {
		t.Errorf("ledger = %q, want it to contain %q", got, want)
	}
}

// Probe B: two folded rows sharing a container name are told apart.
func TestLedgerFoldQualifiesDuplicateContainerSubjects(t *testing.T) {
	got, _ := runPlain(t, Config{}, func(o *Output) {
		for _, repo := range []string{"alpha", "beta"} {
			r := o.Group(repo)
			r.Task(repo).Define(effectOf(EffectDelete, "worktree", 1))
			b := r.Group("branches")
			for i := range 3 {
				b.Task(fmt.Sprintf("%s b%d", repo, i)).Define(effectOf(EffectDelete, "branch", 1))
			}
		}
	})
	want := "[changed] alpha             deleted 1 worktree\n" +
		"[changed] alpha › branches  deleted 3 branches\n" +
		"[changed] beta              deleted 1 worktree\n" +
		"[changed] beta › branches   deleted 3 branches\n"
	if !strings.Contains(got, want) {
		t.Errorf("ledger = %q, want it to contain %q", got, want)
	}
}

// TestLedgerFoldOwnTaskAndItemsShareOneSubject proves a Group's own Task row
// (the Task named for its Group) and the folded row of its mixed items read
// as one category: same subject, not "branches" beside "clean › branches".
func TestLedgerFoldOwnTaskAndItemsShareOneSubject(t *testing.T) {
	got, _ := runPlain(t, Config{}, func(o *Output) {
		g := o.Group("clean").Group("branches")
		g.Task("branches").Define(effectOf(EffectDelete, "worktree", 1))
		for i := range 3 {
			g.Task(fmt.Sprintf("b%d", i)).Define(effectOf(EffectDelete, "branch", 1))
		}
	})
	assertFrame(t, got, "✓ branches\n✓ b0\n✓ b1\n✓ b2\n\n"+
		"[changed] branches  deleted 1 worktree\n[changed] branches  deleted 3 branches\n\n[changed]\n")
}
