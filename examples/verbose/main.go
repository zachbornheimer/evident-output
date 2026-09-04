// Command verbose shows Normal vs Verbose message visibility.
//
//	go run ./examples/verbose/
//	go run ./examples/verbose/ --verbose
package main

import (
	"flag"

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
	evo.Main(func() error {
		evo.Println("Reading configuration")
		evo.Printf("Found %d packages\n", 18)
		// Hidden unless --verbose (still present in Snapshot.Messages).
		evo.Verbose().Printf("Cache: %s\n", "/var/cache/packages")
		evo.Verbose().Println("Using registry mirror us-east-1")

		evo.Task("lockfile").Done()
		return nil
	})
}
