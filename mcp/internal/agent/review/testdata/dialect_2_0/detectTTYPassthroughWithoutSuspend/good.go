package fixture

import (
	"context"
	"os/exec"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// run captures the child's lines as the task's live tail.
func run(ctx context.Context, r *evo.Run) error {
	setup := r.Task("set up zq")
	setup.Define(func(ctx context.Context) error {
		cmd := exec.CommandContext(ctx, "zq", "setup")
		cmd.Stdout = setup.Writer()
		cmd.Stderr = setup.Writer()
		return cmd.Run()
	})
	return nil
}
