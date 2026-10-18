package fixture

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// generate passes template.txt to the generator but leaves it out of Inputs.
func generate(ctx context.Context) error {
	_, err := evo.Exec(ctx, evo.ExecSpec{
		Argv:    []string{"python3", "generate.py", "input.xlsx", "template.txt", "build/out.bin"},
		Inputs:  []string{"generate.py", "input.xlsx"},
		Outputs: []string{"build/out.bin"},
	})
	return err
}
