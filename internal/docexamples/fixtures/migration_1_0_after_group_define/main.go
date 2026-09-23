// Package main compiles docs/migration/1.0.md's "After (1.0)" fence for
// Task.Each's replacement: one named child Task per item under a Group,
// each submitting its own work through Define. See
// TestDocFencesMatchFixtures. Never run.
package main

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

// The fenced block below references packages and install without defining
// them (the doc leaves them to the reader's own program); the fixture
// supplies trivial stand-ins purely so the snippet type-checks.
var packages = []string{"example-pkg"}

func install(pkg string) error { return nil }

func doWork() {
	// docexamples:snippet start
	installs := evo.Group("install")
	for _, pkg := range packages {
		installs.Task(pkg).Define(func(ctx context.Context) error {
			return install(pkg)
		})
	}
	// docexamples:snippet end
}

func main() { doWork() }
