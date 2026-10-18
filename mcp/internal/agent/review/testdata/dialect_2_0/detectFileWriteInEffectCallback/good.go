package fixture

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// updateConfig declares the desired file state; File owns the write.
func updateConfig(ctx context.Context, path string, contents []byte) error {
	return evo.File(ctx, evo.FileSpec{Path: path, Contents: contents, Mode: 0o644})
}
