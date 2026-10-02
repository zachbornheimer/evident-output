// Fixture: API-066 must stay silent. The value flows through evo.Compute,
// and an outer variable that only one callback touches is not a handoff.
package api066

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

func run(out *evo.Output) {
	inventory := evo.Compute(out.Task("discover installed packages"), discoverPackages)
	out.Task("centralize packages").After(inventory).Define(func(ctx context.Context) error {
		return centralize(ctx, inventory.Get())
	})

	var scratch int
	out.Task("count").Define(func(ctx context.Context) error {
		scratch = 1
		return nil
	})
	_ = scratch
}
