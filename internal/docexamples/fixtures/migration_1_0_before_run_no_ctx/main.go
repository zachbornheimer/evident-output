//go:build ignore

// This file pins docs/migration/1.0.md's "Before (0.5)" fence for the
// `Run`/`Main`/`Output.Run` context.Context change — run() has no ctx
// parameter, so `evo.Main(run)` no longer type-checks against 1.0's
// `RunFunc = func(context.Context) error`. It is a byte-for-byte record of
// pre-1.0 API that must never compile again, so it carries a `//go:build
// ignore` tag: `go build ./...`/`go vet ./...` skip it, while
// TestDocFencesMatchFixtures still reads it (via docexamples.FixtureRegion,
// which walks the directory, not the build graph) and pins its exact text.
// See doc.go for the buildable half of this fixture package. Never run,
// never compiled.
package main

import (
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

// docexamples:snippet start
func main() {
	evo.Init(evo.Config{Title: "ghost"})
	os.Exit(evo.Main(run))
}

func run() error {
	evo.Task("working tree").Done()
	return nil
}

// docexamples:snippet end
