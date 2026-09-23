// Package main compiles docs/reference.md's "brew packages" fence (child
// processes / tool-backed gates section) verbatim. See
// TestDocFencesMatchFixtures. Never run.
package main

import (
	"os/exec"

	evo "github.com/zachbornheimer/evident-output"
)

func doWork() error {
	out := evo.Init(evo.Config{Isolated: true})

	// docexamples:snippet start
	upgrade := out.Task("brew packages")
	cmd := exec.Command("brew", "upgrade", "--formula")
	cmd.Stdout = upgrade.Writer()
	cmd.Stderr = upgrade.Writer()
	if err := cmd.Run(); err != nil {
		return upgrade.Failf("brew upgrade failed: %w", err)
	}
	// docexamples:snippet end

	return nil
}

func main() { _ = doWork() }
