// A Build callback that reads a Computed its Group never waits on.
package api064

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func run(r *evo.Run) {
	repo := r.Group("repository")
	branches := evo.Compute(repo.Task("prune landed branches"), pruneBranches)
	report := r.Group("report")
	report.Build(func(g *evo.Group) {
		for _, b := range branches.Get() {
			g.Task("print " + b).Define(func(ctx context.Context) error { return show(b) })
		}
	})
}

func pruneBranches(ctx context.Context) ([]string, error) { return nil, nil }
func show(branch string) error                            { return nil }
