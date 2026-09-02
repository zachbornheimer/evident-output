// Command verbose shows Normal vs Verbose message visibility.
//
//	go run ./examples/verbose/
//	go run ./examples/verbose/ --verbose
package main

import (
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
	out := evo.New(cfg)
	os.Exit(evo.MainWith(out, func(o *evo.Output) error {
		o.Println("Reading configuration")
		o.Printf("Found %d packages\n", 18)
		// Hidden unless --verbose (still present in Snapshot.Messages).
		o.Verbose().Printf("Cache: %s\n", "/var/cache/packages")
		o.Verbose().Println("Using registry mirror us-east-1")

		o.Item("lockfile").OK()
		return nil
	}))
}
