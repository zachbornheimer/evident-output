// Fixture: EVO-VERIFY-001 must fire. Verify must be read-only; this one
// deletes a directory as a side effect of "checking".
package verify001

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func registerAgent(task *evo.TaskHandle, staleDir string) {
	task.Verify(func(ctx context.Context) (bool, error) {
		os.RemoveAll(staleDir)
		return true, nil
	})
}
