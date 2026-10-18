package fixture

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func report(r *evo.Run, version, sha string) {
	task := r.Task("read toolchain")
	task.Define(func(ctx context.Context) error {
		task.Fact("go version", version)
		task.Fact("commit", sha)
		return nil
	})
}
