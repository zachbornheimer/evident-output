package fixture

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func run(ctx context.Context, r *evo.Run) error {
	if !r.Confirm("delete 40 branches?", evo.Destructive(), evo.NonInteractive("--yes")) {
		return evo.Refuse("declined")
	}
	r.Task("delete branches").Define(func(context.Context) error { return nil })
	return nil
}
