package fixture

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// defineAndWait declares the work and then blocks the declaring stack until
// the callback has sent its result.
func defineAndWait(task *evo.Task, fn func(context.Context) error) error {
	done := make(chan error, 1)
	task.Define(func(ctx context.Context) error {
		err := fn(ctx)
		done <- err
		return err
	})
	return <-done
}
