// inventory is assigned in one Define callback and read in another.
package api066

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

type Inventory struct{ Packages []string }

func run(r *evo.Run) {
	var inventory Inventory
	discover := r.Task("discover installed packages")
	discover.Define(func(ctx context.Context) error {
		var err error
		inventory, err = discoverPackages(ctx)
		return err
	})
	r.Task("centralize packages").After(discover).Define(func(ctx context.Context) error {
		return centralize(ctx, inventory)
	})
}

func discoverPackages(ctx context.Context) (Inventory, error) { return Inventory{}, nil }
func centralize(ctx context.Context, inv Inventory) error     { return nil }
