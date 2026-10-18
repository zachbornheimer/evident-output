package fixture

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// updateConfig writes a file from inside an Effect instead of using File.
func updateConfig(ctx context.Context, path string, contents []byte) error {
	spec := evo.EffectSpec{Verb: evo.Update, Object: "config file", Quantity: 1}
	return evo.Effect(ctx, spec, func(context.Context) error {
		return os.WriteFile(path, contents, 0o644)
	})
}
