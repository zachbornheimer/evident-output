// Package main compiles docs/development.md's "Machine output" fence
// verbatim. See TestDocFencesMatchFixtures. Never run.
package main

import (
	evo "github.com/zachbornheimer/evident-output"
)

func doWork() {
	out := evo.Init(evo.Config{Isolated: true})

	// docexamples:snippet start
	snap := out.Snapshot()
	plain, _ := evo.RenderPlain(snap, evo.PlainOptions{Width: 80})
	// docexamples:snippet end

	_ = plain
}

func main() { doWork() }
