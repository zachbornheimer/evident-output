// Fixture: EVO-DAG-006 must stay silent. Same-path File tasks rely on
// process-local holds, not After.
package dag006

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

func writeBoth(out *evo.Output, path string, left, right []byte) {
	writeA := out.Task("write left config")
	writeA.Define(func(ctx context.Context) error {
		return evo.File(ctx, evo.FileSpec{Path: path, Contents: left})
	})
	writeB := out.Task("write right config")
	writeB.Define(func(ctx context.Context) error {
		return evo.File(ctx, evo.FileSpec{Path: path, Contents: right})
	})
}
