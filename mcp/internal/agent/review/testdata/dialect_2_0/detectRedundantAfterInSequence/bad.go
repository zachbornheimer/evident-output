// After(managers) names an earlier step of the same Sequence.
package api065

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func run(r *evo.Run) {
	steps := r.Sequence("consolidate packages")
	managers := evo.Compute(steps.Task("detect package managers"), detect)
	steps.Task("discover installed packages").After(managers).Define(func(ctx context.Context) error {
		return discover(managers.Get())
	})
}

func detect(ctx context.Context) ([]string, error) { return nil, nil }
func discover(managers []string) error             { return nil }
