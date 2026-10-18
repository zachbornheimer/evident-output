package fixture

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// moveWorktree writes a marker while its Effect still holds the source path.
func moveWorktree(ctx context.Context, from, to string, marker []byte) error {
	spec := evo.EffectSpec{Verb: evo.Update, Object: "worktree", Quantity: 1, Path: from}
	return evo.Effect(ctx, spec, func(ctx context.Context) error {
		if err := os.Rename(from, to); err != nil {
			return err
		}
		return evo.File(ctx, evo.FileSpec{Path: to + "/.moved", Contents: marker})
	})
}
