package fixture

import (
	"context"
	"sync"

	evo "github.com/zachbornheimer/evident-output/v2"
)

type writer struct {
	mu   sync.Mutex
	path string
}

// write holds a caller lock around File, which already claims its own path.
func (w *writer) write(ctx context.Context, contents []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return evo.File(ctx, evo.FileSpec{Path: w.path, Contents: contents})
}
