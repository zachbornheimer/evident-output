package fixture

import (
	"context"
	"fmt"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func check(r *evo.Run, verify func(context.Context) error) {
	r.Task("verify manifest").Define(func(ctx context.Context) error {
		if err := verify(ctx); err != nil {
			return err
		}
		fmt.Fprintln(r.Text(), "✓ verified")
		return nil
	})
}
