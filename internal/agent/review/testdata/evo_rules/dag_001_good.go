// Fixture: EVO-DAG-001 must stay silent. Group already schedules
// independent Tasks concurrently; no goroutine is needed.
package dag001

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

type Package struct{ Name string }

func installAll(group *evo.GroupHandle, pkgs []Package) {
	for _, pkg := range pkgs {
		pkg := pkg
		task := group.Task(pkg.Name)
		task.Define(func(ctx context.Context) error {
			return install(ctx, pkg)
		})
	}
}

func install(ctx context.Context, pkg Package) error { return nil }
