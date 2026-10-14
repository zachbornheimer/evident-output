package fixture

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// moveWorktree finishes the Effect before the marker File takes its own claim.
func moveWorktree(ctx context.Context, from, to string, marker []byte) error {
	spec := evo.EffectSpec{Verb: evo.Update, Object: "worktree", Quantity: 1, Path: from}
	err := evo.Effect(ctx, spec, func(context.Context) error {
		return os.Rename(from, to)
	})
	if err != nil {
		return err
	}
	return evo.File(ctx, evo.FileSpec{Path: to + "/.moved", Contents: marker})
}
