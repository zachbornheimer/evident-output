// Command verbose shows Normal vs Verbose message visibility.
//
//	go run ./examples/verbose/
//	go run ./examples/verbose/ --verbose
package main

import (
	"context"
	"flag"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	verbose := flag.Bool("verbose", false, "show Verbose() messages")
	flag.Parse()

	cfg := evo.DefaultConfig()
	cfg.Title = "resolve"
	if *verbose {
		cfg.Verbosity = evo.VerbosityVerbose
	}
	evo.Init(cfg)
	os.Exit(evo.Main(func(ctx context.Context) error {
		evo.Println("Reading configuration")
		evo.Printf("Found %d packages\n", 18)
		// Hidden unless --verbose (still present in Snapshot.Messages).
		evo.Verbose().Println("Using registry mirror us-east-1")

		lockfile := evo.Task("lockfile")
		lockfile.Define(func(context.Context) error {
			// A learned value is a Fact, not a printed "Label: value" line.
			lockfile.Fact("cache", "/var/cache/packages")
			return nil
		})
		return nil
	}))
}
