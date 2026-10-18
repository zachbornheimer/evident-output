package fixture

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// pruneWorktree names the singular Object and lets Quantity drive the plural.
func pruneWorktree(ctx context.Context, remove func(context.Context) error) error {
	spec := evo.EffectSpec{Verb: evo.Delete, Object: "worktree", Quantity: 1}
	return evo.Effect(ctx, spec, remove)
}
