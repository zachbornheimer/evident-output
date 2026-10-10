// Fixture: API-071 must fire. The Task's whole job is to print a header.
package api071

import (
	"context"
	"fmt"

	evo "github.com/zachbornheimer/evident-output"
)

func run(out *evo.Output) {
	out.Task("Phase 2 header").Define(func(ctx context.Context) error {
		fmt.Println("== phase 2 ==")
		return nil
	})
}
