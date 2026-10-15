package fixture

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// declareGenerate declares Inputs and Outputs; Evo skips the run when they are current.
func declareGenerate(r *evo.Run, inputPath, outPath string) {
	r.Task("generate output").Define(func(ctx context.Context) error {
		_, err := evo.Exec(ctx, evo.ExecSpec{
			Argv:    []string{"python3", "generate.py", inputPath, outPath},
			Inputs:  []string{"generate.py", inputPath},
			Outputs: []string{outPath},
		})
		return err
	})
}
