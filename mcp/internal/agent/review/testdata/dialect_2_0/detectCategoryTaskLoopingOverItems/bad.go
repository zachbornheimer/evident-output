// One category Task loops over items that each deserve an outcome.
package api070

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func run(r *evo.Run, branches []string) {
	r.Task("prune branches").Define(func(ctx context.Context) error {
		for _, b := range branches {
			if err := deleteBranch(ctx, b); err != nil {
				return err
			}
		}
		return nil
	})
}

func deleteBranch(ctx context.Context, branch string) error { return nil }
