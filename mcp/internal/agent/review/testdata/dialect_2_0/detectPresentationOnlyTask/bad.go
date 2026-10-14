// The Task's whole job is to print a header.
package api071

import (
	"context"
	"fmt"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func run(r *evo.Run) {
	r.Task("Phase 2 header").Define(func(ctx context.Context) error {
		fmt.Fprintln(r.Text(), "== phase 2 ==")
		return nil
	})
}
