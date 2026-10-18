package fixture

import (
	"context"
	"os"
	"os/exec"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// run hands the terminal's own streams to the child while the live frame is up.
func run(ctx context.Context, r *evo.Run) error {
	r.Task("set up zq").Define(func(ctx context.Context) error {
		cmd := exec.CommandContext(ctx, "zq", "setup")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	})
	return nil
}
