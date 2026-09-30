// Fixture: API-068 must fire. A Task's Define declares child Tasks.
package api068

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

func run(out *evo.Output, packages []string) {
	group := out.Group("centralize packages")
	group.Task("centralize packages").Define(func(ctx context.Context) error {
		for _, pkg := range packages {
			group.Task("centralize " + pkg).Define(func(ctx context.Context) error { return nil })
		}
		return nil
	})
}
