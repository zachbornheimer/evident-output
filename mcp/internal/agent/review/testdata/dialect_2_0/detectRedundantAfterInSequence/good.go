// The After crosses containers: the producer belongs to a Group, not to the
// consumer's Sequence.
package api065

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func run(r *evo.Run) {
	repo := r.Group("repository")
	branches := evo.Compute(repo.Task("prune landed branches"), pruneBranches)
	steps := r.Sequence("report")
	steps.Task("print branches").After(branches).Define(func(ctx context.Context) error {
		return show(branches.Get())
	})
}

func pruneBranches(ctx context.Context) ([]string, error) { return nil, nil }
func show(branches []string) error                        { return nil }
