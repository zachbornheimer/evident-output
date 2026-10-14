package fixture

import (
	"context"
	"fmt"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// run prints its own green color and [OK] tag instead of resolving the Task.
func run(ctx context.Context, r *evo.Run) error {
	r.Task("build packages").Define(func(ctx context.Context) error {
		fmt.Fprintln(r.Text(), "\x1b[32m[OK]\x1b[0m built 12 packages")
		return nil
	})
	return nil
}
