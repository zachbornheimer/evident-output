// Fixture: API-068 must stay silent. The Group's builder declares the
// children; the Task callbacks only do work.
package api068

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

func run(out *evo.Output, packages []string) {
	group := out.Group("centralize packages")
	group.Define(func(g *evo.GroupHandle) {
		for _, pkg := range packages {
			g.Task("centralize " + pkg).Define(func(ctx context.Context) error { return centralize(ctx, pkg) })
		}
	})
}
