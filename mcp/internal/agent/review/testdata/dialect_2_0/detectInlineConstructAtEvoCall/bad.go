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

// capture builds the stdout buffer inline, so the caller cannot read it back.
func capture(ctx context.Context) int {
	res := evo.Execute(ctx, evo.Config{Facades: evo.Facades{Stdout: new(bytes.Buffer)}}, run)
	return res.ExitCode()
}
