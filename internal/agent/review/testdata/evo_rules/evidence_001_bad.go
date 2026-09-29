// Fixture: EVO-EVIDENCE-001 must fire. A legacy named Evidence callback
// mutates state directly instead of moving the write into Define.
package evidence001

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func writePlist(task *evo.TaskHandle, path string, data []byte) {
	task.Evidence("write", func() error {
		return os.WriteFile(path, data, 0o644)
	})
	task.Define(func(ctx context.Context) error {
		return nil
	})
}
