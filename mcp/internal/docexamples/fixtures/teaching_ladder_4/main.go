// Package main compiles mcp/docs/teaching-ladder.md's "Confirm" fence
// verbatim. See TestDocFencesMatchFixtures. Never run.
package main

import (
	evo "github.com/zachbornheimer/evident-output/v2"
)

func doWork(r *evo.Run) bool {
	// docexamples:snippet start
	ok := r.Confirm("delete origin/production-hotfix?", evo.Destructive(), evo.NonInteractive("--yes"))
	// docexamples:snippet end
	return ok
}

func main() { _ = doWork(nil) }
