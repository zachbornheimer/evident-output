// Fixture: API-064 must fire. The report step reads branches.Get() but
// nothing orders it after the producer (different container, no After).
package api064

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

func run(out *evo.Output) {
	repo := out.Group("repository")
	branches := evo.Compute(repo.Task("prune landed branches"), pruneBranches)
	report := out.Sequence("report")
	report.Task("print branches").Define(func(ctx context.Context) error {
		return show(branches.Get())
	})
}
