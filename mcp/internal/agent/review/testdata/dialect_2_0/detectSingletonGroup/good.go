// A lone Task, a Group filled by a loop, a Group that gets a Build callback
// or a nested container, and a Group handed back to its caller.
package api039

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func run(r *evo.Run, names []string) *evo.Group {
	r.Task("install:fresh-start").Define(install)

	jobs := r.Group("jobs")
	for _, name := range names {
		jobs.Task(name).Define(install)
	}

	built := r.Group("built")
	built.Task("first").Define(install)
	built.Build(func(g *evo.Group) {
		for _, name := range names {
			g.Task(name).Define(install)
		}
	})

	nested := r.Group("nested")
	nested.Task("first").Define(install)
	nested.Sequence("then")

	returned := r.Group("returned")
	returned.Task("first").Define(install)
	return returned
}

func install(ctx context.Context) error { return nil }
