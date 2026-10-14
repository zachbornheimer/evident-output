package fixture

import (
	"context"
	"fmt"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func report(r *evo.Run, version, sha string) {
	r.Task("read toolchain").Define(func(ctx context.Context) error {
		fmt.Fprintf(r.Text(), "go version: %s\n", version)
		fmt.Fprintf(r.Text(), "commit: %s\n", sha)
		return nil
	})
}
