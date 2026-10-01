package evo_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// plannedEstimate is the quantity a measured Effect declares before it runs.
var plannedEstimate = evo.EffectSpec{Verb: evo.EffectDelete, Object: "merged branch", Quantity: 5}

func runMeasured(t *testing.T, dryRun bool, define func(out *evo.Output)) (string, evo.Snapshot) {
	t.Helper()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Stderr: io.Discard, Color: evo.ColorNever, Plain: true, DryRun: dryRun})
	t.Cleanup(func() { _ = out.Close() })
	define(out)
	_ = out.Finish()
	return buf.String(), out.Snapshot()
}

func measuredTask(measured int) func(context.Context) error {
	return func(ctx context.Context) error {
		return evo.MeasuredEffect(ctx, plannedEstimate, func(context.Context) (int, error) {
			return measured, nil
		})
	}
}

func TestMeasuredEffect_LedgerRecordsTheFinalQuantityNotThePlan(t *testing.T) {
	t.Parallel()
	rendered, snap := runMeasured(t, false, func(out *evo.Output) { out.Task("local").Define(measuredTask(2)) })
	if !strings.Contains(rendered, "deleted 2 merged branches") {
		t.Fatalf("ledger must show the measured quantity:\n%s", rendered)
	}
	if got := changedQuantities(snap); len(got) != 1 || got[0] != 2 {
		t.Fatalf("changed quantities = %v, want [2]", got)
	}
}

func TestMeasuredEffect_ClosingSummaryAggregatesMeasuredQuantities(t *testing.T) {
	t.Parallel()
	rendered, _ := runMeasured(t, false, func(out *evo.Output) {
		out.Task("local").Define(measuredTask(2))
		out.Task("remote").Define(measuredTask(4))
		out.Task("after").Define(func(context.Context) error { return errors.New("later step failed") })
	})
	if !strings.Contains(rendered, "6 merged branches delete") {
		t.Fatalf("already-mutated summary must sum measured quantities:\n%s", rendered)
	}
}

func TestMeasuredEffect_ZeroRecordsNothing(t *testing.T) {
	t.Parallel()
	_, snap := runMeasured(t, false, func(out *evo.Output) { out.Task("local").Define(measuredTask(0)) })
	if got := changedQuantities(snap); len(got) != 0 {
		t.Fatalf("a measured 0 must record no Effect, got %v", got)
	}
}

func TestMeasuredEffect_DryRunShowsThePlannedQuantityAndSkipsTheCallback(t *testing.T) {
	t.Parallel()
	ran := false
	rendered, _ := runMeasured(t, true, func(out *evo.Output) {
		out.Task("local").Define(func(ctx context.Context) error {
			return evo.MeasuredEffect(ctx, plannedEstimate, func(context.Context) (int, error) {
				ran = true
				return 2, nil
			})
		})
	})
	if ran || !strings.Contains(rendered, "delete 5 merged branches") {
		t.Fatalf("dry-run must show the planned quantity without running (ran=%v):\n%s", ran, rendered)
	}
}

func TestMeasuredEffect_NegativeQuantityFailsAndRecordsNothing(t *testing.T) {
	t.Parallel()
	var got error
	_, snap := runMeasured(t, false, func(out *evo.Output) {
		out.Task("local").Define(func(ctx context.Context) error {
			got = evo.MeasuredEffect(ctx, plannedEstimate, func(context.Context) (int, error) { return -1, nil })
			return got
		})
	})
	if !errors.Is(got, evo.ErrEffectMeasuredNegative) || len(changedQuantities(snap)) != 0 {
		t.Fatalf("err = %v, changed = %v, want ErrEffectMeasuredNegative and none", got, changedQuantities(snap))
	}
}

func TestMeasuredEffect_PartialEffectWinsOnFailure(t *testing.T) {
	t.Parallel()
	cause := errors.New("remote rejected")
	_, snap := runMeasured(t, false, func(out *evo.Output) {
		out.Task("local").Define(func(ctx context.Context) error {
			return evo.MeasuredEffect(ctx, plannedEstimate, func(context.Context) (int, error) {
				return 4, evo.PartialEffect(1, cause)
			})
		})
	})
	if got := changedQuantities(snap); len(got) != 1 || got[0] != 1 {
		t.Fatalf("changed quantities = %v, want [1]", got)
	}
}

func TestMeasuredEffect_NilCallbackIsRejected(t *testing.T) {
	t.Parallel()
	var got error
	runMeasured(t, false, func(out *evo.Output) {
		out.Task("local").Define(func(ctx context.Context) error {
			got = evo.MeasuredEffect(ctx, plannedEstimate, nil)
			return nil
		})
	})
	if !errors.Is(got, evo.ErrEffectCallbackMissing) {
		t.Fatalf("err = %v, want ErrEffectCallbackMissing", got)
	}
}
