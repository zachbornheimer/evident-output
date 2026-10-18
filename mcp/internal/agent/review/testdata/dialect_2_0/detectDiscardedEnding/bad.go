package fixture

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// resolve builds an ending and throws it away, so the Task ends with the
// bare error instead.
func resolve(r *evo.Run, lookup func(context.Context) error) {
	r.Task("resolve refs").Define(func(ctx context.Context) error {
		if err := lookup(ctx); err != nil {
			evo.Fail("cannot resolve refs", evo.Detail(err.Error()))
			return err
		}
		return nil
	})
}
