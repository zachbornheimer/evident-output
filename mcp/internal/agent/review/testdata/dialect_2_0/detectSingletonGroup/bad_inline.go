// The Group is declared and given its only child in one expression.
package api039

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func run(r *evo.Run) {
	r.Group("run").Task("install:fresh-start").Define(install)
}

func install(ctx context.Context) error { return nil }
