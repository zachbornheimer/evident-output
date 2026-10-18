package fixture

import (
	"context"
	"os/exec"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// declarePatch shells out to git, so Evo cannot plan or guard the change.
func declarePatch(r *evo.Run, mboxPath string) {
	r.Task("apply patch").Define(func(ctx context.Context) error {
		return exec.CommandContext(ctx, "git", "am", mboxPath).Run()
	})
}
