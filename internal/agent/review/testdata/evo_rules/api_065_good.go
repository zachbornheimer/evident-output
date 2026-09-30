// Fixture: API-065 must stay silent. The After crosses containers: the
// producer belongs to a Group, not to the consumer's Sequence.
package api065

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

func run(out *evo.Output) {
	repo := out.Group("repository")
	branches := evo.Compute(repo.Task("prune landed branches"), pruneBranches)
	steps := out.Sequence("report")
	steps.Task("print branches").After(branches).Define(func(ctx context.Context) error {
		return show(branches.Get())
	})
}
