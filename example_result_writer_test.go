package evo_test

import (
	"context"
	"fmt"
	"io"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// ExampleRun_ResultWriter writes the data payload to stdout while the
// presentation stays on stderr, so `tool | consumer` sees only the payload.
func ExampleRun_ResultWriter() {
	evo.Execute(context.Background(), evo.Config{
		Name:    "build",
		Output:  evo.Plain,
		Facades: evo.Facades{Stdout: os.Stdout, Stderr: io.Discard},
	}, func(_ context.Context, r *evo.Run) error {
		r.Task("compile").Define(func(context.Context) error { return nil })
		_, err := fmt.Fprintln(r.ResultWriter(), "bin/app")
		return err
	})
	// Output:
	// bin/app
}
