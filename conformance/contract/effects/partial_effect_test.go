// Package effects_test binds contract §30 "PartialEffect" rules that no
// existing test proves to the public evo API.
package effects_test

import (
	"context"
	"errors"
	"io"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

const (
	plannedQuantity   = 3
	committedQuantity = 2
)

var (
	errRejected = errors.New("remote rejected")
	refDelete   = evo.EffectSpec{Verb: evo.EffectDelete, Object: "remote ref", Quantity: plannedQuantity}
)

func newOutput(t *testing.T, dryRun bool) *evo.Output {
	t.Helper()
	out := evo.Init(evo.Config{
		Isolated: true, Plain: true, Color: evo.ColorNever, DryRun: dryRun,
		Stdout: io.Discard, Stderr: io.Discard,
	})
	t.Cleanup(func() { _ = out.Close() })
	return out
}

func recordedQuantities(records [][]evo.EffectRecord) []int64 {
	var quantities []int64
	for _, group := range records {
		for _, record := range group {
			quantities = append(quantities, record.Quantity)
		}
	}
	return quantities
}

func changedQuantities(snap evo.Snapshot) []int64 {
	var groups [][]evo.EffectRecord
	for _, changes := range snap.Changes {
		groups = append(groups, changes.Records)
	}
	return recordedQuantities(groups)
}

func plannedQuantities(snap evo.Snapshot) []int64 {
	var groups [][]evo.EffectRecord
	for _, plan := range snap.Plans {
		groups = append(groups, plan.Records)
	}
	return recordedQuantities(groups)
}

func TestC30_055_PartialEffectDryRunPlansTheFullQuantity(t *testing.T) {
	out := newOutput(t, true)
	invoked := false
	task := out.Task("refs")
	task.Define(func(ctx context.Context) error {
		return evo.Effect(ctx, refDelete, func(context.Context) error {
			invoked = true
			return evo.PartialEffect(committedQuantity, errRejected)
		})
	})
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if invoked {
		t.Fatal("dry run invoked the Effect callback")
	}
	snap := out.Snapshot()
	if got := plannedQuantities(snap); len(got) != 1 || got[0] != plannedQuantity {
		t.Fatalf("planned quantities = %v, want [%d]", got, plannedQuantity)
	}
	if got := changedQuantities(snap); len(got) != 0 {
		t.Fatalf("dry run recorded changed quantities %v", got)
	}
}

func TestC30_056_PartialEffectImpliesNoRollbackAndNoRetry(t *testing.T) {
	out := newOutput(t, false)
	calls := 0
	task := out.Task("refs")
	task.Define(func(ctx context.Context) error {
		return evo.Effect(ctx, refDelete, func(context.Context) error {
			calls++
			return evo.PartialEffect(committedQuantity, errRejected)
		})
	})
	_ = out.Finish()
	if calls != 1 {
		t.Fatalf("Effect callback ran %d times, want exactly once (no retry)", calls)
	}
	if got := changedQuantities(out.Snapshot()); len(got) != 1 || got[0] != committedQuantity {
		t.Fatalf("changed quantities = %v, want [%d] kept (no rollback)", got, committedQuantity)
	}
	if got := task.Snapshot().State; got != evo.Failed {
		t.Fatalf("task state = %s, want Failed", got)
	}
}
