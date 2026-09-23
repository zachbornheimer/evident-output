// Package main compiles docs/guides/teaching-ladder.md's "Evidence" fence
// verbatim. See TestDocFencesMatchFixtures. Never run.
package main

import (
	"os/exec"

	evo "github.com/zachbornheimer/evident-output"
)

func doWork(task *evo.TaskHandle, cmd *exec.Cmd) error {
	// docexamples:snippet start
	cmd.Stdout = task.Writer()
	cmd.Stderr = task.Writer()
	if err := cmd.Run(); err != nil {
		return task.Failf("failed: %w", err)
	}
	// docexamples:snippet end
	return nil
}

func main() {
	out := evo.Init(evo.Config{Isolated: true})
	_ = doWork(out.Task("build"), exec.Command("true"))
}
