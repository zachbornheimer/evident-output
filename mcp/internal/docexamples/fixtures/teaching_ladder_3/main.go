// Package main compiles mcp/docs/teaching-ladder.md's "Evidence" fence
// verbatim. See TestDocFencesMatchFixtures. Never run.
package main

import (
	"context"
	"os/exec"

	evo "github.com/zachbornheimer/evident-output"
)

func doWork(task *evo.Task) {
	// docexamples:snippet start
	task.Define(func(ctx context.Context) error {
		cmd := exec.CommandContext(ctx, "make", "test")
		cmd.Stdout = task.Writer()
		cmd.Stderr = task.Writer()
		return cmd.Run()
	})
	// docexamples:snippet end
}

func main() {
	doWork(nil)
}
