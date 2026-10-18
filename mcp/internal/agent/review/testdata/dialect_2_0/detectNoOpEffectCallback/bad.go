package fixture

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// installModules records a mutation but hands Effect no work to perform.
func installModules(ctx context.Context, count int) error {
	spec := evo.EffectSpec{Verb: evo.Create, Object: "module", Quantity: count}
	return evo.Effect(ctx, spec, nil)
}
