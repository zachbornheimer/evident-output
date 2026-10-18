// A Task per item under a Group, and one long Task that narrates its loop
// with Progress.
package api070

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func run(r *evo.Run, branches []string) {
	group := r.Group("prune branches")
	for _, b := range branches {
		group.Task("delete " + b).Define(func(ctx context.Context) error { return deleteBranch(ctx, b) })
	}
	task := r.Task("copy archive")
	task.Define(func(ctx context.Context) error {
		for i, b := range branches {
			task.Progress(i, len(branches))
			if err := deleteBranch(ctx, b); err != nil {
				return err
			}
		}
		return nil
	})
}

func deleteBranch(ctx context.Context, branch string) error { return nil }
