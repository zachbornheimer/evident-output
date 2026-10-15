// The builder reads a Computed result; the I/O lives in a Task callback
// nested inside it.
package api069

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func run(r *evo.Run) {
	packages := r.Sequence("consolidate packages")
	names := evo.Compute(packages.Task("list packages"), listPackages)
	packages.Group("centralize packages").Build(func(g *evo.Group) {
		for _, name := range names.Get() {
			g.Task("centralize " + name).Define(func(ctx context.Context) error {
				_, err := os.ReadDir(name)
				return err
			})
		}
	})
}

func listPackages(ctx context.Context) ([]string, error) { return nil, nil }
