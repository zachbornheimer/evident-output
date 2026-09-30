// Fixture: API-071 must not fire. The callback prints while doing real work.
package api071

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

func run(out *evo.Output) {
	out.Task("build index").Define(func(ctx context.Context) error {
		out.Println("indexing")
		return buildIndex(ctx)
	})
}
