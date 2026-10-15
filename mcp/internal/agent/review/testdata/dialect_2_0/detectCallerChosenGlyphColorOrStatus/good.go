package fixture

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// run resolves the Task and lets the renderer pick the glyph and the color.
func run(ctx context.Context, r *evo.Run) error {
	build := r.Task("build packages")
	build.Define(func(ctx context.Context) error {
		build.Summary("built 12 packages")
		return nil
	})
	return nil
}
