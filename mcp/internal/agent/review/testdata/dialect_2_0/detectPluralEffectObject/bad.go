package fixture

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// pruneWorktree names the Object in the plural; Evo pluralizes from Quantity.
func pruneWorktree(ctx context.Context, remove func(context.Context) error) error {
	spec := evo.EffectSpec{Verb: evo.Delete, Object: "worktrees", Quantity: 1}
	return evo.Effect(ctx, spec, remove)
}
