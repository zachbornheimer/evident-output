package fixture

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// declarePatch hands the diff to Evo, which commits it through File.
func declarePatch(r *evo.Run, diff []byte) {
	r.Task("apply patch").Define(func(ctx context.Context) error {
		return evo.Patch(ctx, diff)
	})
}
