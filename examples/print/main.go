// Command print is the first adoption rung: Print/Printf/Println like fmt.
//
//	go run ./examples/print/
package main

import (
	"context"
	"fmt"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	os.Exit(evo.Main(evo.Config{Name: "packages"}, func(ctx context.Context, r *evo.Run) error {
		// Managed, line-oriented replacement for human-facing fmt.Print*
		// (not a byte-for-byte fmt drop-in).
		_, _ = fmt.Fprintln(r.Text(), "Reading configuration")
		_, _ = fmt.Fprintf(r.Text(), "Found %d packages\n", 18)
		_, _ = fmt.Fprint(r.Text(), "Resolving")
		_, _ = fmt.Fprint(r.Text(), "…")
		_, _ = fmt.Fprint(r.Text(), "\n")
		_, _ = fmt.Fprintln(r.Text(), "Ready")
		return nil
	}))
}
