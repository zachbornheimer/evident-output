// Fixture: EVO-DAG-005 must stay silent. Sequence owns the order.
package dag005

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

func apply(out *evo.Output, path string, data []byte) {
	seq := evo.Sequence("apply greeting")
	write := seq.Task("write config")
	write.Define(func(ctx context.Context) error {
		return evo.File(ctx, evo.FileSpec{Path: path, Contents: data})
	})
	read := seq.Task("read config")
	read.Define(func(ctx context.Context) error { return nil })
}
