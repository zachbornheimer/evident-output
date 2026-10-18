// The Group's Build callback declares the children; Task callbacks only do work.
package api068

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func run(r *evo.Run, packages []string) {
	group := r.Group("centralize packages")
	group.Build(func(g *evo.Group) {
		for _, pkg := range packages {
			g.Task("centralize " + pkg).Define(func(ctx context.Context) error { return centralize(ctx, pkg) })
		}
	})
}

func centralize(ctx context.Context, pkg string) error { return nil }
