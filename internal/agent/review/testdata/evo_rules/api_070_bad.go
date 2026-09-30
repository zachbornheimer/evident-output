// Fixture: API-070 must fire. One category Task loops over items that each
// deserve an outcome.
package api070

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

func run(out *evo.Output, branches []string) {
	out.Task("prune branches").Define(func(ctx context.Context) error {
		for _, b := range branches {
			if err := deleteBranch(ctx, b); err != nil {
				return err
			}
		}
		return nil
	})
}
