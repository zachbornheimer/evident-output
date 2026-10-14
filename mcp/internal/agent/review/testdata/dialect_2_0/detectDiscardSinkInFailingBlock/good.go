package fixture

import (
	"context"
	"os/exec"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func gate(ctx context.Context, t *evo.Task) error {
	cmd := exec.CommandContext(ctx, "git", "diff", "--check")
	cmd.Stdout = t.Writer()
	cmd.Stderr = t.Writer()
	if err := cmd.Run(); err != nil {
		return evo.Refuse("policy check failed", evo.Detail(err.Error()))
	}
	return nil
}
