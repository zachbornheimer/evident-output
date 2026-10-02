// Fixture: API-066 must fire. inventory is assigned in one Define callback
// and read in another.
package api066

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

func run(out *evo.Output) {
	var inventory Inventory
	discover := out.Task("discover installed packages")
	discover.Define(func(ctx context.Context) error {
		var err error
		inventory, err = discoverPackages(ctx)
		return err
	})
	out.Task("centralize packages").After(discover).Define(func(ctx context.Context) error {
		return centralize(ctx, inventory)
	})
}
