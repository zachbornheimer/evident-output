// Fixture: EVO-DAG-001 must fire. A goroutine wraps a call that already
// submits work to Evo's scheduler — Group already runs children
// concurrently without any goroutine of the caller's own.
package dag001

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

type Package struct{ Name string }

func installAll(group *evo.GroupHandle, pkgs []Package) {
	for _, pkg := range pkgs {
		pkg := pkg
		go func(pkg Package) {
			task := group.Task(pkg.Name)
			task.Define(func(ctx context.Context) error {
				return install(ctx, pkg)
			})
		}(pkg)
	}
}

func install(ctx context.Context, pkg Package) error { return nil }
