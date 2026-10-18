// legacy_output_run_named: the v1.0.0 hosted shape with the run callback a
// package func, so Output.Run hands evo.Execute a name and not a literal.
package main

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	out := evo.Init(evo.Config{Title: "hosted"})
	res := out.Run(context.Background(), run)
	os.Exit(res.Conclusion.ExitCode)
}

func run(ctx context.Context) error {
	evo.Task("work").Define(func(context.Context) error { return nil })
	return nil
}
