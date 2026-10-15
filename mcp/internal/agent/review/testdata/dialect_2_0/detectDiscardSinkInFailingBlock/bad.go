package fixture

import (
	"context"
	"io"
	"os/exec"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func gate(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "git", "diff", "--check")
	cmd.Stdout = io.Discard
	if err := cmd.Run(); err != nil {
		return evo.Refuse("policy check failed")
	}
	return nil
}
