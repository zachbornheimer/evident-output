// Fixture: EVO-EVIDENCE-001 must stay silent. The write lives in Define,
// where evo.File tracks and reconciles it.
package evidence001

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

func writePlist(task *evo.TaskHandle, path string, data []byte) {
	task.Define(func(ctx context.Context) error {
		return evo.File(ctx, evo.FileSpec{Path: path, Contents: data})
	})
}
