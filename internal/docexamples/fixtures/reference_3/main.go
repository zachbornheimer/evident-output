// Package main compiles docs/reference.md's "docker daemon" fence
// (tool-backed condition, resolved directly) verbatim. See
// TestDocFencesMatchFixtures. Never run.
package main

import (
	"errors"

	evo "github.com/zachbornheimer/evident-output"
)

func pingDocker() error { return errors.New("daemon unreachable") }

func doWork() {
	out := evo.Init(evo.Config{Isolated: true})

	// docexamples:snippet start
	docker := out.Task("docker daemon")
	if err := pingDocker(); err != nil {
		docker.Failf("could not inspect the daemon: %w", err)
	} else {
		docker.Done()
	}
	// docexamples:snippet end
}

func main() { doWork() }
