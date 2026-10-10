// Package main compiles docs/migration/1.0.md's "duplicate sibling names
// are not a get-or-create" fence — both the discouraged repeated-call shape
// (still valid Go, just records ErrDuplicateSiblingName at runtime, which
// this fixture never runs) and the recommended keep-the-handle shape. See
// TestDocFencesMatchFixtures. Never run.
package main

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

func doWork() {
	// docexamples:snippet start
	// Before (relied on repeated calls resolving to the same task)
	evo.Task("branches").Doing("scanning")
	// ... later ...
	evo.Task("branches").Define(scanBranches)

	// After — keep the handle
	branches := evo.Task("branches")
	branches.Doing("scanning")
	// ... later ...
	branches.Define(scanBranches)
	// docexamples:snippet end
}

func scanBranches(ctx context.Context) error { return nil }

func main() { doWork() }
