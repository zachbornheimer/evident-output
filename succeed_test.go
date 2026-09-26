package evo_test

import (
	"context"
	"fmt"

	evo "github.com/zachbornheimer/evident-output"
)

// succeed resolves task through the 1.1 success path — an empty Define,
// since ZYS-812 removed the TaskHandle.Done stamp — with summary as its
// optional result metadata (TaskHandle.Summary). It waits, so the row is
// terminal when succeed returns, as the removed Done was.
func succeed(task *evo.TaskHandle, summary ...string) {
	if len(summary) > 0 {
		task.Summary(summary[0])
	}
	_ = task.Define(func(context.Context) error { return nil }).Wait()
}

// commit resolves task through one Define whose work is specs, each an
// evo.Effect around a no-op mutation — the 1.1 form of the removed
// record-only TaskHandle.Record (ZYS-974): the ledger row comes from the
// Effect itself. It waits, so the rows exist when commit returns.
func commit(task *evo.TaskHandle, specs ...evo.EffectSpec) {
	_ = task.Define(func(ctx context.Context) error {
		for _, spec := range specs {
			if err := evo.Effect(ctx, spec, func(context.Context) error { return nil }); err != nil {
				return err
			}
		}
		return nil
	}).Wait()
}

// distinctBranchDeletes is n one-branch Effects, each naming its own
// branch, so a ledger holds n distinct rows (identical rows merge).
func distinctBranchDeletes(n int) []evo.EffectSpec {
	specs := make([]evo.EffectSpec, n)
	for i := range specs {
		specs[i] = evo.EffectSpec{Verb: evo.EffectDelete, Object: fmt.Sprintf("feat/branch-%d", i), Quantity: 1}
	}
	return specs
}

// skipItems declares n children of items, each resolving Skipped with
// reason, so items' own Task (same name as items — docs/reference.md
// "own Task") folds them into one "- skipped N (...)" tally row instead of
// n rows (contract §18/§25/§26).
func skipItems(items *evo.GroupHandle, prefix string, reason evo.TaxonomyReason, n int) {
	for i := range n {
		items.Task(fmt.Sprintf("%s-%d", prefix, i)).Skipped(reason)
	}
}

// satisfied resolves task AlreadySatisfied — a Verify that already holds,
// so its Define callback never runs: the 1.1 zero-information no-op row
// (a Define'd Task that returns nil executed work and is not a no-op).
func satisfied(task *evo.TaskHandle) {
	task.Verify(func(context.Context) (bool, error) { return true, nil })
	_ = task.Define(func(context.Context) error { return nil }).Wait()
}
