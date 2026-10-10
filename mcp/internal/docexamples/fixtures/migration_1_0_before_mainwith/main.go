//go:build ignore

// This file pins docs/migration/1.0.md's "Before (0.5)" fence for
// evo.MainWith, which was removed in 1.0 — the call is undefined against
// the shipped API. It is a byte-for-byte record of pre-1.0 API that must
// never compile again, so it carries a `//go:build ignore` tag: `go build
// ./...`/`go vet ./...` skip it, while TestDocFencesMatchFixtures still
// reads it (via docexamples.FixtureRegion, which walks the directory, not
// the build graph) and pins its exact text. See doc.go for the buildable
// half of this fixture package. Never run, never compiled.
package main

import "os"

// snippet wraps the marked region in a function purely so gofmt (which
// parses every staged .go file regardless of build tags, in the
// pre-commit hook) can parse this file — the fence's bare `out :=`
// statement is only valid inside a function body. The wrapper itself is
// outside the markers and asserts nothing about the doc.
func snippet() {
	// docexamples:snippet start
	out := evo.Init(evo.Config{Isolated: true})
	os.Exit(evo.MainWith(out, run))
	// docexamples:snippet end
}
