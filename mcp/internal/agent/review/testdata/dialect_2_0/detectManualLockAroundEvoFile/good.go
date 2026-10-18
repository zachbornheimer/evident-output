package fixture

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

type writer struct {
	path string
}

// write leaves serialization to File's own path claim.
func (w *writer) write(ctx context.Context, contents []byte) error {
	return evo.File(ctx, evo.FileSpec{Path: w.path, Contents: contents})
}
