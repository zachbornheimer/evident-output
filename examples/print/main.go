// Command print is the first adoption rung: free human text through
// Run.Text, an io.Writer, so fmt.Fprint, Fprintf and Fprintln work unchanged.
//
//	go run ./examples/print/
package main

import (
	"context"
	"fmt"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func main() {
	os.Exit(evo.Main(evo.Config{Name: "packages"}, func(ctx context.Context, r *evo.Run) error {
		// Run.Text is routed with the presentation: stderr in Human and Plain,
		// and never stdout, which belongs to Run.ResultWriter.
		_, _ = fmt.Fprintln(r.Text(), "Reading configuration")
		_, _ = fmt.Fprintf(r.Text(), "Found %d packages\n", 18)
		_, _ = fmt.Fprint(r.Text(), "Resolving")
		_, _ = fmt.Fprint(r.Text(), "…")
		_, _ = fmt.Fprint(r.Text(), "\n")
		_, _ = fmt.Fprintln(r.Text(), "Ready")
		return nil
	}))
}
