//go:build ignore

// This file pins docs/adoption/librarian.md's construction fence, which
// documents the librarian case study's pre-1.0 call site: evo.New and
// evo.MainWith, both removed in 1.0 (the fence's own trailing comments say
// so and name the 1.0 equivalent). It is a byte-for-byte record of pre-1.0
// API that must never compile again, so it carries a `//go:build ignore`
// tag: `go build ./...`/`go vet ./...` skip it, while
// TestDocFencesMatchFixtures still reads it (via docexamples.FixtureRegion,
// which walks the directory, not the build graph) and pins its exact text.
// See doc.go for the buildable half of this fixture package. Never run,
// never compiled.
package main

import "os"

// snippet wraps the marked region in a function purely so gofmt (which
// parses every staged .go file regardless of build tags, in the
// pre-commit hook) can parse this file — the fence's bare `out :=`
// statement is only valid inside a function body. The wrapper itself is
// outside the markers and asserts nothing about the doc.
func snippet() {
	// docexamples:snippet start
	out := evo.New(evo.Config{ // evo.New; MainWith below — both removed in 1.0, see mcp/docs/mcp.md
		Title: "librarian",
		Debug: evo.DebugConfig{Level: evo.LevelWarn},
	})
	os.Exit(evo.MainWith(out, run)) // removed in 1.0 — current equivalent: os.Exit(out.Run(ctx, run).ExitCode())
	// docexamples:snippet end
}
