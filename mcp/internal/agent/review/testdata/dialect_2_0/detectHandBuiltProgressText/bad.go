package fixture

import (
	"context"
	"fmt"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func copyAll(r *evo.Run, files []string, copyOne func(context.Context, string) error) {
	r.Task("copy files").Define(func(ctx context.Context) error {
		for i, f := range files {
			if err := copyOne(ctx, f); err != nil {
				return err
			}
			fmt.Fprintf(r.Text(), "%d/%d done\n", i+1, len(files))
		}
		return nil
	})
}
