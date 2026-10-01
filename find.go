package evo

import (
	"context"

	"github.com/zachbornheimer/evident-output/internal/engine"
)

// Find returns every regular file under root whose base name is exactly one
// of names, sorted by path, each with an absolute Path and nil Content. It
// searches in parallel, honours ctx cancellation, coalesces identical
// concurrent searches, does not follow symlinked directories, and takes no
// tree-wide lock.
func Find(ctx context.Context, root string, names ...string) ([]File, error) {
	return nil, errNotImplemented
}

// ErrFindNamesMissing is a Find called with no names.
var ErrFindNamesMissing = engine.ErrFindNamesMissing
