package fixture

import (
	"context"
	"os/exec"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// upload wires the child's output to the Task, so its last line is the live text.
func upload(ctx context.Context, t *evo.Task, dest string) error {
	t.Doing("uploading to %s", dest)
	cmd := exec.CommandContext(ctx, "rsync", "-a", "build/", dest)
	cmd.Stdout = t.Writer()
	cmd.Stderr = t.Writer()
	return cmd.Run()
}
