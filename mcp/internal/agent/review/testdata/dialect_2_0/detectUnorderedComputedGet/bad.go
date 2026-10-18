// The report step reads branches.Get() with nothing ordering it after the
// producer: different container, no After.
package api064

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func run(r *evo.Run) {
	repo := r.Group("repository")
	branches := evo.Compute(repo.Task("prune landed branches"), pruneBranches)
	report := r.Sequence("report")
	report.Task("print branches").Define(func(ctx context.Context) error {
		return show(branches.Get())
	})
}

func pruneBranches(ctx context.Context) ([]string, error) { return nil, nil }
func show(branches []string) error                        { return nil }
