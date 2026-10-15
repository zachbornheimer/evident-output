package evo

import (
	"context"
	"fmt"

	"github.com/zachbornheimer/evident-output/v2/internal/change"
)

// succeed resolves task through the 1.1 success path — an empty Define,
// since ZYS-812 removed the Task.Done stamp — with summary as its
// optional result metadata (Task.Summary). It waits, so the row is
// terminal when succeed returns, as the removed Done was.
func succeed(task *Task, summary ...string) {
	if len(summary) > 0 {
		task.Summary(summary[0])
	}
	_ = task.Define(func(context.Context) error { return nil }).wait()
}

// commit resolves task through one Define whose work is specs, each an
// Effect around a no-op mutation — the 1.1 form of the removed
// record-only Task.Record (ZYS-974): the ledger row comes from the
// Effect itself. It waits, so the rows exist when commit returns. A spec
// is a public EffectSpec or, for a verb only the engine conjugates, a
// change.EffectSpec.
func commit[S EffectSpec | change.EffectSpec](task *Task, specs ...S) {
	_ = task.Define(func(ctx context.Context) error {
		for _, spec := range specs {
			if err := noopEffect(ctx, spec); err != nil {
				return err
			}
		}
		return nil
	}).wait()
}

// noopEffect performs spec around a mutation that does nothing.
func noopEffect(ctx context.Context, spec any) error {
	noop := func(context.Context) error { return nil }
	switch s := spec.(type) {
	case EffectSpec:
		return Effect(ctx, s, noop)
	case change.EffectSpec:
		return change.Effect(ctx, s, noop)
	}
	return nil
}

// distinctBranchDeletes is n one-branch Effects, each naming its own
// branch, so a ledger holds n distinct rows (identical rows merge).
func distinctBranchDeletes(n int) []EffectSpec {
	specs := make([]EffectSpec, n)
	for i := range specs {
		specs[i] = EffectSpec{Verb: Delete, Object: fmt.Sprintf("feat/branch-%d", i), Quantity: 1}
	}
	return specs
}

// satisfied resolves task AlreadySatisfied — a Verify that already holds,
// so its Define callback never runs: the 1.1 zero-information no-op row
// (a Define'd Task that returns nil executed work and is not a no-op).
func satisfied(task *Task) {
	task.Verify(func(context.Context) (bool, error) { return true, nil })
	_ = task.Define(func(context.Context) error { return nil }).wait()
}
