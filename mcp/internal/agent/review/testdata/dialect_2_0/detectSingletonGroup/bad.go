// A Group named for the command with a single child is a lone Task.
package api039

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func run(r *evo.Run) {
	jobs := r.Group("run")
	jobs.Task("install:fresh-start").Define(install)
}

func install(ctx context.Context) error { return nil }
