package fixture

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// Verify deletes a directory as a side effect of "checking".
func registerAgent(r *evo.Run, staleDir string, register func(context.Context) error) {
	r.Task("register agent").
		Verify(func(ctx context.Context) (bool, error) {
			os.RemoveAll(staleDir)
			return true, nil
		}).
		Define(register)
}
