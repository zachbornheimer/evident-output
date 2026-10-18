package fixture

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func install(r *evo.Run, installed, latest string, apply func(context.Context) error) {
	r.Task("install tool").
		Verify(func(ctx context.Context) (bool, error) { return installed == latest, nil }).
		Define(apply)
}

func skipProtected(r *evo.Run, branch string) {
	r.Task("prune " + branch).Define(func(ctx context.Context) error {
		return evo.Exclude("protected")
	})
}
