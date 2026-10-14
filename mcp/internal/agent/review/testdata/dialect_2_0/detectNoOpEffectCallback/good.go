package fixture

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// installModules performs the install inside the Effect callback.
func installModules(ctx context.Context, count int, install func(context.Context) error) error {
	spec := evo.EffectSpec{Verb: evo.Create, Object: "module", Quantity: count}
	return evo.Effect(ctx, spec, install)
}
