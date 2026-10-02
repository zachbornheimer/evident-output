package engine

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/zachbornheimer/evident-output/internal/find"
)

var findSearches find.Coalescer

// FindFiles is evo.Find: absolute paths of the regular files under root whose
// base name is one of names, sorted. Identical concurrent searches share one
// traversal.
func FindFiles(ctx context.Context, root string, names []string) ([]string, error) {
	if len(names) == 0 {
		return nil, ErrFindNamesMissing
	}
	if root == "" {
		return nil, ErrPathMissing
	}
	out := checksumScope(ctx)
	display := filepath.Clean(out.checksumPath(root))
	source := checksumSource{fsys: out.fileFSOrDefault()}
	real, err := out.resolveTreeRoot(display)
	if err != nil {
		return nil, err
	}
	walker := find.Walker{Source: source}
	key := find.Key(out, display, names)
	paths, err := findSearches.Do(ctx, key, func(c context.Context) ([]string, error) {
		return walker.Search(c, real, display, names)
	})
	if errors.Is(err, find.ErrNotDirectory) {
		err = fmt.Errorf("%w: %w", ErrTreePathTypeMismatch, err)
	}
	return slices.Clone(paths), err
}
