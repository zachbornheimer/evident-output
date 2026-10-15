package fixture

import (
	"bytes"
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func run(ctx context.Context, r *evo.Run) error {
	r.Task("scan sources").Define(func(ctx context.Context) error { return nil })
	return nil
}

// capture binds the stdout buffer first, so the caller can read it back.
func capture(ctx context.Context) (int, int) {
	stdout := new(bytes.Buffer)
	res := evo.Execute(ctx, evo.Config{Facades: evo.Facades{Stdout: stdout}}, run)
	return res.ExitCode(), stdout.Len()
}
