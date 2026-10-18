package fixture

import (
	"context"
	"fmt"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func checkBranches(r *evo.Run, check func(context.Context) (int, error)) {
	task := r.Task("check branches")
	task.Define(func(ctx context.Context) error {
		n, err := check(ctx)
		if err != nil {
			return err
		}
		task.Summary(fmt.Sprintf("%d checked", n))
		return nil
	})
}
