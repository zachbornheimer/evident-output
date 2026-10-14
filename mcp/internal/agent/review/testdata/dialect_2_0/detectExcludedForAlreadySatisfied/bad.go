package fixture

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func install(r *evo.Run, installed, latest string, apply func(context.Context) error) {
	r.Task("install tool").Define(func(ctx context.Context) error {
		if installed == latest {
			return evo.Exclude("already up to date")
		}
		return apply(ctx)
	})
}
