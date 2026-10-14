// A Task's Define callback declares child Tasks.
package api068

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func run(r *evo.Run, packages []string) {
	group := r.Group("centralize packages")
	group.Task("centralize packages").Define(func(ctx context.Context) error {
		for _, pkg := range packages {
			group.Task("centralize " + pkg).Define(func(ctx context.Context) error { return nil })
		}
		return nil
	})
}
