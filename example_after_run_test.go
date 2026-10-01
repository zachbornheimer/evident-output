package evo_test

import (
	"context"
	"fmt"
	"io"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

// ExampleAfterRun prints a closing line below everything the run rendered.
// Print after the run would be dropped; the hook is the supported place.
func ExampleAfterRun() {
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: os.Stdout})
	out.AfterRun(func(w io.Writer, r evo.Result) {
		_, _ = fmt.Fprintf(w, "exit code %d\n", r.ExitCode())
	})
	_ = out.Run(context.Background(), func(context.Context) error {
		out.Task("prune").Define(func(context.Context) error { return nil })
		return nil
	})
	// Output:
	// ✓ prune
	// exit code 0
}

// ExampleAfterRunFunc names the hook's shape: the human stream and the
// finished Result.
func ExampleAfterRunFunc() {
	var hook evo.AfterRunFunc = func(w io.Writer, r evo.Result) {
		_, _ = fmt.Fprintln(w, "done")
	}
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: os.Stdout})
	out.AfterRun(hook)
	_ = out.Run(context.Background(), func(context.Context) error { return nil })
	// Output:
	// done
}
