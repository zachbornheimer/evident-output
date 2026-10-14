package fixture

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// run asks to delete a remote branch without marking the question destructive.
func run(ctx context.Context, r *evo.Run) error {
	if !r.Confirm("delete origin/production-hotfix?", evo.NonInteractive("--yes")) {
		return evo.Refuse("declined")
	}
	return nil
}
