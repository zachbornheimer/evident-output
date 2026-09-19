// Fixture: EVO-DAG-004 must stay silent. File inside Define; no lock table.
package dag004

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

func writeConfig(out *evo.Output, path string, data []byte) {
	t := out.Task("write config")
	t.Define(func(ctx context.Context) error {
		return evo.File(ctx, evo.FileSpec{Path: path, Contents: data})
	})
}
