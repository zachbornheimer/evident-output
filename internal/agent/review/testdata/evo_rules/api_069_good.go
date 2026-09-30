// Fixture: API-069 must stay silent. The builder reads a Compute result;
// the I/O lives in a Task callback nested inside the builder.
package api069

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func run(out *evo.Output) {
	packages := out.Sequence("consolidate packages")
	names := evo.Compute(packages.Task("list packages"), listPackages)
	packages.Group("centralize packages").Define(func(g *evo.GroupHandle) {
		for _, name := range names.Get() {
			g.Task("centralize " + name).Define(func(ctx context.Context) error {
				_, err := os.ReadDir(name)
				return err
			})
		}
	})
}
