// Fixture: API-070 must stay silent. A Task per item under a Group, and one
// long Task that narrates its loop with Progress.
package api070

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

func run(out *evo.Output, branches []string) {
	group := out.Group("prune branches")
	for _, b := range branches {
		group.Task("delete " + b).Define(func(ctx context.Context) error { return deleteBranch(ctx, b) })
	}
	task := out.Task("copy archive")
	task.Define(func(ctx context.Context) error {
		for i, b := range branches {
			task.Progress(i, len(branches))
			copyOne(ctx, b)
		}
		return nil
	})
}
