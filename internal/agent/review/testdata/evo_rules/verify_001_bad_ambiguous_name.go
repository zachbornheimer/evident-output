// Fixture: EVO-VERIFY-001 must still fire when the real *evo.TaskHandle
// parameter happens to be named "process" — a name isLikelyEvoReceiver's
// Start-only exclusion list would otherwise treat as definitely non-Evo.
package verify001ambiguous

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func registerAgent(process *evo.TaskHandle, staleDir string) {
	process.Verify(func(ctx context.Context) (bool, error) {
		os.RemoveAll(staleDir)
		return true, nil
	})
}
