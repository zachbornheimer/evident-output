package fixture

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// run lets Confirm own the prompt, the unattended flag and the decline.
func run(ctx context.Context, r *evo.Run) error {
	if !r.Confirm("delete origin/production-hotfix?", evo.Destructive(), evo.NonInteractive("--yes")) {
		return evo.Refuse("declined")
	}
	return nil
}
