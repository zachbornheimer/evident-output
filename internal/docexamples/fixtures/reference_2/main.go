// Package main compiles docs/reference.md's "brew packages" fence (child
// processes / tool-backed gates section) verbatim. See
// TestDocFencesMatchFixtures. Never run.
package main

import (
	"context"
	"fmt"
	"os/exec"

	evo "github.com/zachbornheimer/evident-output"
)

func doWork() {
	out := evo.Init(evo.Config{Isolated: true})

	// docexamples:snippet start
	upgrade := out.Task("brew packages")
	upgrade.Define(func(ctx context.Context) error {
		cmd := exec.CommandContext(ctx, "brew", "upgrade", "--formula")
		cmd.Stdout = upgrade.Writer()
		cmd.Stderr = upgrade.Writer()
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("brew upgrade failed: %w", err)
		}
		return nil
	})
	// docexamples:snippet end
}

func main() { doWork() }
