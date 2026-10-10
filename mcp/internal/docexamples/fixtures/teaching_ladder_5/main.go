// Package main compiles mcp/docs/teaching-ladder.md's "Suspend (handing
// the tty to a child)" fence verbatim. See TestDocFencesMatchFixtures.
// Never run.
package main

import (
	"os"
	"os/exec"

	evo "github.com/zachbornheimer/evident-output"
)

func doWork(out *evo.Output) error {
	// docexamples:snippet start
	cmd := exec.Command("zq", "setup")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	_ = out.Suspend(func() error { return cmd.Run() })
	// docexamples:snippet end
	return nil
}

func main() {
	out := evo.Init(evo.Config{Isolated: true})
	_ = doWork(out)
}
