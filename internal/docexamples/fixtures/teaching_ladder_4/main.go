// Package main compiles docs/guides/teaching-ladder.md's "Confirm" fence
// verbatim. See TestDocFencesMatchFixtures. Never run.
package main

import (
	evo "github.com/zachbornheimer/evident-output"
)

var flagYes bool

func doWork() bool {
	// docexamples:snippet start
	ok := evo.Confirm("delete origin/production-hotfix?", evo.Destructive(), evo.AssumeYes(flagYes))
	// docexamples:snippet end
	return ok
}

func main() {
	evo.Init(evo.Config{Isolated: true})
	_ = doWork()
}
