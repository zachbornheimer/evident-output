package fixture

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// declareConfig routes the write through File so dry run plans it.
func declareConfig(r *evo.Run, path string, data []byte) {
	r.Task("write config").Define(func(ctx context.Context) error {
		return evo.File(ctx, evo.FileSpec{Path: path, Contents: data, Mode: 0o644})
	})
}
