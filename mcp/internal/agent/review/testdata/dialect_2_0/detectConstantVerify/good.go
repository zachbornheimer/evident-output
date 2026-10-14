package fixture

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func setup(r *evo.Run, hooks string, apply func(context.Context) error) {
	r.Task("install hooks").
		Verify(func(context.Context) (bool, error) {
			_, err := os.Stat(hooks)
			return err == nil, nil
		}).
		Define(apply)
}
