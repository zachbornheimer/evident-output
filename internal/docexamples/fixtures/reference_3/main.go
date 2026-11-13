// Package main compiles docs/reference.md's "docker daemon" fence
// (tool-backed condition, checked in its Define callback) verbatim. See
// TestDocFencesMatchFixtures. Never run.
package main

import (
	"context"
	"errors"
	"fmt"

	evo "github.com/zachbornheimer/evident-output"
)

func pingDocker() error { return errors.New("daemon unreachable") }

func doWork() {
	out := evo.Init(evo.Config{Isolated: true})

	// docexamples:snippet start
	docker := out.Task("docker daemon")
	docker.Define(func(ctx context.Context) error {
		if err := pingDocker(); err != nil {
			return fmt.Errorf("could not inspect the daemon: %w", err)
		}
		return nil
	})
	// docexamples:snippet end
}

func main() { doWork() }
