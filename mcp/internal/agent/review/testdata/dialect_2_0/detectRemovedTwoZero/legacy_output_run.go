// legacy_output_run: the v1.0.0 hosted shape, an *Output from Init and Output.Run.
package main

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	out := evo.Init(evo.Config{Title: "hosted"})
	res := out.Run(context.Background(), func(ctx context.Context) error {
		out.Task("work").Define(func(context.Context) error { return nil })
		return nil
	})
	os.Exit(res.Conclusion.ExitCode)
}
