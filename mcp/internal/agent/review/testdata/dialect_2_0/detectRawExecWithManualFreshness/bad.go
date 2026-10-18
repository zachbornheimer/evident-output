package fixture

import (
	"context"
	"os"
	"os/exec"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// declareGenerate hand-rolls the staleness check evo.Exec derives from Inputs and Outputs.
func declareGenerate(r *evo.Run, inputPath, outPath string) {
	r.Task("generate output").Define(func(ctx context.Context) error {
		if outputIsStale(inputPath, outPath) {
			return exec.CommandContext(ctx, "python3", "generate.py", inputPath, outPath).Run()
		}
		return nil
	})
}

func outputIsStale(input, output string) bool {
	in, inErr := os.Stat(input)
	out, outErr := os.Stat(output)
	return inErr != nil || outErr != nil || in.ModTime().After(out.ModTime())
}
