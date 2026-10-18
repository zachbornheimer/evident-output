package fixture

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func check(r *evo.Run, verify func(context.Context) error) {
	r.Task("check policy").Define(func(ctx context.Context) error {
		if err := verify(ctx); err != nil {
			return evo.Fail("policy check failed",
				evo.Detail("manifest missing"),
				evo.Next(evo.Action{Label: "run zq init"}))
		}
		return nil
	})
}
