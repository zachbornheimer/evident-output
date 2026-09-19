// Fixture: EVO-DAG-005 must fire. A caller-owned goroutine Waits on a
// Task solely to sequence Evo work.
package dag005

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

func apply(out *evo.Output, path string, data []byte) {
	write := out.Task("write config")
	write.Define(func(ctx context.Context) error {
		return evo.File(ctx, evo.FileSpec{Path: path, Contents: data})
	})
	go func() {
		_ = write.Wait()
	}()
	read := out.Task("read config")
	read.Define(func(ctx context.Context) error { return nil })
}
