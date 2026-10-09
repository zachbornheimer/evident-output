// Package main compiles docs/development.md's "Production ANSI driver"
// fence verbatim, split across this file and main.go — see
// TestDocFencesMatchFixtures and FixtureRegion in the parent package for
// why. Never run.
package main

// docexamples:snippet start
import "github.com/zachbornheimer/evident-output/internal/terminal"

// docexamples:snippet end

// This blank declaration keeps the import above used from this file — the
// doc fence stops at the bare import line, and the actual
// terminal.NewANSI call this import is for lives in main.go (a real
// caller-scope statement can't share a file with a file-scope import
// goimports would otherwise merge the two of them into one block, erasing
// the doc's own "just add this import" framing).
var _ terminal.Option
