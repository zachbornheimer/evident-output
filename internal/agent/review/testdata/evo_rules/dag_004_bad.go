// Fixture: EVO-DAG-004 must fire. A custom writeLocks table and a
// synthetic //fix-schedule/ path around Evo.
package dag004

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

func writeConfig(out *evo.Output, path string, data []byte) {
	writeLocks := map[string]struct{}{path: {}}
	_ = writeLocks
	schedule := "//fix-schedule/" + path
	_ = schedule
	t := out.Task("write config")
	t.Define(func(ctx context.Context) error {
		return evo.File(ctx, evo.FileSpec{Path: path, Contents: data})
	})
}
