// Fixture: API-064 must stay silent for every ordered reader: an explicit
// After(computed), a later step of the producer's own Sequence, a transitive
// After, and a Get nested in a builder whose Group waits on the producer.
package api064

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

func run(out *evo.Output) {
	repo := out.Group("repository")
	branches := evo.Compute(repo.Task("prune landed branches"), pruneBranches)
	repo.Task("print branches").After(branches).Define(func(ctx context.Context) error {
		return show(branches.Get())
	})

	steps := out.Sequence("steps")
	managers := evo.Compute(steps.Task("detect package managers"), detect)
	steps.Task("discover installed packages").Define(func(ctx context.Context) error {
		return discover(managers.Get())
	})

	mid := steps.Task("middle").After(branches)
	steps.Task("late").After(mid).Define(func(ctx context.Context) error {
		return show(branches.Get())
	})

	centralize := steps.Group("centralize").After(managers)
	centralize.Define(func(g *evo.GroupHandle) {
		for _, m := range managers.Get() {
			g.Task("centralize " + m).Define(func(ctx context.Context) error { return show(managers.Get()) })
		}
	})
}
