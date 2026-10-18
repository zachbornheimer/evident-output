// The callback prints while doing real work.
package api071

import (
	"context"
	"fmt"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func run(r *evo.Run) {
	r.Task("build index").Define(func(ctx context.Context) error {
		fmt.Fprintln(r.Text(), "indexing")
		return buildIndex(ctx)
	})
}

func buildIndex(ctx context.Context) error { return nil }
