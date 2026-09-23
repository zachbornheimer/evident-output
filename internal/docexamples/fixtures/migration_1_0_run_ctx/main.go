// Package main compiles docs/migration/1.0.md's "After (1.0)" fence for
// `Run`/`Main`/`Output.Run` taking and deriving a context.Context (the
// `run(ctx context.Context) error` shape). See TestDocFencesMatchFixtures.
// Never run.
package main

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

// docexamples:snippet start
func main() {
	evo.Init(evo.Config{Title: "ghost"})
	os.Exit(evo.Main(run))
}

func run(ctx context.Context) error {
	evo.Task("working tree").Done()
	return nil
}

// docexamples:snippet end
