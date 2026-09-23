// Package main compiles the README.md Quickstart fence verbatim (see
// TestDocFencesMatchFixtures) so the flagship example never drifts from the
// shipped API. It is never run.
package main

// docexamples:snippet start
import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	evo.Init(evo.Config{Title: "bpp-csharp"}) // first statement — arms first paint before any I/O
	os.Exit(evo.Main(run))                    // exits the process itself; evo.Run(ctx, run) if you need the Result without exiting
}

func run(ctx context.Context) error {
	// Start as casually as fmt — then promote to structure when useful.
	evo.Println("Reading configuration")
	evo.Printf("Found %d packages\n", 18)

	evo.Task("working tree").Done()
	evo.Task("branches").Block(
		"local-only branch",
		evo.Detail("commit or stash before continuing"),
	)

	evo.Task("cleanup").Delete("stale local branch", func() error {
		return removeStaleBranches()
	}, evo.Affected(2)) // singular object, ledger renders "2 stale local branches"

	installs := evo.Group("install")
	for _, pkg := range packages {
		installs.Task(pkg).Define(func(ctx context.Context) error { return install(pkg) })
	}
	return nil // Block is a presentation outcome, not a Go error
}

// docexamples:snippet end

// The fenced block above references these three names without defining
// them (the doc leaves them to the reader's own program); the fixture
// supplies trivial stand-ins purely so the snippet type-checks.
var packages = []string{"example-pkg"}

func removeStaleBranches() error { return nil }

func install(pkg string) error { return nil }
