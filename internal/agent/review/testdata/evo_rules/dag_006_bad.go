// Fixture: EVO-DAG-006 must fire. After serializes two File calls on the
// same path — resource contention, not a DAG edge.
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
	writeB.After(writeA)
}
