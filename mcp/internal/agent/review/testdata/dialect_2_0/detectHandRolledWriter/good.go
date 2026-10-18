package fixture

import (
	"context"
	"os/exec"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func build(ctx context.Context, t *evo.Task) error {
	cmd := exec.CommandContext(ctx, "make")
	cmd.Stdout = t.Writer()
	return cmd.Run()
}
