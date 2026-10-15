package fixture

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// declare orders the dependent work with After, so nothing blocks.
func declare(r *evo.Run, apply, reload func(context.Context) error) {
	first := r.Task("apply config")
	first.Define(apply)
	r.Task("reload service").After(first).Define(reload)
}
