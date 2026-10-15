package fixture

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// declare orders two identical writes only to avoid a race File already prevents.
func declare(r *evo.Run, contents []byte) {
	config := r.Task("write config")
	config.Define(func(ctx context.Context) error {
		return evo.File(ctx, evo.FileSpec{Path: "config.json", Contents: contents})
	})
	cache := r.Task("warm cache")
	cache.Define(func(ctx context.Context) error {
		return evo.File(ctx, evo.FileSpec{Path: "config.json", Contents: contents})
	})
	// same file, avoid concurrent write race
	cache.After(config)
}
