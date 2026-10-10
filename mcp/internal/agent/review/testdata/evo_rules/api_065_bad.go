// Fixture: API-065 must fire. After(managers) names an earlier step of the
// same Sequence.
package api065

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

func run(out *evo.Output) {
	steps := out.Sequence("consolidate packages")
	managers := evo.Compute(steps.Task("detect package managers"), detect)
	steps.Task("discover installed packages").After(managers).Define(func(ctx context.Context) error {
		return discover(managers.Get())
	})
}
