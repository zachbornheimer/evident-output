//go:build ignore

// This file pins docs/migration/1.0.md's Format/Projection fence. Unlike
// the other fixtures in this directory it documents current (1.0) names —
// evo.ProjectionHuman etc. really exist (internal/engine/env.go) — but the
// fence itself elides the `Projection = iota + 1` typing/initializer the
// real const block carries, showing only the five names and their
// comments. That shape is not a complete Go const declaration (the first
// ConstSpec in a block needs a value), so it fails go vet's "missing init
// expr" check even though it parses. It is pinned as exact text only, not
// proven to compile, so it carries a `//go:build ignore` tag: `go build
// ./...`/`go vet ./...` skip it, while TestDocFencesMatchFixtures still
// reads it (via docexamples.FixtureRegion, which walks the directory, not
// the build graph) and pins its exact text. See doc.go for the buildable
// half of this fixture package. Never run, never compiled.
package main

// docexamples:snippet start
const (
	ProjectionHuman      // TTY/plain inference (default)
	ProjectionPlain      // durable report, no live region
	ProjectionJSON       // one JSONDocument at Finish
	ProjectionJSONL      // Event JSON Lines at Finish
	ProjectionStreamJSON // one EventJSON line per journal append
)

// docexamples:snippet end
