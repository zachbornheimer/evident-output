// Command verbose shows Normal vs Verbose message visibility.
//
//	go run ./examples/verbose/
//	go run ./examples/verbose/ --verbose
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	verbose := flag.Bool("verbose", false, "show Verbose() messages")
	flag.Parse()

	cfg := evo.Config{}
	cfg.Name = "resolve"
	if *verbose {
		cfg.Verbose = true
	}
	os.Exit(evo.Main(func(ctx context.Context, r *evo.Run) error {
		_, _ = fmt.Fprintln(r.Text(), "Reading configuration")
		_, _ = fmt.Fprintf(r.Text(), "Found %d packages\n", 18)
		// Hidden unless --verbose (still present in Snapshot.Messages).
		_, _ = fmt.Fprintln(r.Text(), "Using registry mirror us-east-1")

		lockfile := r.Task("lockfile")
		lockfile.Define(func(context.Context) error {
			// A learned value is a Fact, not a printed "Label: value" line.
			lockfile.Fact("cache", "/var/cache/packages")
			return nil
		})
		return nil
	}, cfg))
}
