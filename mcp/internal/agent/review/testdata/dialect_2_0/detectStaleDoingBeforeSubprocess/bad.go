package fixture

import (
	"context"
	"os/exec"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// upload sets Doing once, then waits on a child that prints nothing back.
func upload(ctx context.Context, t *evo.Task, dest string) error {
	t.Doing("uploading to %s", dest)
	cmd := exec.CommandContext(ctx, "rsync", "-a", "build/", dest)
	return cmd.Run()
}
