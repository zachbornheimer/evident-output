package evo_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// effectRun runs one Task whose Define calls evo.Effect(spec, fn) and
// returns the Output (finished), the rendered text, and Effect's error.
func effectRun(t *testing.T, dryRun bool, spec evo.EffectSpec, fn func(context.Context) error) (*evo.Output, string, error) {
	t.Helper()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Title: "prune", Color: evo.ColorNever, Plain: true, DryRun: dryRun})
	t.Cleanup(func() { _ = out.Close() })
	var effectErr error
	out.Task("worktrees").Define(func(ctx context.Context) error {
		effectErr = evo.Effect(ctx, spec, fn)
		return effectErr
	})
	_ = out.Finish()
	return out, strings.Join(strings.Fields(buf.String()), " "), effectErr
}

var worktreeDelete = evo.EffectSpec{Verb: evo.EffectDelete, Object: "worktree", Quantity: 3}

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
	snap := out.Snapshot()
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
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Title: "prune", Color: evo.ColorNever, Plain: true})
	t.Cleanup(func() { _ = out.Close() })
	changesDuringFn := -1
	var effectErr error
	out.Task("worktrees").Define(func(ctx context.Context) error {
		effectErr = evo.Effect(ctx, worktreeDelete, func(context.Context) error {
			changesDuringFn = len(out.Snapshot().Changes)
			return nil
		})
		return effectErr
	})
	if err := out.Finish(); err != nil || effectErr != nil {
		t.Fatalf("Finish err = %v, Effect err = %v, want nil", err, effectErr)
	}
	if changesDuringFn != 0 {
		t.Fatalf("changes while fn ran = %d, want 0 (recorded before fn returned)", changesDuringFn)
	}
	snap := out.Snapshot()
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
	if snap := out.Snapshot(); len(snap.Changes) != 0 {
		t.Fatalf("changes = %d, want 0 after a failed callback", len(snap.Changes))
	}
}

func TestEffect_FnReceivesSchedulerOwnedContext(t *testing.T) {
	t.Parallel()
	out := evo.Init(evo.Config{Isolated: true, Title: "prune", Color: evo.ColorNever})
	t.Cleanup(func() { _ = out.Close() })
	var defineCtx, fnCtx context.Context
	out.Task("worktrees").Define(func(ctx context.Context) error {
		defineCtx = ctx
		return evo.Effect(ctx, worktreeDelete, func(ctx context.Context) error {
			fnCtx = ctx
			return nil
		})
	})
	if err := out.Finish(); err != nil {
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
		spec evo.EffectSpec
		fn   func(context.Context) error
		want error
	}{
		{"zero quantity", evo.EffectSpec{Verb: evo.EffectDelete, Object: "worktree"}, ok, evo.ErrEffectQuantityNotPositive},
		{"negative quantity", evo.EffectSpec{Verb: evo.EffectDelete, Object: "worktree", Quantity: -1}, ok, evo.ErrEffectQuantityNotPositive},
		{"missing verb", evo.EffectSpec{Object: "worktree", Quantity: 1}, ok, evo.ErrEffectVerbInvalid},
		{"write verb", evo.EffectSpec{Verb: "write", Object: "worktree", Quantity: 1}, ok, evo.ErrEffectVerbInvalid},
		{"missing object", evo.EffectSpec{Verb: evo.EffectDelete, Quantity: 1}, ok, evo.ErrEffectObjectMissing},
		{"blank object", evo.EffectSpec{Verb: evo.EffectDelete, Object: "  ", Quantity: 1}, ok, evo.ErrEffectObjectMissing},
		{"nil callback", worktreeDelete, nil, evo.ErrEffectCallbackMissing},
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
				if snap := out.Snapshot(); len(snap.Plans)+len(snap.Changes) != 0 {
					t.Fatalf("dryRun=%v: rejected Effect recorded plan=%d changes=%d", dryRun, len(snap.Plans), len(snap.Changes))
				}
			})
		}
	}
}

func TestEffect_OutsideDefineReturnsErrNoTaskContext(t *testing.T) {
	t.Parallel()
	invoked := false
	err := evo.Effect(context.Background(), worktreeDelete, func(context.Context) error {
		invoked = true
		return nil
	})
	if !errors.Is(err, evo.ErrNoTaskContext) || invoked {
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
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true, DryRun: true})
	const total = 500
	commit(out.Task("branches"), distinctBranchDeletes(total)...)
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	snap := out.Snapshot()
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
