package fixture

import (
	"context"
	"os"
	"os/exec"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// run lets an interactive child own the terminal while the live frame is paused.
func run(ctx context.Context, r *evo.Run) error {
	return r.Suspend(func() error {
		cmd := exec.CommandContext(ctx, "zq", "login")
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	})
}
