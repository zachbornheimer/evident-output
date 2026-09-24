package gates_test

import (
	"context"

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
