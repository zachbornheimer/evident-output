// Package main compiles docs/migration/1.0.md's "After (1.0)" fence for an
// Isolated *Output exiting through its own Output.Run (the evo.MainWith
// replacement). See TestDocFencesMatchFixtures. Never run.
package main

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

// The fenced block below assumes ctx and run are already in scope (it is a
// continuation of the doc's own "Ordinary main() still uses evo.Main
// unchanged" narration); declared as fixture scaffolding here so the
// marked region matches the doc byte-for-byte.
var ctx = context.Background()

func run(context.Context) error { return nil }

func doWork() {
	// docexamples:snippet start
	out := evo.Init(evo.Config{Isolated: true})
	os.Exit(out.Run(ctx, run).ExitCode())
	// docexamples:snippet end
}

func main() { doWork() }
