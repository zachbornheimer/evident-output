// Fixture: EVO-DRYRUN-001 must stay silent. The mutation routes through
// evo.File, an Evo-native operation dry-run can intercept.
package dryrun001

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

func writeConfig(task *evo.TaskHandle, path string, data []byte) {
	task.Define(func(ctx context.Context) error {
		return evo.File(ctx, evo.FileSpec{Path: path, Contents: data})
	})
}
