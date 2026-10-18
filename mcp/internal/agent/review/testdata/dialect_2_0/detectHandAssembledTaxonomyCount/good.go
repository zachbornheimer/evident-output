package fixture

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// declare gives each branch its own Task; evo counts the Excluded ones by reason.
func declare(r *evo.Run, protected []string) {
	branches := r.Group("branches")
	for _, name := range protected {
		branches.Task(name).Define(func(context.Context) error {
			return evo.Exclude("protected")
		})
	}
}
