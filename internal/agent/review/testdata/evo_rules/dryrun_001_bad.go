// Fixture: EVO-DRYRUN-001 must fire. Define raw-calls os.WriteFile, which
// Evo's dry-run cannot intercept.
package dryrun001

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func writeConfig(task *evo.TaskHandle, path string, data []byte) {
	task.Define(func(ctx context.Context) error {
		return os.WriteFile(path, data, 0o644)
	})
}
