//go:build ignore

// This file pins docs/migration/1.0.md's Before/After fence for the
// `Result`-vs-bare-exit-code change. Its "Before" half, `code :=
// evo.Run(run)`, calls evo.Run with one argument against 1.0's two-argument
// `Run(ctx context.Context, run RunFunc) Result`, so the whole fence no
// longer type-checks as one file. It is a byte-for-byte record of pre-1.0
// API that must never compile again, so it carries a `//go:build ignore`
// tag: `go build ./...`/`go vet ./...` skip it, while
// TestDocFencesMatchFixtures still reads it (via docexamples.FixtureRegion,
// which walks the directory, not the build graph) and pins its exact text.
// See doc.go for the buildable half of this fixture package. Never run,
// never compiled.
package main

import evo "github.com/zachbornheimer/evident-output"

// snippet wraps the marked region in a function purely so gofmt (which
// parses every staged .go file regardless of build tags, in the
// pre-commit hook) can parse this file — the fence's bare `code :=`
// statements are only valid inside a function body. The wrapper itself
// is outside the markers and asserts nothing about the doc.
func snippet() {
	// docexamples:snippet start
	// Before
	code := evo.Run(run)

	// After
	result := evo.Run(ctx, run)
	code := result.ExitCode()
	// docexamples:snippet end
}
