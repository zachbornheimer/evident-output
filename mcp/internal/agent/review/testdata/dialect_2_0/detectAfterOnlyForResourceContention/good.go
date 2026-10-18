package fixture

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// declare leaves the identical writes unordered; File serializes the overlap.
func declare(r *evo.Run, contents []byte) {
	config := r.Task("write config")
	config.Define(func(ctx context.Context) error {
		return evo.File(ctx, evo.FileSpec{Path: "config.json", Contents: contents})
	})
	cache := r.Task("warm cache")
	cache.Define(func(ctx context.Context) error {
		return evo.File(ctx, evo.FileSpec{Path: "config.json", Contents: contents})
	})
}
