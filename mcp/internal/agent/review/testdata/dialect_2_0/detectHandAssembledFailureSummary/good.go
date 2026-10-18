package fixture

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

type failure struct{ name, message string }

// declare resolves each failure on its own Task; Conclusion summarizes them.
func declare(r *evo.Run, failures []failure) {
	for _, f := range failures {
		r.Task(f.name).Define(func(context.Context) error {
			return evo.Fail(f.message)
		})
	}
}
