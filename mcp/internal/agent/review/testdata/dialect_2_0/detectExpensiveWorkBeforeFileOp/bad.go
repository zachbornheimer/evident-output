package fixture

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// writeReport renders before File runs, so a current output cannot skip the render.
func writeReport(ctx context.Context, output string, render func(context.Context) ([]byte, error)) error {
	data, err := render(ctx)
	if err != nil {
		return err
	}
	return evo.File(ctx, evo.FileSpec{Path: output, Contents: data, Mode: 0o644})
}
