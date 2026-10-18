package fixture

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// Verify only observes; the removal lives in Define.
func registerAgent(r *evo.Run, staleDir string) {
	r.Task("clear stale agent").
		Verify(func(ctx context.Context) (bool, error) {
			_, err := os.Stat(staleDir)
			return os.IsNotExist(err), nil
		}).
		Define(func(ctx context.Context) error {
			_, err := evo.Exec(ctx, evo.ExecSpec{Argv: []string{"rm", "-rf", staleDir}})
			return err
		})
}
