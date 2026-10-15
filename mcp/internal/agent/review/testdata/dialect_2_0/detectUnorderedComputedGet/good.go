// Every reader is ordered: an explicit After(computed), a later step of the
// producer's own Sequence, a transitive After, and a Get inside a Build
// whose Group waits on the producer.
package api064

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func run(r *evo.Run) {
	repo := r.Group("repository")
	branches := evo.Compute(repo.Task("prune landed branches"), pruneBranches)
	repo.Task("print branches").After(branches).Define(func(ctx context.Context) error {
		return show(branches.Get())
	})

	steps := r.Sequence("steps")
	managers := evo.Compute(steps.Task("detect package managers"), detect)
	steps.Task("discover installed packages").Define(func(ctx context.Context) error {
		return discover(managers.Get())
	})

	mid := steps.Task("middle").After(branches)
	steps.Task("late").After(mid).Define(func(ctx context.Context) error {
		return show(branches.Get())
	})

	centralize := steps.Group("centralize").After(managers)
	centralize.Build(func(g *evo.Group) {
		for _, m := range managers.Get() {
			g.Task("centralize " + m).Define(func(ctx context.Context) error { return discover(managers.Get()) })
		}
	})
}

func pruneBranches(ctx context.Context) ([]string, error) { return nil, nil }
func detect(ctx context.Context) ([]string, error)        { return nil, nil }
func show(branches []string) error                        { return nil }
func discover(managers []string) error                    { return nil }
