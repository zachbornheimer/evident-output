package fixture

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func writeConfig(r *evo.Run, path string, contents []byte) {
	task := r.Task("write config")
	task.Define(func(ctx context.Context) error {
		if err := evo.File(ctx, evo.FileSpec{Path: path, Contents: contents}); err != nil {
			return err
		}
		task.Summary("wrote config.json")
		return nil
	})
}
