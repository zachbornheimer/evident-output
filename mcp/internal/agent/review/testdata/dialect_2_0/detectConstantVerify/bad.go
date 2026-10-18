package fixture

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func setup(r *evo.Run, apply func(context.Context) error) {
	r.Task("install hooks").
		Verify(func(context.Context) (bool, error) { return true, nil }).
		Define(apply)
	notYet := func(context.Context) (bool, error) { return false, nil }
	r.Task("register agent").Verify(notYet).Define(apply)
}
