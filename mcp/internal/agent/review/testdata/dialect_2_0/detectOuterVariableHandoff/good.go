// The value flows through evo.Compute; an outer variable that only one
// callback touches is not a handoff.
package api066

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

type Inventory struct{ Packages []string }

func run(r *evo.Run) {
	inventory := evo.Compute(r.Task("discover installed packages"), discoverPackages)
	r.Task("centralize packages").After(inventory).Define(func(ctx context.Context) error {
		return centralize(ctx, inventory.Get())
	})

	var scratch int
	r.Task("count").Define(func(ctx context.Context) error {
		scratch = 1
		return nil
	})
	_ = scratch
}

func discoverPackages(ctx context.Context) (Inventory, error) { return Inventory{}, nil }
func centralize(ctx context.Context, inv Inventory) error     { return nil }
