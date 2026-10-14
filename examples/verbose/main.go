// Command verbose shows what --verbose (Config.Verbose) adds: Task Facts. Free
// text written to Run.Text prints at every verbosity.
//
//	go run ./examples/verbose/
//	go run ./examples/verbose/ --verbose
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func main() {
	verbose := flag.Bool("verbose", false, "show Task Facts")
	flag.Parse()

	cfg := evo.Config{}
	cfg.Name = "resolve"
	if *verbose {
		cfg.Verbose = true
	}
	os.Exit(evo.Main(cfg, func(ctx context.Context, r *evo.Run) error {
		_, _ = fmt.Fprintln(r.Text(), "Reading configuration")
		_, _ = fmt.Fprintf(r.Text(), "Found %d packages\n", 18)

		lockfile := r.Task("lockfile")
		lockfile.Define(func(context.Context) error {
			// A learned value is a Fact, not a printed "Label: value" line.
			// Facts stay hidden until --verbose asks for them.
			lockfile.Fact("registry mirror", "us-east-1")
			lockfile.Fact("cache", "/var/cache/packages")
			return nil
		})
		return nil
	}))
}
