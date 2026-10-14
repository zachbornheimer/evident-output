package evo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/internal/change"
	"github.com/zachbornheimer/evident-output/v2/internal/graph"
)

// effectRun runs one Task whose Define calls Effect(spec, fn) and
// returns the Output (finished), the rendered text, and Effect's error.
func effectRun(t *testing.T, dryRun bool, spec EffectSpec, fn func(context.Context) error) (*Run, string, error) {
	t.Helper()
	var buf bytes.Buffer
	out := initRun(Config{Name: "prune", DryRun: dryRun, Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})
	t.Cleanup(func() { _ = out.close() })
	var effectErr error
	out.Task("worktrees").Define(func(ctx context.Context) error {
		effectErr = Effect(ctx, spec, fn)
		return effectErr
	})
	_ = out.finish()
	return out, strings.Join(strings.Fields(buf.String()), " "), effectErr
}

var worktreeDelete = EffectSpec{Verb: Delete, Object: "worktree", Quantity: 3}

func TestEffect_DryRunRecordsPlannedAndNeverInvokesFn(t *testing.T) {
	t.Parallel()
	invoked := false
	out, got, err := effectRun(t, true, worktreeDelete, func(context.Context) error {
		invoked = true
		return nil
	})
	if err != nil {
		t.Fatalf("Effect err = %v, want nil", err)
	}
	if invoked {
		t.Fatal("dry run invoked the Effect callback")
	}
	snap := out.snapshot()
	if len(snap.Plans) != 1 || len(snap.Changes) != 0 {
		t.Fatalf("plan=%d changes=%d, want 1 planned Effect and no changes", len(snap.Plans), len(snap.Changes))
	}
	if !strings.Contains(got, "delete 3 worktrees") || strings.Contains(got, "deleted") {
		t.Fatalf("want planned imperative 'delete 3 worktrees', got:\n%s", got)
	}
}

func TestEffect_ApplyRecordsChangedOnlyAfterFnSucceeds(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := initRun(Config{Name: "prune", Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})
	t.Cleanup(func() { _ = out.close() })
	changesDuringFn := -1
	var effectErr error
	out.Task("worktrees").Define(func(ctx context.Context) error {
		effectErr = Effect(ctx, worktreeDelete, func(context.Context) error {
			changesDuringFn = len(out.snapshot().Changes)
			return nil
		})
		return effectErr
	})
	if err := out.finish(); err != nil || effectErr != nil {
		t.Fatalf("Finish err = %v, Effect err = %v, want nil", err, effectErr)
	}
	if changesDuringFn != 0 {
		t.Fatalf("changes while fn ran = %d, want 0 (recorded before fn returned)", changesDuringFn)
	}
	snap := out.snapshot()
	if len(snap.Changes) != 1 || len(snap.Plans) != 0 {
		t.Fatalf("changes=%d plan=%d, want 1 changed Effect and no plan", len(snap.Changes), len(snap.Plans))
	}
	if got := strings.Join(strings.Fields(buf.String()), " "); !strings.Contains(got, "deleted 3 worktrees") {
		t.Fatalf("want 'deleted 3 worktrees', got:\n%s", got)
	}
}

func TestEffect_ApplyFailureRecordsNothingAndReturnsFnError(t *testing.T) {
	t.Parallel()
	boom := errors.New("git worktree remove: locked")
	out, _, err := effectRun(t, false, worktreeDelete, func(context.Context) error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("Effect err = %v, want fn's error", err)
	}
	if snap := out.snapshot(); len(snap.Changes) != 0 {
		t.Fatalf("changes = %d, want 0 after a failed callback", len(snap.Changes))
	}
}

func TestEffect_FnReceivesSchedulerOwnedContext(t *testing.T) {
	t.Parallel()
	out := initRun(Config{Name: "prune"}, tuning{color: colorNever})
	t.Cleanup(func() { _ = out.close() })
	var defineCtx, fnCtx context.Context
	out.Task("worktrees").Define(func(ctx context.Context) error {
		defineCtx = ctx
		return Effect(ctx, worktreeDelete, func(ctx context.Context) error {
			fnCtx = ctx
			return nil
		})
	})
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	if fnCtx == nil || fnCtx != defineCtx {
		t.Fatalf("fn ctx %v is not the Define (scheduler-owned) ctx %v", fnCtx, defineCtx)
	}
}

func TestEffect_RejectsContentFreeSpec(t *testing.T) {
	t.Parallel()
	ok := func(context.Context) error { return nil }
	cases := []struct {
		name string
		spec EffectSpec
		fn   func(context.Context) error
		want error
	}{
		{"zero quantity", EffectSpec{Verb: Delete, Object: "worktree"}, ok, change.ErrEffectQuantityNotPositive},
		{"negative quantity", EffectSpec{Verb: Delete, Object: "worktree", Quantity: -1}, ok, change.ErrEffectQuantityNotPositive},
		{"missing verb", EffectSpec{Object: "worktree", Quantity: 1}, ok, change.ErrEffectVerbInvalid},
		{"unnamed verb", EffectSpec{Verb: Verb(99), Object: "worktree", Quantity: 1}, ok, change.ErrEffectVerbInvalid},
		{"missing object", EffectSpec{Verb: Delete, Quantity: 1}, ok, change.ErrEffectObjectMissing},
		{"blank object", EffectSpec{Verb: Delete, Object: "  ", Quantity: 1}, ok, change.ErrEffectObjectMissing},
		{"nil callback", worktreeDelete, nil, change.ErrEffectCallbackMissing},
	}
	for _, tc := range cases {
		for _, dryRun := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/dryRun=%v", tc.name, dryRun), func(t *testing.T) {
				t.Parallel()
				invoked := false
				fn := tc.fn
				if fn != nil {
					fn = func(ctx context.Context) error { invoked = true; return tc.fn(ctx) }
				}
				out, _, err := effectRun(t, dryRun, tc.spec, fn)
				if !errors.Is(err, tc.want) {
					t.Fatalf("dryRun=%v: err = %v, want %v", dryRun, err, tc.want)
				}
				if invoked {
					t.Fatalf("dryRun=%v: rejected Effect invoked its callback", dryRun)
				}
				if snap := out.snapshot(); len(snap.Plans)+len(snap.Changes) != 0 {
					t.Fatalf("dryRun=%v: rejected Effect recorded plan=%d changes=%d", dryRun, len(snap.Plans), len(snap.Changes))
				}
			})
		}
	}
}

func TestEffect_OutsideDefineReturnsErrNoTaskContext(t *testing.T) {
	t.Parallel()
	invoked := false
	err := Effect(context.Background(), worktreeDelete, func(context.Context) error {
		invoked = true
		return nil
	})
	if !errors.Is(err, graph.ErrNoTaskContext) || invoked {
		t.Fatalf("err = %v invoked = %v, want ErrNoTaskContext and no call", err, invoked)
	}
}

// TestWriteEffects_BoundedRows_500Records is red-first for item 3: a plan
// section with 500 records renders a bounded number of visible rows plus one
// dim overflow line, while the full 500 remain in the snapshot untouched.
// Each record names a distinct branch — release-gate round 3 finding 6
// merges identical (verb, object) records into one summed row, so the
// bounded-rows overflow this test proves needs 500 distinct rows to exercise.
func TestWriteEffects_BoundedRows_500Records(t *testing.T) {
	var buf strings.Builder
	out := initRun(Config{DryRun: true, Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})
	const total = 500
	commit(out.Task("branches"), distinctBranchDeletes(total)...)
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	snap := out.snapshot()
	if len(snap.Plans) != 1 || len(snap.Plans[0].Records) != total {
		t.Fatalf("snapshot must retain all %d records, got %+v", total, snap.Plans)
	}
	got := buf.String()
	visibleRows := strings.Count(got, "feat/branch")
	if visibleRows >= total {
		t.Fatalf("human view must bound visible rows, rendered all %d", visibleRows)
	}
	if !strings.Contains(got, "+495 more (not shown)") {
		t.Fatalf("want bounded-rows overflow line, got:\n%s", got)
	}
}

// TestEffect_OnResolvedTaskReturnsErrorNeverNil proves a
// mutation verb called after the task already resolved (Done) never
// executes the call and never silently swallows the misuse as a nil error.
func TestEffect_OnResolvedTaskReturnsErrorNeverNil(t *testing.T) {
	out := initRun(Config{Output: Plain}, tuning{color: colorNever})
	t.Cleanup(func() { _ = out.close() })

	task := out.Task("branches")
	succeed(task)
	task.Define(effectOf(Delete, "stale local branch", 1))
	if !errors.Is(out.err(), errAlreadyResolved) {
		t.Fatalf("Err() = %v, want ErrAlreadyResolved for a mutation on an already-resolved task", out.err())
	}
}

// TestEffect_ConcurrentSummaryDuringCallDoesNotDropEffect proves
// the ledger target resolves once: a concurrent Summary racing an Effect's
// in-flight call must not cause the effect that call just committed
// to be silently dropped as spurious misuse.
func TestEffect_ConcurrentSummaryDuringCallDoesNotDropEffect(t *testing.T) {
	out := initRun(Config{Output: Plain}, tuning{color: colorNever})
	t.Cleanup(func() { _ = out.close() })

	started := make(chan struct{})
	release := make(chan struct{})
	branches := out.Task("branches")
	branches.Define(func(ctx context.Context) error {
		return Effect(ctx, EffectSpec{Verb: Delete, Object: "stale local branch", Quantity: 2}, func(context.Context) error {
			close(started)
			<-release
			return nil
		})
	})
	<-started
	branches.Summary("2 deleted")
	close(release)
	_ = out.finish()
	snap := out.snapshot()
	found := false
	for _, ch := range snap.Changes {
		if len(ch.Records) > 0 {
			found = true
		}
	}
	if !found {
		t.Fatal("want the effect committed despite the concurrent Done, got no Changes records")
	}
}
